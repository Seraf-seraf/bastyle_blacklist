package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestFFmpegFrameExtractorReturnsNoMoreThanMaxFrames(t *testing.T) {
	extractor := newTestFFmpegFrameExtractor(t, "frames", time.Second)

	extracted, err := extractor.Extract(context.Background(), testVideoFile(), domain.MediaExtractionPlan{
		MaxFrames:    3,
		TargetWidth:  320,
		TargetHeight: 320,
	})
	if err != nil {
		t.Fatalf("extract frames: %v", err)
	}
	if len(extracted.Frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(extracted.Frames))
	}
	for _, frame := range extracted.Frames {
		if frame.Image == nil {
			t.Fatal("expected decoded frame image")
		}
	}
}

func TestFFmpegFrameExtractorStopsOnTimeout(t *testing.T) {
	extractor := newTestFFmpegFrameExtractor(t, "sleep", 20*time.Millisecond)

	start := time.Now()
	_, err := extractor.Extract(context.Background(), testVideoFile(), domain.MediaExtractionPlan{
		MaxFrames:    3,
		TargetWidth:  320,
		TargetHeight: 320,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("extract error = %v, want deadline exceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("extractor did not stop promptly after timeout")
	}
}

func TestFFmpegFrameExtractorReturnsErrorForCorruptedFrame(t *testing.T) {
	extractor := newTestFFmpegFrameExtractor(t, "invalid", time.Second)

	_, err := extractor.Extract(context.Background(), testVideoFile(), domain.MediaExtractionPlan{
		MaxFrames:    3,
		TargetWidth:  320,
		TargetHeight: 320,
	})
	if err == nil {
		t.Fatal("expected corrupted frame error")
	}
}

func TestFFmpegFrameExtractorReturnsCommandError(t *testing.T) {
	extractor := newTestFFmpegFrameExtractor(t, "fail", time.Second)

	_, err := extractor.Extract(context.Background(), testVideoFile(), domain.MediaExtractionPlan{
		MaxFrames:    3,
		TargetWidth:  320,
		TargetHeight: 320,
	})
	if err == nil {
		t.Fatal("expected ffmpeg command error")
	}
}

func TestNewFFmpegFrameExtractorRejectsInvalidDependencies(t *testing.T) {
	_, err := NewFFmpegFrameExtractor("", time.Second)
	if err == nil {
		t.Fatal("expected empty binary to be rejected")
	}

	_, err = NewFFmpegFrameExtractor("ffmpeg", 0)
	if err == nil {
		t.Fatal("expected invalid timeout to be rejected")
	}
}

func TestFFmpegFrameExtractorRejectsInvalidPlan(t *testing.T) {
	extractor := newTestFFmpegFrameExtractor(t, "frames", time.Second)

	tests := []domain.MediaExtractionPlan{
		{MaxFrames: 0, TargetWidth: 320, TargetHeight: 320},
		{MaxFrames: 21, TargetWidth: 320, TargetHeight: 320},
		{MaxFrames: 3, TargetWidth: 0, TargetHeight: 320},
		{MaxFrames: 3, TargetWidth: 1025, TargetHeight: 320},
		{MaxFrames: 3, TargetWidth: 320, TargetHeight: 0},
		{MaxFrames: 3, TargetWidth: 320, TargetHeight: 1025},
	}

	for _, plan := range tests {
		_, err := extractor.Extract(context.Background(), testVideoFile(), plan)
		if err == nil {
			t.Fatalf("expected invalid plan error for plan: %+v", plan)
		}
	}
}

func newTestFFmpegFrameExtractor(t *testing.T, mode string, timeout time.Duration) *ffmpegFrameExtractor {
	t.Helper()

	extractor, err := NewFFmpegFrameExtractor(writeFakeFFmpeg(t, mode), timeout)
	if err != nil {
		t.Fatalf("new extractor: %v", err)
	}

	typed, ok := extractor.(*ffmpegFrameExtractor)
	if !ok {
		t.Fatalf("extractor type = %T, want *ffmpegFrameExtractor", extractor)
	}

	return typed
}

func writeFakeFFmpeg(t *testing.T, mode string) string {
	t.Helper()

	pngData := base64.StdEncoding.EncodeToString(testPNG(t))
	script := `#!/bin/sh
mode="` + mode + `"
if [ "$mode" = "sleep" ]; then
  while :; do
    :
  done
fi

frames=1
pattern=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-frames:v" ]; then
    shift
    frames="$1"
  fi
  pattern="$1"
  shift
done

if [ "$mode" = "fail" ]; then
  exit 2
fi

if [ "$mode" = "invalid" ]; then
  path=$(printf "$pattern" 1)
  printf 'not-image' > "$path"
  exit 0
fi

frames=$((frames + 2))
i=1
while [ "$i" -le "$frames" ]; do
  path=$(printf "$pattern" "$i")
  printf '` + pngData + `' | base64 -d > "$path"
  i=$((i + 1))
done
`

	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	return path
}

func testPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 0xff, A: 0xff})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func testVideoFile() domain.MediaFile {
	return domain.MediaFile{
		Content: domain.Content{
			FileID:       "file-id",
			FileUniqueID: "unique-id",
			Type:         domain.MediaAnimation,
		},
		FilePath: "animation.mp4",
		Data:     []byte("video bytes"),
	}
}
