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
  token: "токен"
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
media_config:
  max_animation_duration: 9s
  max_video_sticker_duration: 3s
  max_animation_size: 1MB
  max_video_sticker_size: 200KB
  max_frames: 8
  target_width: 256
  target_height: 256
  max_upload_bytes: 1MB
  max_image_pixels: 2000000
  ffmpeg_binary: /usr/bin/ffmpeg
  ffmpeg_timeout: 7s
matching:
  exact:
    db_path: "exact.sqlite"
    buffer: 30
  image_hash:
    db_path: "test.sqlite"
    threshold: 8
    buffer: 40
  video_like:
    db_path: "video-like.sqlite"
    threshold: 9
    buffer: 50
    min_matched_frames: 3
    min_matched_ratio: 0.5
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
    min_matched_frames: 4
    min_matched_ratio: 0.6
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

	if cfg.Telegram.Token != "токен" {
		t.Fatalf("неожиданное значение токен: %q", cfg.Telegram.Token)
	}
	if cfg.Telegram.UpdateTimeoutSeconds != 45 {
		t.Fatalf("неожиданное значение таймаут обновлений: %d", cfg.Telegram.UpdateTimeoutSeconds)
	}
	if !cfg.Telegram.HTTPClient.Enabled {
		t.Fatal("ожидалось, что HTTP-клиент Telegram будет включен")
	}
	if cfg.Telegram.HTTPClient.ProxyURL != "http://127.0.0.1:8080" {
		t.Fatalf("неожиданное значение HTTP-клиент Telegram URL прокси: %q", cfg.Telegram.HTTPClient.ProxyURL)
	}
	if cfg.Workers != 3 {
		t.Fatalf("неожиданное значение воркеры: %d", cfg.Workers)
	}
	if !cfg.Health.Enabled {
		t.Fatal("ожидалось, что health-сервер будет включен")
	}
	if cfg.Health.Host != "127.0.0.1" {
		t.Fatalf("неожиданное значение хост health-сервера: %q", cfg.Health.Host)
	}
	if cfg.Health.Port != 18081 {
		t.Fatalf("неожиданное значение порт health-сервера: %d", cfg.Health.Port)
	}
	if cfg.JobsBuffer != 20 {
		t.Fatalf("неожиданное значение буфер задач: %d", cfg.JobsBuffer)
	}
	if cfg.Matching.Exact.DBPath != "exact.sqlite" {
		t.Fatalf("неожиданное значение exact путь БД: %q", cfg.Matching.Exact.DBPath)
	}
	if cfg.Matching.Exact.Buffer != 30 {
		t.Fatalf("неожиданное значение буфер exact: %d", cfg.Matching.Exact.Buffer)
	}
	if cfg.Matching.ImageHash.DBPath != "test.sqlite" {
		t.Fatalf("неожиданное значение image-hash путь БД: %q", cfg.Matching.ImageHash.DBPath)
	}
	if cfg.Matching.ImageHash.Threshold != 8 {
		t.Fatalf("неожиданное значение image-hash порог: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.Matching.ImageHash.Buffer != 40 {
		t.Fatalf("неожиданное значение image-hash буфер: %d", cfg.Matching.ImageHash.Buffer)
	}
	if cfg.MediaConfig.MaxAnimationDuration.Value() != 9*time.Second {
		t.Fatalf("неожиданное значение максимальная длительность анимации длительность: %s", cfg.MediaConfig.MaxAnimationDuration.Value())
	}
	if cfg.MediaConfig.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("неожиданное значение максимальная длительность видеостикера длительность: %s", cfg.MediaConfig.MaxVideoStickerDuration.Value())
	}
	if cfg.MediaConfig.MaxAnimationSize.Bytes() != 1000000 {
		t.Fatalf("неожиданное значение максимальная длительность анимации размер: %d", cfg.MediaConfig.MaxAnimationSize.Bytes())
	}
	if cfg.MediaConfig.MaxVideoStickerSize.Bytes() != 200000 {
		t.Fatalf("неожиданное значение максимальная длительность видеостикера размер: %d", cfg.MediaConfig.MaxVideoStickerSize.Bytes())
	}
	if cfg.Matching.VideoLike.DBPath != "video-like.sqlite" {
		t.Fatalf("неожиданное значение video-like путь БД: %q", cfg.Matching.VideoLike.DBPath)
	}
	if cfg.Matching.VideoLike.Threshold != 9 {
		t.Fatalf("неожиданное значение video-like порог: %d", cfg.Matching.VideoLike.Threshold)
	}
	if cfg.Matching.VideoLike.Buffer != 50 {
		t.Fatalf("неожиданное значение video-like буфер: %d", cfg.Matching.VideoLike.Buffer)
	}
	if cfg.Matching.VideoLike.MinMatchedFrames != 3 {
		t.Fatalf("неожиданное значение video-like минимум совпавших кадров: %d", cfg.Matching.VideoLike.MinMatchedFrames)
	}
	if cfg.Matching.VideoLike.MinMatchedRatio != 0.5 {
		t.Fatalf("неожиданное значение video-like минимальная доля совпадения: %f", cfg.Matching.VideoLike.MinMatchedRatio)
	}
	if cfg.MediaConfig.MaxFrames != 8 {
		t.Fatalf("неожиданное значение максимум кадров: %d", cfg.MediaConfig.MaxFrames)
	}
	if cfg.MediaConfig.TargetWidth != 256 {
		t.Fatalf("неожиданное значение целевая ширина: %d", cfg.MediaConfig.TargetWidth)
	}
	if cfg.MediaConfig.TargetHeight != 256 {
		t.Fatalf("неожиданное значение целевая высота: %d", cfg.MediaConfig.TargetHeight)
	}
	if cfg.MediaConfig.MaxUploadBytes.Bytes() != 1000000 {
		t.Fatalf("неожиданное значение максимум байт загрузки: %d", cfg.MediaConfig.MaxUploadBytes.Bytes())
	}
	if cfg.MediaConfig.MaxImagePixels != 2000000 {
		t.Fatalf("неожиданное значение максимум пикселей изображения: %d", cfg.MediaConfig.MaxImagePixels)
	}
	if cfg.MediaConfig.FFmpegBinary != "/usr/bin/ffmpeg" {
		t.Fatalf("неожиданное значение бинарный файл ffmpeg: %q", cfg.MediaConfig.FFmpegBinary)
	}
	if cfg.MediaConfig.FFmpegTimeout.Value() != 7*time.Second {
		t.Fatalf("неожиданное значение таймаут ffmpeg: %s", cfg.MediaConfig.FFmpegTimeout.Value())
	}
	if !cfg.Matching.AIVector.Enabled {
		t.Fatal("ожидалось, что AI-vector матчер будет включен")
	}
	if cfg.Matching.AIVector.ModelName != "test-model" {
		t.Fatalf("неожиданное значение AI-vector имя модели: %q", cfg.Matching.AIVector.ModelName)
	}
	if cfg.Matching.AIVector.ModelRevision != "test-revision" {
		t.Fatalf("неожиданное значение AI-vector ревизия модели: %q", cfg.Matching.AIVector.ModelRevision)
	}
	if cfg.Matching.AIVector.DBPath != "ai-vector.sqlite" {
		t.Fatalf("неожиданное значение AI-vector путь БД: %q", cfg.Matching.AIVector.DBPath)
	}
	if cfg.Matching.AIVector.IndexPath != "faiss.index" {
		t.Fatalf("неожиданное значение AI-vector путь индекса: %q", cfg.Matching.AIVector.IndexPath)
	}
	if cfg.Matching.AIVector.Threshold != 0.91 {
		t.Fatalf("неожиданное значение AI-vector порог: %f", cfg.Matching.AIVector.Threshold)
	}
	if cfg.Matching.AIVector.TopK != 7 {
		t.Fatalf("неожиданное значение AI-vector top_k: %d", cfg.Matching.AIVector.TopK)
	}
	if cfg.Matching.AIVector.MinMatchedFrames != 4 {
		t.Fatalf("неожиданное значение AI-vector минимум совпавших кадров: %d", cfg.Matching.AIVector.MinMatchedFrames)
	}
	if cfg.Matching.AIVector.MinMatchedRatio != 0.6 {
		t.Fatalf("неожиданное значение AI-vector минимальная доля совпадения: %f", cfg.Matching.AIVector.MinMatchedRatio)
	}
	if cfg.Matching.AIVector.RequestTimeout.Value() != 6*time.Second {
		t.Fatalf("неожиданное значение AI-vector таймаут запроса: %s", cfg.Matching.AIVector.RequestTimeout.Value())
	}
	if cfg.Matching.AIVector.Service.Host != "127.0.0.1" {
		t.Fatalf("неожиданное значение AI-vector host сервиса: %q", cfg.Matching.AIVector.Service.Host)
	}
	if cfg.Matching.AIVector.Service.Port != 18080 {
		t.Fatalf("неожиданное значение AI-vector port сервиса: %d", cfg.Matching.AIVector.Service.Port)
	}
	if cfg.Matching.AIVector.HNSW.M != 16 {
		t.Fatalf("неожиданное значение AI-vector HNSW m: %d", cfg.Matching.AIVector.HNSW.M)
	}
	if cfg.Matching.AIVector.HNSW.EFConstruction != 40 {
		t.Fatalf("неожиданное значение AI-vector HNSW ef_construction: %d", cfg.Matching.AIVector.HNSW.EFConstruction)
	}
	if cfg.Matching.AIVector.HNSW.EFSearch != 24 {
		t.Fatalf("неожиданное значение AI-vector HNSW ef_search: %d", cfg.Matching.AIVector.HNSW.EFSearch)
	}
}

