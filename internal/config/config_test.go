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
  http_client:
    enabled: true
    proxy_url: "http://127.0.0.1:8080"
workers: 3
health:
  enabled: true
  host: "127.0.0.1"
  port: 18081
jobs_buffer: 20
matching:
  exact:
    buffer: 30
  image_hash:
    db_path: "test.sqlite"
    threshold: 8
    buffer: 40
  video_media:
    max_animation_duration: 9s
    max_video_sticker_duration: 3s
    max_animation_size: 1MB
    max_video_sticker_size: 200KB
    max_frames: 8
    target_width: 256
    target_height: 256
    max_upload_bytes: 1000000
    max_image_pixels: 2000000
    ffmpeg_binary: /usr/bin/ffmpeg
    ffmpeg_timeout: 7s
  video_match:
    min_matched_frames: 3
    min_matched_ratio: 0.5
  video_like:
    db_path: "video-like.sqlite"
    threshold: 9
    buffer: 50
  ai_vector:
    enabled: true
    model_name: "test-model"
    model_revision: "test-revision"
    device: "cpu"
    db_path: "ai-vector.sqlite"
    index_path: "faiss.index"
    max_files: 8
    threshold: 0.91
    top_k: 7
    request_timeout: 6s
    service:
      host: "127.0.0.1"
      port: 18080
    hnsw:
      m: 16
      ef_construction: 40
      ef_search: 24
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
	if !cfg.Telegram.HTTPClient.Enabled {
		t.Fatal("expected telegram http client to be enabled")
	}
	if cfg.Telegram.HTTPClient.ProxyURL != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected telegram http client proxy url: %q", cfg.Telegram.HTTPClient.ProxyURL)
	}
	if cfg.Workers != 3 {
		t.Fatalf("unexpected workers: %d", cfg.Workers)
	}
	if !cfg.Health.Enabled {
		t.Fatal("expected health to be enabled")
	}
	if cfg.Health.Host != "127.0.0.1" {
		t.Fatalf("unexpected health host: %q", cfg.Health.Host)
	}
	if cfg.Health.Port != 18081 {
		t.Fatalf("unexpected health port: %d", cfg.Health.Port)
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
	if cfg.Matching.VideoMedia.MaxAnimationDuration.Value() != 9*time.Second {
		t.Fatalf("unexpected max animation duration: %s", cfg.Matching.VideoMedia.MaxAnimationDuration.Value())
	}
	if cfg.Matching.VideoMedia.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("unexpected max video sticker duration: %s", cfg.Matching.VideoMedia.MaxVideoStickerDuration.Value())
	}
	if cfg.Matching.VideoMedia.MaxAnimationSize.Bytes() != 1000000 {
		t.Fatalf("unexpected max animation size: %d", cfg.Matching.VideoMedia.MaxAnimationSize.Bytes())
	}
	if cfg.Matching.VideoMedia.MaxVideoStickerSize.Bytes() != 200000 {
		t.Fatalf("unexpected max video sticker size: %d", cfg.Matching.VideoMedia.MaxVideoStickerSize.Bytes())
	}
	if cfg.Matching.VideoLike.DBPath != "video-like.sqlite" {
		t.Fatalf("unexpected video like db path: %q", cfg.Matching.VideoLike.DBPath)
	}
	if cfg.Matching.VideoLike.Threshold != 9 {
		t.Fatalf("unexpected video like threshold: %d", cfg.Matching.VideoLike.Threshold)
	}
	if cfg.Matching.VideoLike.Buffer != 50 {
		t.Fatalf("unexpected video like buffer: %d", cfg.Matching.VideoLike.Buffer)
	}
	if cfg.Matching.VideoMatch.MinMatchedFrames != 3 {
		t.Fatalf("unexpected min matched frames: %d", cfg.Matching.VideoMatch.MinMatchedFrames)
	}
	if cfg.Matching.VideoMatch.MinMatchedRatio != 0.5 {
		t.Fatalf("unexpected min matched ratio: %f", cfg.Matching.VideoMatch.MinMatchedRatio)
	}
	if cfg.Matching.VideoMedia.MaxFrames != 8 {
		t.Fatalf("unexpected max frames: %d", cfg.Matching.VideoMedia.MaxFrames)
	}
	if cfg.Matching.VideoMedia.TargetWidth != 256 {
		t.Fatalf("unexpected target width: %d", cfg.Matching.VideoMedia.TargetWidth)
	}
	if cfg.Matching.VideoMedia.TargetHeight != 256 {
		t.Fatalf("unexpected target height: %d", cfg.Matching.VideoMedia.TargetHeight)
	}
	if cfg.Matching.VideoMedia.MaxUploadBytes != 1000000 {
		t.Fatalf("unexpected max upload bytes: %d", cfg.Matching.VideoMedia.MaxUploadBytes)
	}
	if cfg.Matching.VideoMedia.MaxImagePixels != 2000000 {
		t.Fatalf("unexpected max image pixels: %d", cfg.Matching.VideoMedia.MaxImagePixels)
	}
	if cfg.Matching.VideoMedia.FFmpegBinary != "/usr/bin/ffmpeg" {
		t.Fatalf("unexpected ffmpeg binary: %q", cfg.Matching.VideoMedia.FFmpegBinary)
	}
	if cfg.Matching.VideoMedia.FFmpegTimeout.Value() != 7*time.Second {
		t.Fatalf("unexpected ffmpeg timeout: %s", cfg.Matching.VideoMedia.FFmpegTimeout.Value())
	}
	if !cfg.Matching.AIVector.Enabled {
		t.Fatal("expected ai vector matcher to be enabled")
	}
	if cfg.Matching.AIVector.ModelName != "test-model" {
		t.Fatalf("unexpected ai vector model name: %q", cfg.Matching.AIVector.ModelName)
	}
	if cfg.Matching.AIVector.ModelRevision != "test-revision" {
		t.Fatalf("unexpected ai vector model revision: %q", cfg.Matching.AIVector.ModelRevision)
	}
	if cfg.Matching.AIVector.DBPath != "ai-vector.sqlite" {
		t.Fatalf("unexpected ai vector db path: %q", cfg.Matching.AIVector.DBPath)
	}
	if cfg.Matching.AIVector.IndexPath != "faiss.index" {
		t.Fatalf("unexpected ai vector index path: %q", cfg.Matching.AIVector.IndexPath)
	}
	if cfg.Matching.AIVector.Threshold != 0.91 {
		t.Fatalf("unexpected ai vector threshold: %f", cfg.Matching.AIVector.Threshold)
	}
	if cfg.Matching.AIVector.TopK != 7 {
		t.Fatalf("unexpected ai vector top k: %d", cfg.Matching.AIVector.TopK)
	}
	if cfg.Matching.AIVector.RequestTimeout.Value() != 6*time.Second {
		t.Fatalf("unexpected ai vector request timeout: %s", cfg.Matching.AIVector.RequestTimeout.Value())
	}
	if cfg.Matching.AIVector.Service.Host != "127.0.0.1" {
		t.Fatalf("unexpected ai vector service host: %q", cfg.Matching.AIVector.Service.Host)
	}
	if cfg.Matching.AIVector.Service.Port != 18080 {
		t.Fatalf("unexpected ai vector service port: %d", cfg.Matching.AIVector.Service.Port)
	}
	if cfg.Matching.AIVector.HNSW.M != 16 {
		t.Fatalf("unexpected ai vector hnsw m: %d", cfg.Matching.AIVector.HNSW.M)
	}
	if cfg.Matching.AIVector.HNSW.EFConstruction != 40 {
		t.Fatalf("unexpected ai vector hnsw ef construction: %d", cfg.Matching.AIVector.HNSW.EFConstruction)
	}
	if cfg.Matching.AIVector.HNSW.EFSearch != 24 {
		t.Fatalf("unexpected ai vector hnsw ef search: %d", cfg.Matching.AIVector.HNSW.EFSearch)
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
	if !cfg.Health.Enabled {
		t.Fatal("expected default health to be enabled")
	}
	if cfg.Health.Host != "127.0.0.1" {
		t.Fatalf("unexpected default health host: %q", cfg.Health.Host)
	}
	if cfg.Health.Port != 8081 {
		t.Fatalf("unexpected default health port: %d", cfg.Health.Port)
	}
	if cfg.Matching.ImageHash.Threshold != 12 {
		t.Fatalf("unexpected default image hash threshold: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.Matching.VideoMedia.MaxAnimationDuration.Value() != 10*time.Second {
		t.Fatalf("unexpected default max animation duration: %s", cfg.Matching.VideoMedia.MaxAnimationDuration.Value())
	}
	if cfg.Matching.VideoMedia.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("unexpected default max video sticker duration: %s", cfg.Matching.VideoMedia.MaxVideoStickerDuration.Value())
	}
	if cfg.Matching.VideoMedia.MaxAnimationSize.Bytes() != 20<<20 {
		t.Fatalf("unexpected default max animation size: %d", cfg.Matching.VideoMedia.MaxAnimationSize.Bytes())
	}
	if cfg.Matching.VideoMedia.MaxVideoStickerSize.Bytes() != 256<<10 {
		t.Fatalf("unexpected default max video sticker size: %d", cfg.Matching.VideoMedia.MaxVideoStickerSize.Bytes())
	}
	if cfg.Matching.VideoLike.DBPath != "bastyle.sqlite" {
		t.Fatalf("unexpected default video like db path: %q", cfg.Matching.VideoLike.DBPath)
	}
	if cfg.Matching.VideoLike.Threshold != 12 {
		t.Fatalf("unexpected default video like threshold: %d", cfg.Matching.VideoLike.Threshold)
	}
	if cfg.Matching.VideoLike.Buffer != 500 {
		t.Fatalf("unexpected default video like buffer: %d", cfg.Matching.VideoLike.Buffer)
	}
	if cfg.Matching.VideoMatch.MinMatchedFrames != 2 {
		t.Fatalf("unexpected default min matched frames: %d", cfg.Matching.VideoMatch.MinMatchedFrames)
	}
	if cfg.Matching.VideoMatch.MinMatchedRatio != 0.4 {
		t.Fatalf("unexpected default min matched ratio: %f", cfg.Matching.VideoMatch.MinMatchedRatio)
	}
	if cfg.Matching.VideoMedia.MaxFrames != 10 {
		t.Fatalf("unexpected default max frames: %d", cfg.Matching.VideoMedia.MaxFrames)
	}
	if cfg.Matching.VideoMedia.TargetWidth != 320 {
		t.Fatalf("unexpected default target width: %d", cfg.Matching.VideoMedia.TargetWidth)
	}
	if cfg.Matching.VideoMedia.TargetHeight != 320 {
		t.Fatalf("unexpected default target height: %d", cfg.Matching.VideoMedia.TargetHeight)
	}
	if cfg.Matching.VideoMedia.FFmpegBinary != "ffmpeg" {
		t.Fatalf("unexpected default ffmpeg binary: %q", cfg.Matching.VideoMedia.FFmpegBinary)
	}
	if cfg.Matching.VideoMedia.FFmpegTimeout.Value() != 10*time.Second {
		t.Fatalf("unexpected default ffmpeg timeout: %s", cfg.Matching.VideoMedia.FFmpegTimeout.Value())
	}
	if cfg.Telegram.HTTPClient.Enabled {
		t.Fatal("expected default telegram http client to be disabled")
	}
	if cfg.Telegram.HTTPClient.ProxyURL != "" {
		t.Fatalf("unexpected default telegram http client proxy url: %q", cfg.Telegram.HTTPClient.ProxyURL)
	}
	if cfg.Matching.AIVector.Enabled {
		t.Fatal("expected default ai vector matcher to be disabled")
	}
	if cfg.Matching.AIVector.Threshold != 0.92 {
		t.Fatalf("unexpected default ai vector threshold: %f", cfg.Matching.AIVector.Threshold)
	}
	if cfg.Matching.AIVector.TopK != 5 {
		t.Fatalf("unexpected default ai vector top k: %d", cfg.Matching.AIVector.TopK)
	}
	if cfg.Matching.AIVector.RequestTimeout.Value() != 10*time.Second {
		t.Fatalf("unexpected default ai vector request timeout: %s", cfg.Matching.AIVector.RequestTimeout.Value())
	}
	if cfg.Matching.AIVector.Service.Host != "127.0.0.1" {
		t.Fatalf("unexpected default ai vector service host: %q", cfg.Matching.AIVector.Service.Host)
	}
	if cfg.Matching.AIVector.Service.Port != 8080 {
		t.Fatalf("unexpected default ai vector service port: %d", cfg.Matching.AIVector.Service.Port)
	}
	if cfg.Matching.AIVector.HNSW.M != 32 {
		t.Fatalf("unexpected default ai vector hnsw m: %d", cfg.Matching.AIVector.HNSW.M)
	}
}

