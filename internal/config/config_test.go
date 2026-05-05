package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadReadsYAMLConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
  update_timeout_seconds: 45
workers: 3
jobs_buffer: 20
matching:
  exact:
    buffer: 30
  image_hash:
    db_path: "test.sqlite"
    threshold: 8
    buffer: 40
  video_like:
    max_animation_duration: 9s
    max_video_sticker_duration: 3s
    max_animation_size: 1MB
    max_video_sticker_size: 200KB
    max_frames: 8
    target_width: 256
    target_height: 256
    ffmpeg_binary: /usr/bin/ffmpeg
    ffmpeg_timeout: 7s
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Telegram.Token != "token" {
		t.Fatalf("unexpected token: %q", cfg.Telegram.Token)
	}
	if cfg.Telegram.UpdateTimeoutSeconds != 45 {
		t.Fatalf("unexpected update timeout: %d", cfg.Telegram.UpdateTimeoutSeconds)
	}
	if cfg.Workers != 3 {
		t.Fatalf("unexpected workers: %d", cfg.Workers)
	}
	if cfg.JobsBuffer != 20 {
		t.Fatalf("unexpected jobs buffer: %d", cfg.JobsBuffer)
	}
	if cfg.Matching.Exact.Buffer != 30 {
		t.Fatalf("unexpected exact buffer: %d", cfg.Matching.Exact.Buffer)
	}
	if cfg.Matching.ImageHash.DBPath != "test.sqlite" {
		t.Fatalf("unexpected image hash db path: %q", cfg.Matching.ImageHash.DBPath)
	}
	if cfg.Matching.ImageHash.Threshold != 8 {
		t.Fatalf("unexpected image hash threshold: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.Matching.ImageHash.Buffer != 40 {
		t.Fatalf("unexpected image hash buffer: %d", cfg.Matching.ImageHash.Buffer)
	}
	if cfg.Matching.VideoLike.MaxAnimationDuration.Value() != 9*time.Second {
		t.Fatalf("unexpected max animation duration: %s", cfg.Matching.VideoLike.MaxAnimationDuration.Value())
	}
	if cfg.Matching.VideoLike.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("unexpected max video sticker duration: %s", cfg.Matching.VideoLike.MaxVideoStickerDuration.Value())
	}
	if cfg.Matching.VideoLike.MaxAnimationSize.Bytes() != 1000000 {
		t.Fatalf("unexpected max animation size: %d", cfg.Matching.VideoLike.MaxAnimationSize.Bytes())
	}
	if cfg.Matching.VideoLike.MaxVideoStickerSize.Bytes() != 200000 {
		t.Fatalf("unexpected max video sticker size: %d", cfg.Matching.VideoLike.MaxVideoStickerSize.Bytes())
	}
	if cfg.Matching.VideoLike.MaxFrames != 8 {
		t.Fatalf("unexpected max frames: %d", cfg.Matching.VideoLike.MaxFrames)
	}
	if cfg.Matching.VideoLike.TargetWidth != 256 {
		t.Fatalf("unexpected target width: %d", cfg.Matching.VideoLike.TargetWidth)
	}
	if cfg.Matching.VideoLike.TargetHeight != 256 {
		t.Fatalf("unexpected target height: %d", cfg.Matching.VideoLike.TargetHeight)
	}
	if cfg.Matching.VideoLike.FFmpegBinary != "/usr/bin/ffmpeg" {
		t.Fatalf("unexpected ffmpeg binary: %q", cfg.Matching.VideoLike.FFmpegBinary)
	}
	if cfg.Matching.VideoLike.FFmpegTimeout.Value() != 7*time.Second {
		t.Fatalf("unexpected ffmpeg timeout: %s", cfg.Matching.VideoLike.FFmpegTimeout.Value())
	}
}

func TestLoadKeepsDefaultsForMissingOptionalValues(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Workers != 5 {
		t.Fatalf("unexpected default workers: %d", cfg.Workers)
	}
	if cfg.JobsBuffer != 100 {
		t.Fatalf("unexpected default jobs buffer: %d", cfg.JobsBuffer)
	}
	if cfg.Matching.ImageHash.Threshold != 12 {
		t.Fatalf("unexpected default image hash threshold: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.Matching.VideoLike.MaxAnimationDuration.Value() != 10*time.Second {
		t.Fatalf("unexpected default max animation duration: %s", cfg.Matching.VideoLike.MaxAnimationDuration.Value())
	}
	if cfg.Matching.VideoLike.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("unexpected default max video sticker duration: %s", cfg.Matching.VideoLike.MaxVideoStickerDuration.Value())
	}
	if cfg.Matching.VideoLike.MaxAnimationSize.Bytes() != 20<<20 {
		t.Fatalf("unexpected default max animation size: %d", cfg.Matching.VideoLike.MaxAnimationSize.Bytes())
	}
	if cfg.Matching.VideoLike.MaxVideoStickerSize.Bytes() != 256<<10 {
		t.Fatalf("unexpected default max video sticker size: %d", cfg.Matching.VideoLike.MaxVideoStickerSize.Bytes())
	}
	if cfg.Matching.VideoLike.MaxFrames != 10 {
		t.Fatalf("unexpected default max frames: %d", cfg.Matching.VideoLike.MaxFrames)
	}
	if cfg.Matching.VideoLike.TargetWidth != 320 {
		t.Fatalf("unexpected default target width: %d", cfg.Matching.VideoLike.TargetWidth)
	}
	if cfg.Matching.VideoLike.TargetHeight != 320 {
		t.Fatalf("unexpected default target height: %d", cfg.Matching.VideoLike.TargetHeight)
	}
	if cfg.Matching.VideoLike.FFmpegBinary != "ffmpeg" {
		t.Fatalf("unexpected default ffmpeg binary: %q", cfg.Matching.VideoLike.FFmpegBinary)
	}
	if cfg.Matching.VideoLike.FFmpegTimeout.Value() != 10*time.Second {
		t.Fatalf("unexpected default ffmpeg timeout: %s", cfg.Matching.VideoLike.FFmpegTimeout.Value())
	}
}

func TestLoadRequiresTelegramToken(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: ""
matching:
  image_hash:
    db_path: "test.sqlite"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidVideoLikeConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    max_animation_duration: 0s
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidVideoLikeSize(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    max_animation_size: "not-a-size"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsExcessiveVideoLikeFrames(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    max_frames: 21
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsExcessiveVideoLikeTargetSize(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    target_width: 1025
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsEmptyFFmpegBinary(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    ffmpeg_binary: ""
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	return path
}