func TestLoadKeepsDefaultsForMissingOptionalValues(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Workers != 5 {
		t.Fatalf("неожиданное значение по умолчанию: воркеры: %d", cfg.Workers)
	}
	if cfg.JobsBuffer != 100 {
		t.Fatalf("неожиданное значение по умолчанию: буфер задач: %d", cfg.JobsBuffer)
	}
	if !cfg.Health.Enabled {
		t.Fatal("ожидалось, что health-сервер по умолчанию будет включен")
	}
	if cfg.Health.Host != "127.0.0.1" {
		t.Fatalf("неожиданное значение по умолчанию: хост health-сервера: %q", cfg.Health.Host)
	}
	if cfg.Health.Port != 8081 {
		t.Fatalf("неожиданное значение по умолчанию: порт health-сервера: %d", cfg.Health.Port)
	}
	if cfg.Matching.Exact.DBPath != "bastyle.sqlite" {
		t.Fatalf("неожиданное значение по умолчанию: exact путь БД: %q", cfg.Matching.Exact.DBPath)
	}
	if cfg.Matching.ImageHash.Threshold != 12 {
		t.Fatalf("неожиданное значение по умолчанию: image-hash порог: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.MediaConfig.MaxAnimationDuration.Value() != 10*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: максимальная длительность анимации длительность: %s", cfg.MediaConfig.MaxAnimationDuration.Value())
	}
	if cfg.MediaConfig.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: максимальная длительность видеостикера длительность: %s", cfg.MediaConfig.MaxVideoStickerDuration.Value())
	}
	if cfg.MediaConfig.MaxAnimationSize.Bytes() != 20<<20 {
		t.Fatalf("неожиданное значение по умолчанию: максимальная длительность анимации размер: %d", cfg.MediaConfig.MaxAnimationSize.Bytes())
	}
	if cfg.MediaConfig.MaxVideoStickerSize.Bytes() != 256<<10 {
		t.Fatalf("неожиданное значение по умолчанию: максимальная длительность видеостикера размер: %d", cfg.MediaConfig.MaxVideoStickerSize.Bytes())
	}
	if cfg.Matching.VideoLike.DBPath != "bastyle.sqlite" {
		t.Fatalf("неожиданное значение по умолчанию: video-like путь БД: %q", cfg.Matching.VideoLike.DBPath)
	}
	if cfg.Matching.VideoLike.Threshold != 12 {
		t.Fatalf("неожиданное значение по умолчанию: video-like порог: %d", cfg.Matching.VideoLike.Threshold)
	}
	if cfg.Matching.VideoLike.Buffer != 500 {
		t.Fatalf("неожиданное значение по умолчанию: video-like буфер: %d", cfg.Matching.VideoLike.Buffer)
	}
	if cfg.Matching.VideoLike.MinMatchedFrames != 2 {
		t.Fatalf("неожиданное значение по умолчанию: video-like минимум совпавших кадров: %d", cfg.Matching.VideoLike.MinMatchedFrames)
	}
	if cfg.Matching.VideoLike.MinMatchedRatio != 0.4 {
		t.Fatalf("неожиданное значение по умолчанию: video-like минимальная доля совпадения: %f", cfg.Matching.VideoLike.MinMatchedRatio)
	}
	if cfg.MediaConfig.MaxFrames != 10 {
		t.Fatalf("неожиданное значение по умолчанию: максимум кадров: %d", cfg.MediaConfig.MaxFrames)
	}
	if cfg.MediaConfig.TargetWidth != 320 {
		t.Fatalf("неожиданное значение по умолчанию: целевая ширина: %d", cfg.MediaConfig.TargetWidth)
	}
	if cfg.MediaConfig.TargetHeight != 320 {
		t.Fatalf("неожиданное значение по умолчанию: целевая высота: %d", cfg.MediaConfig.TargetHeight)
	}
	if cfg.MediaConfig.FFmpegBinary != "ffmpeg" {
		t.Fatalf("неожиданное значение по умолчанию: бинарный файл ffmpeg: %q", cfg.MediaConfig.FFmpegBinary)
	}
	if cfg.MediaConfig.FFmpegTimeout.Value() != 10*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: таймаут ffmpeg: %s", cfg.MediaConfig.FFmpegTimeout.Value())
	}
	if cfg.Telegram.HTTPClient.Enabled {
		t.Fatal("ожидалось, что HTTP-клиент Telegram по умолчанию будет выключен")
	}
	if cfg.Telegram.HTTPClient.ProxyURL != "" {
		t.Fatalf("неожиданное значение по умолчанию: HTTP-клиент Telegram URL прокси: %q", cfg.Telegram.HTTPClient.ProxyURL)
	}
	if cfg.Matching.AIVector.Enabled {
		t.Fatal("ожидалось, что AI-vector матчер по умолчанию будет выключен")
	}
	if cfg.Matching.AIVector.Threshold != 0.92 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector порог: %f", cfg.Matching.AIVector.Threshold)
	}
	if cfg.Matching.AIVector.TopK != 5 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector top_k: %d", cfg.Matching.AIVector.TopK)
	}
	if cfg.Matching.AIVector.MinMatchedFrames != 2 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector минимум совпавших кадров: %d", cfg.Matching.AIVector.MinMatchedFrames)
	}
	if cfg.Matching.AIVector.MinMatchedRatio != 0.4 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector минимальная доля совпадения: %f", cfg.Matching.AIVector.MinMatchedRatio)
	}
	if cfg.Matching.AIVector.RequestTimeout.Value() != 10*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector таймаут запроса: %s", cfg.Matching.AIVector.RequestTimeout.Value())
	}
	if cfg.Matching.AIVector.Service.Host != "127.0.0.1" {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector host сервиса: %q", cfg.Matching.AIVector.Service.Host)
	}
	if cfg.Matching.AIVector.Service.Port != 8080 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector port сервиса: %d", cfg.Matching.AIVector.Service.Port)
	}
	if cfg.Matching.AIVector.HNSW.M != 32 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector HNSW m: %d", cfg.Matching.AIVector.HNSW.M)
	}
}

func TestLoadRejectsMediaConfigFieldsUnderVideoLike(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
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
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsUnknownVideoLikeField(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    typo: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
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
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsInvalidMediaConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
media_config:
  max_animation_duration: 0s
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsInvalidMediaConfigSize(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
media_config:
  max_animation_size: "not-a-size"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsExcessiveMediaConfigFrames(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
media_config:
  max_frames: 21
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsExcessiveMediaConfigTargetSize(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
media_config:
  target_width: 1025
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsEmptyMediaConfigFFmpegBinary(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
media_config:
  ffmpeg_binary: ""
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsInvalidHTTPClientProxyURL(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
  http_client:
    enabled: true
    proxy_url: "127.0.0.1:8080"
matching:
  image_hash:
    db_path: "test.sqlite"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsVideoLikeEnabledField(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
  video_like:
    enabled: false
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsInvalidAIVectorConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
  ai_vector:
    enabled: true
    threshold: 1.1
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsInvalidAIVectorServiceURL(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
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
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadAllowsInvalidDisabledAIVectorConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
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