func TestLoadRejectsVideoMediaFieldsUnderVideoLike(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    max_animation_duration: 8s
    max_video_sticker_duration: 2s
    max_animation_size: 2MiB
    max_video_sticker_size: 128KiB
    max_frames: 7
    target_width: 240
    target_height: 220
    ffmpeg_binary: /usr/local/bin/ffmpeg
    ffmpeg_timeout: 6s
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsUnknownVideoLikeField(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    typo: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
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
  video_media:
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
  video_media:
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
  video_media:
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
  video_media:
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
  video_media:
    ffmpeg_binary: ""
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidHTTPClientProxyURL(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
  http_client:
    enabled: true
    proxy_url: "127.0.0.1:8080"
matching:
  image_hash:
    db_path: "test.sqlite"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsVideoLikeEnabledField(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    enabled: false
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidAIVectorConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  ai_vector:
    enabled: true
    threshold: 1.1
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsInvalidAIVectorServiceURL(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  ai_vector:
    enabled: true
    service:
      host: ""
      port: 8080
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadAllowsInvalidDisabledAIVectorConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "token"
matching:
  image_hash:
    db_path: "test.sqlite"
  ai_vector:
    enabled: false
    threshold: 2
    service:
      host: ""
      port: 0
`)

	_, err := Load(path)
	if err != nil {
		t.Fatal(err)
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
