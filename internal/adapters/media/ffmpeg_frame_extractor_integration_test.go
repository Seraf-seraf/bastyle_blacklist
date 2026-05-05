package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

func TestFFmpegFrameExtractorIntegrationMP4AnimationSample(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	media := generatedFFmpegMedia(t, ffmpeg, "animation.mp4")
	extractor := newRealFFmpegExtractor(t, ffmpeg, 5*time.Second)

	extracted, err := extractor.Extract(context.Background(), media, smallFFmpegIntegrationPlan())
	if err != nil {
		t.Fatalf("extract mp4 animation sample: %v", err)
	}
	if len(extracted.Frames) == 0 {
		t.Fatal("expected extracted frames from mp4 animation sample")
	}
}

func TestFFmpegFrameExtractorIntegrationWEBMStickerSample(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	media := generatedFFmpegMedia(t, ffmpeg, "sticker.webm")
	media.Content.Type = domain.MediaStickerStatic
	extractor := newRealFFmpegExtractor(t, ffmpeg, 5*time.Second)

	extracted, err := extractor.Extract(context.Background(), media, smallFFmpegIntegrationPlan())
	if err != nil {
		t.Fatalf("extract webm sticker sample: %v", err)
	}
	if len(extracted.Frames) == 0 {
		t.Fatal("expected extracted frames from webm sticker sample")
	}
}

func TestFFmpegFrameExtractorIntegrationBrokenFile(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	extractor := newRealFFmpegExtractor(t, ffmpeg, 5*time.Second)

	_, err := extractor.Extract(context.Background(), domain.MediaFile{
		Content: domain.Content{
			FileID:       "broken-file",
			FileUniqueID: "broken-unique",
			Type:         domain.MediaAnimation,
		},
		FilePath: "broken.mp4",
		Data:     []byte("not a real video"),
	}, smallFFmpegIntegrationPlan())
	if err == nil {
		t.Fatal("expected broken media file to fail extraction")
	}
}

func TestFFmpegFrameExtractorIntegrationTimeout(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	media := generatedFFmpegMedia(t, ffmpeg, "animation.mp4")
	extractor := newRealFFmpegExtractor(t, ffmpeg, time.Nanosecond)

	_, err := extractor.Extract(context.Background(), media, smallFFmpegIntegrationPlan())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("extract error = %v, want deadline exceeded", err)
	}
}

func TestFFmpegFrameExtractorIntegrationFrameLimitCase(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	media := generatedFFmpegMedia(t, ffmpeg, "animation.mp4")
	extractor := newRealFFmpegExtractor(t, ffmpeg, 5*time.Second)

	_, err := extractor.Extract(context.Background(), media, domain.MediaExtractionPlan{
		MaxFrames:    21,
		TargetWidth:  64,
		TargetHeight: 64,
	})
	if err == nil {
		t.Fatal("expected oversized extraction plan to be rejected before ffmpeg runs")
	}
}

func BenchmarkFFmpegFrameExtractorSmallAnimation(b *testing.B) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		b.Skip("ffmpeg is not available")
	}

	media := generatedFFmpegMediaB(b, ffmpeg, "animation.mp4")
	extractor, err := NewFFmpegFrameExtractor(ffmpeg, 5*time.Second)
	if err != nil {
		b.Fatalf("new extractor: %v", err)
	}
	plan := smallFFmpegIntegrationPlan()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := extractor.Extract(context.Background(), media, plan); err != nil {
			b.Fatalf("extract small animation: %v", err)
		}
	}
}

func requireFFmpeg(t *testing.T) string {
	t.Helper()

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}

	return ffmpeg
}

func newRealFFmpegExtractor(t *testing.T, binary string, timeout time.Duration) portsExtractor {
	t.Helper()

	extractor, err := NewFFmpegFrameExtractor(binary, timeout)
	if err != nil {
		t.Fatalf("new ffmpeg extractor: %v", err)
	}

	return extractor
}

type portsExtractor interface {
	Extract(context.Context, domain.MediaFile, domain.MediaExtractionPlan) (domain.ExtractedMedia, error)
}

func smallFFmpegIntegrationPlan() domain.MediaExtractionPlan {
	return domain.MediaExtractionPlan{
		MaxFrames:    2,
		TargetWidth:  64,
		TargetHeight: 64,
	}
}

func generatedFFmpegMedia(t *testing.T, ffmpeg string, name string) domain.MediaFile {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	writeGeneratedFFmpegMedia(t, ffmpeg, path)
	return readGeneratedFFmpegMedia(t, path)
}

func generatedFFmpegMediaB(b *testing.B, ffmpeg string, name string) domain.MediaFile {
	b.Helper()

	path := filepath.Join(b.TempDir(), name)
	writeGeneratedFFmpegMediaB(b, ffmpeg, path)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read generated media: %v", err)
	}

	return domain.MediaFile{
		Content: domain.Content{
			FileID:       "generated-file",
			FileUniqueID: "generated-unique",
			Type:         domain.MediaAnimation,
			DurationSec:  2,
			Width:        64,
			Height:       64,
		},
		FilePath: path,
		Data:     data,
	}
}

func writeGeneratedFFmpegMedia(t *testing.T, ffmpeg string, path string) {
	t.Helper()

	if err := runGenerateMedia(ffmpeg, path); err != nil {
		t.Fatalf("generate media sample: %v", err)
	}
}

func writeGeneratedFFmpegMediaB(b *testing.B, ffmpeg string, path string) {
	b.Helper()

	if err := runGenerateMedia(ffmpeg, path); err != nil {
		b.Fatalf("generate media sample: %v", err)
	}
}

func readGeneratedFFmpegMedia(t *testing.T, path string) domain.MediaFile {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated media: %v", err)
	}

	return domain.MediaFile{
		Content: domain.Content{
			FileID:       "generated-file",
			FileUniqueID: "generated-unique",
			Type:         domain.MediaAnimation,
			DurationSec:  2,
			Width:        64,
			Height:       64,
		},
		FilePath: path,
		Data:     data,
	}
}

func runGenerateMedia(ffmpeg string, path string) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=64x64:rate=2:duration=2",
	}
	if filepath.Ext(path) == ".webm" {
		args = append(args, "-c:v", "libvpx-vp9", "-pix_fmt", "yuv420p")
	} else {
		args = append(args, "-c:v", "mpeg4", "-pix_fmt", "yuv420p")
	}
	args = append(args, path)

	return exec.Command(ffmpeg, args...).Run()
}
