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
database:
  dsn: "postgres://user:pass@db:5432/app?sslmode=disable"
  max_conns: 12
  min_conns: 2
  max_conn_lifetime: 2h
  max_conn_idle_time: 20m
  health_check_period: 45s
  connect_timeout: 6s
  statement_timeout: 11s
  migration:
    enabled: false
rabbitmq:
  url: "amqp://user:pass@rabbitmq:5672/"
  exchange: "test.events"
  exchange_type: "direct"
  publish_timeout: 4s
  reconnect_interval: 6s
outbox_publisher:
  enabled: true
  instance_id: "test-publisher"
  batch_size: 11
  poll_interval: 2s
  idle_interval: 3s
  lock_ttl: 15s
  retry_base_delay: 7s
  retry_max_delay: 1m
  max_attempts: 9
consumers:
  index_events:
    enabled: true
    replica_id: "replica-1"
    queue_template: "bastyle.replica.%s.events"
    routing_keys:
      - "media.ban.#"
      - "index.#"
    prefetch: 4
    retry_delay: 8s
    catch_up_interval: 9s
    catch_up_batch_size: 12
metrics:
  enabled: true
  host: "127.0.0.1"
  port: 19090
  path: "/custom_metrics"
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
    buffer: 30
  image_hash:
    threshold: 8
    buffer: 40
  video_like:
    threshold: 9
    buffer: 50
    min_matched_frames: 3
    min_matched_ratio: 0.5
  ai_vector:
    enabled: true
    model_name: "test-model"
    model_revision: "test-revision"
    device: "cpu"
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
	if cfg.Health.Address() != "127.0.0.1:18081" {
		t.Fatalf("неожиданное значение адрес health-сервера: %q", cfg.Health.Address())
	}
	if cfg.Database.DSN != "postgres://user:pass@db:5432/app?sslmode=disable" {
		t.Fatalf("неожиданное значение DSN БД: %q", cfg.Database.DSN)
	}
	if cfg.Database.MaxConns != 12 {
		t.Fatalf("неожиданное значение максимум соединений БД: %d", cfg.Database.MaxConns)
	}
	if cfg.Database.MinConns != 2 {
		t.Fatalf("неожиданное значение минимум соединений БД: %d", cfg.Database.MinConns)
	}
	if cfg.Database.MaxConnLifetime.Value() != 2*time.Hour {
		t.Fatalf("неожиданное значение время жизни соединения БД: %s", cfg.Database.MaxConnLifetime.Value())
	}
	if cfg.Database.MaxConnIdleTime.Value() != 20*time.Minute {
		t.Fatalf("неожиданное значение время простоя соединения БД: %s", cfg.Database.MaxConnIdleTime.Value())
	}
	if cfg.Database.HealthCheckPeriod.Value() != 45*time.Second {
		t.Fatalf("неожиданное значение период проверки БД: %s", cfg.Database.HealthCheckPeriod.Value())
	}
	if cfg.Database.ConnectTimeout.Value() != 6*time.Second {
		t.Fatalf("неожиданное значение таймаут подключения БД: %s", cfg.Database.ConnectTimeout.Value())
	}
	if cfg.Database.StatementTimeout.Value() != 11*time.Second {
		t.Fatalf("неожиданное значение таймаут SQL-запроса: %s", cfg.Database.StatementTimeout.Value())
	}
	if cfg.Database.Migration.Enabled {
		t.Fatal("ожидалось, что миграции БД будут выключены")
	}
	if cfg.RabbitMQ.URL != "amqp://user:pass@rabbitmq:5672/" {
		t.Fatalf("неожиданное значение RabbitMQ URL: %q", cfg.RabbitMQ.URL)
	}
	if cfg.RabbitMQ.Exchange != "test.events" {
		t.Fatalf("неожиданное значение RabbitMQ exchange: %q", cfg.RabbitMQ.Exchange)
	}
	if cfg.RabbitMQ.ExchangeType != "direct" {
		t.Fatalf("неожиданное значение RabbitMQ exchange type: %q", cfg.RabbitMQ.ExchangeType)
	}
	if cfg.RabbitMQ.PublishTimeout.Value() != 4*time.Second {
		t.Fatalf("неожиданное значение RabbitMQ publish timeout: %s", cfg.RabbitMQ.PublishTimeout.Value())
	}
	if cfg.RabbitMQ.ReconnectInterval.Value() != 6*time.Second {
		t.Fatalf("неожиданное значение RabbitMQ reconnect interval: %s", cfg.RabbitMQ.ReconnectInterval.Value())
	}
	if cfg.OutboxPublisher.InstanceID != "test-publisher" {
		t.Fatalf("неожиданное значение outbox publisher instance_id: %q", cfg.OutboxPublisher.InstanceID)
	}
	if cfg.OutboxPublisher.BatchSize != 11 {
		t.Fatalf("неожиданное значение outbox publisher batch_size: %d", cfg.OutboxPublisher.BatchSize)
	}
	if cfg.OutboxPublisher.PollInterval.Value() != 2*time.Second {
		t.Fatalf("неожиданное значение outbox publisher poll_interval: %s", cfg.OutboxPublisher.PollInterval.Value())
	}
	if cfg.OutboxPublisher.LockTTL.Value() != 15*time.Second {
		t.Fatalf("неожиданное значение outbox publisher lock_ttl: %s", cfg.OutboxPublisher.LockTTL.Value())
	}
	if cfg.OutboxPublisher.RetryMaxDelay.Value() != time.Minute {
		t.Fatalf("неожиданное значение outbox publisher retry_max_delay: %s", cfg.OutboxPublisher.RetryMaxDelay.Value())
	}
	if cfg.OutboxPublisher.MaxAttempts != 9 {
		t.Fatalf("неожиданное значение outbox publisher max_attempts: %d", cfg.OutboxPublisher.MaxAttempts)
	}
	if !cfg.Consumers.IndexEvents.Enabled {
		t.Fatal("ожидалось, что index_events consumer будет включен")
	}
	if cfg.Consumers.IndexEvents.ReplicaID != "replica-1" {
		t.Fatalf("неожиданное значение index_events replica_id: %q", cfg.Consumers.IndexEvents.ReplicaID)
	}
	if cfg.Consumers.IndexEvents.QueueTemplate != "bastyle.replica.%s.events" {
		t.Fatalf("неожиданное значение index_events queue_template: %q", cfg.Consumers.IndexEvents.QueueTemplate)
	}
	if len(cfg.Consumers.IndexEvents.RoutingKeys) != 2 || cfg.Consumers.IndexEvents.RoutingKeys[0] != "media.ban.#" || cfg.Consumers.IndexEvents.RoutingKeys[1] != "index.#" {
		t.Fatalf("неожиданное значение index_events routing_keys: %#v", cfg.Consumers.IndexEvents.RoutingKeys)
	}
	if cfg.Consumers.IndexEvents.Prefetch != 4 {
		t.Fatalf("неожиданное значение index_events prefetch: %d", cfg.Consumers.IndexEvents.Prefetch)
	}
	if cfg.Consumers.IndexEvents.CatchUpInterval.Value() != 9*time.Second {
		t.Fatalf("неожиданное значение index_events catch_up_interval: %s", cfg.Consumers.IndexEvents.CatchUpInterval.Value())
	}
	if cfg.Consumers.IndexEvents.CatchUpBatchSize != 12 {
		t.Fatalf("неожиданное значение index_events catch_up_batch_size: %d", cfg.Consumers.IndexEvents.CatchUpBatchSize)
	}
	if cfg.Metrics.Address() != "127.0.0.1:19090" {
		t.Fatalf("неожиданное значение metrics address: %q", cfg.Metrics.Address())
	}
	if cfg.Metrics.Path != "/custom_metrics" {
		t.Fatalf("неожиданное значение metrics path: %q", cfg.Metrics.Path)
	}
	if cfg.JobsBuffer != 20 {
		t.Fatalf("неожиданное значение буфер задач: %d", cfg.JobsBuffer)
	}
	if cfg.Matching.Exact.Buffer != 30 {
		t.Fatalf("неожиданное значение буфер exact: %d", cfg.Matching.Exact.Buffer)
	}
	if cfg.Matching.ImageHash.Threshold != 8 {
		t.Fatalf("неожиданное значение image-hash порог: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.Matching.ImageHash.Buffer != 40 {
		t.Fatalf("неожиданное значение image-hash буфер: %d", cfg.Matching.ImageHash.Buffer)
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
	if cfg.MediaConfig.MaxAnimationDuration.Value() != 9*time.Second {
		t.Fatalf("неожиданное значение максимальная длительность анимации: %s", cfg.MediaConfig.MaxAnimationDuration.Value())
	}
	if cfg.MediaConfig.MaxVideoStickerDuration.Value() != 3*time.Second {
		t.Fatalf("неожиданное значение максимальная длительность видеостикера: %s", cfg.MediaConfig.MaxVideoStickerDuration.Value())
	}
	if cfg.MediaConfig.MaxAnimationSize.Bytes() != 1000000 {
		t.Fatalf("неожиданное значение максимальный размер анимации: %d", cfg.MediaConfig.MaxAnimationSize.Bytes())
	}
	if cfg.MediaConfig.MaxVideoStickerSize.Bytes() != 200000 {
		t.Fatalf("неожиданное значение максимальный размер видеостикера: %d", cfg.MediaConfig.MaxVideoStickerSize.Bytes())
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
	if cfg.Matching.AIVector.Service.URL() != "http://127.0.0.1:18080" {
		t.Fatalf("неожиданное значение AI-vector URL сервиса: %q", cfg.Matching.AIVector.Service.URL())
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
	if cfg.Database.DSN != "postgres://bastyle:bastyle@bastyle-postgresql:5432/bastyle?sslmode=disable" {
		t.Fatalf("неожиданное значение по умолчанию: DSN БД: %q", cfg.Database.DSN)
	}
	if cfg.Database.MaxConns != 10 {
		t.Fatalf("неожиданное значение по умолчанию: максимум соединений БД: %d", cfg.Database.MaxConns)
	}
	if cfg.Database.MinConns != 1 {
		t.Fatalf("неожиданное значение по умолчанию: минимум соединений БД: %d", cfg.Database.MinConns)
	}
	if cfg.Database.MaxConnLifetime.Value() != time.Hour {
		t.Fatalf("неожиданное значение по умолчанию: время жизни соединения БД: %s", cfg.Database.MaxConnLifetime.Value())
	}
	if cfg.Database.MaxConnIdleTime.Value() != 15*time.Minute {
		t.Fatalf("неожиданное значение по умолчанию: время простоя соединения БД: %s", cfg.Database.MaxConnIdleTime.Value())
	}
	if cfg.Database.HealthCheckPeriod.Value() != 30*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: период проверки БД: %s", cfg.Database.HealthCheckPeriod.Value())
	}
	if cfg.Database.ConnectTimeout.Value() != 5*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: таймаут подключения БД: %s", cfg.Database.ConnectTimeout.Value())
	}
	if cfg.Database.StatementTimeout.Value() != 10*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: таймаут SQL-запроса: %s", cfg.Database.StatementTimeout.Value())
	}
	if !cfg.Database.Migration.Enabled {
		t.Fatal("ожидалось, что миграции БД по умолчанию будут включены")
	}
	if cfg.RabbitMQ.URL != "amqp://guest:guest@bastyle-rabbitmq:5672/" {
		t.Fatalf("неожиданное значение по умолчанию: RabbitMQ URL: %q", cfg.RabbitMQ.URL)
	}
	if cfg.RabbitMQ.Exchange != "bastyle.events" {
		t.Fatalf("неожиданное значение по умолчанию: RabbitMQ exchange: %q", cfg.RabbitMQ.Exchange)
	}
	if cfg.RabbitMQ.ExchangeType != "topic" {
		t.Fatalf("неожиданное значение по умолчанию: RabbitMQ exchange type: %q", cfg.RabbitMQ.ExchangeType)
	}
	if !cfg.OutboxPublisher.Enabled {
		t.Fatal("ожидалось, что outbox publisher по умолчанию будет включен")
	}
	if cfg.OutboxPublisher.BatchSize != 50 {
		t.Fatalf("неожиданное значение по умолчанию: outbox batch_size: %d", cfg.OutboxPublisher.BatchSize)
	}
	if cfg.OutboxPublisher.RetryMaxDelay.Value() != 10*time.Minute {
		t.Fatalf("неожиданное значение по умолчанию: outbox retry_max_delay: %s", cfg.OutboxPublisher.RetryMaxDelay.Value())
	}
	if cfg.Consumers.IndexEvents.Enabled {
		t.Fatal("ожидалось, что index_events consumer по умолчанию будет выключен")
	}
	if cfg.Consumers.IndexEvents.QueueTemplate != "bastyle.replica.%s.events" {
		t.Fatalf("неожиданное значение по умолчанию: index_events queue_template: %q", cfg.Consumers.IndexEvents.QueueTemplate)
	}
	if cfg.Consumers.IndexEvents.CatchUpBatchSize != 100 {
		t.Fatalf("неожиданное значение по умолчанию: index_events catch_up_batch_size: %d", cfg.Consumers.IndexEvents.CatchUpBatchSize)
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Fatalf("неожиданное значение по умолчанию: metrics path: %q", cfg.Metrics.Path)
	}
	if cfg.Matching.Exact.Buffer != 500 {
		t.Fatalf("неожиданное значение по умолчанию: exact буфер: %d", cfg.Matching.Exact.Buffer)
	}
	if cfg.Matching.ImageHash.Threshold != 12 {
		t.Fatalf("неожиданное значение по умолчанию: image-hash порог: %d", cfg.Matching.ImageHash.Threshold)
	}
	if cfg.Matching.VideoLike.MinMatchedFrames != 2 {
		t.Fatalf("неожиданное значение по умолчанию: video-like минимум совпавших кадров: %d", cfg.Matching.VideoLike.MinMatchedFrames)
	}
	if cfg.MediaConfig.MaxAnimationDuration.Value() != 10*time.Second {
		t.Fatalf("неожиданное значение по умолчанию: максимальная длительность анимации: %s", cfg.MediaConfig.MaxAnimationDuration.Value())
	}
	if cfg.MediaConfig.MaxVideoStickerSize.Bytes() != 256<<10 {
		t.Fatalf("неожиданное значение по умолчанию: максимальный размер видеостикера: %d", cfg.MediaConfig.MaxVideoStickerSize.Bytes())
	}
	if cfg.MediaConfig.FFmpegBinary != "ffmpeg" {
		t.Fatalf("неожиданное значение по умолчанию: бинарный файл ffmpeg: %q", cfg.MediaConfig.FFmpegBinary)
	}
	if cfg.Telegram.HTTPClient.Enabled {
		t.Fatal("ожидалось, что HTTP-клиент Telegram по умолчанию будет выключен")
	}
	if cfg.Matching.AIVector.Enabled {
		t.Fatal("ожидалось, что AI-vector матчер по умолчанию будет выключен")
	}
	if cfg.Matching.AIVector.Threshold != 0.92 {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector порог: %f", cfg.Matching.AIVector.Threshold)
	}
	if cfg.Matching.AIVector.Service.URL() != "http://127.0.0.1:8080" {
		t.Fatalf("неожиданное значение по умолчанию: AI-vector URL сервиса: %q", cfg.Matching.AIVector.Service.URL())
	}
}

func TestLoadAppliesSecretEnvOverrides(t *testing.T) {
	t.Setenv("BASTYLE_DATABASE_DSN", "postgres://env-user:env-pass@postgres:5432/bastyle?sslmode=disable")
	t.Setenv("BASTYLE_RABBITMQ_URL", "amqp://env-user:env-pass@rabbitmq:5672/")
	t.Setenv("BASTYLE_TELEGRAM_TOKEN", "env-token")

	path := writeConfig(t, `
telegram:
  token: ""
database:
  dsn: ""
rabbitmq:
  url: ""
outbox_publisher:
  enabled: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Database.DSN != "postgres://env-user:env-pass@postgres:5432/bastyle?sslmode=disable" {
		t.Fatalf("неожиданное значение DSN БД из env: %q", cfg.Database.DSN)
	}
	if cfg.RabbitMQ.URL != "amqp://env-user:env-pass@rabbitmq:5672/" {
		t.Fatalf("неожиданное значение RabbitMQ URL из env: %q", cfg.RabbitMQ.URL)
	}
	if cfg.Telegram.Token != "env-token" {
		t.Fatalf("неожиданное значение Telegram token из env: %q", cfg.Telegram.Token)
	}
}

func TestLoadIgnoresBlankSecretEnvOverrides(t *testing.T) {
	t.Setenv("BASTYLE_DATABASE_DSN", " \t\n ")
	t.Setenv("BASTYLE_RABBITMQ_URL", " \t\n ")
	t.Setenv("BASTYLE_TELEGRAM_TOKEN", " \t\n ")

	path := writeConfig(t, `
telegram:
  token: "yaml-token"
database:
  dsn: "postgres://yaml-user:yaml-pass@postgres:5432/bastyle?sslmode=disable"
rabbitmq:
  url: "amqp://yaml-user:yaml-pass@rabbitmq:5672/"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Database.DSN != "postgres://yaml-user:yaml-pass@postgres:5432/bastyle?sslmode=disable" {
		t.Fatalf("неожиданное значение DSN БД после пустого env: %q", cfg.Database.DSN)
	}
	if cfg.RabbitMQ.URL != "amqp://yaml-user:yaml-pass@rabbitmq:5672/" {
		t.Fatalf("неожиданное значение RabbitMQ URL после пустого env: %q", cfg.RabbitMQ.URL)
	}
	if cfg.Telegram.Token != "yaml-token" {
		t.Fatalf("неожиданное значение Telegram token после пустого env: %q", cfg.Telegram.Token)
	}
}

func TestLoadRejectsDeprecatedMatcherDBPath(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  image_hash:
    db_path: "test.sqlite"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsMediaConfigFieldsUnderVideoLike(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
matching:
  video_like:
    max_animation_duration: 8s
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
media_config:
  max_frames: 21
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

func TestLoadRejectsInvalidDatabaseConfig(t *testing.T) {
	tests := map[string]string{
		"dsn": `
database:
  dsn: ""
`,
		"max_conns": `
database:
  max_conns: 0
`,
		"min_conns": `
database:
  min_conns: 11
  max_conns: 10
`,
		"connect_timeout": `
database:
  connect_timeout: 0s
`,
		"statement_timeout": `
database:
  statement_timeout: 0s
`,
	}

	for name, databaseConfig := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, `
telegram:
  token: "токен"
`+databaseConfig)

			_, err := Load(path)
			if err == nil {
				t.Fatal("ожидалось: ошибка")
			}
		})
	}
}

func TestLoadRejectsInvalidOutboxPublisherConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
outbox_publisher:
  batch_size: 0
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsMissingRabbitMQURLWhenOutboxEnabled(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
rabbitmq:
  url: ""
outbox_publisher:
  enabled: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadAllowsMissingRabbitMQURLWhenOutboxDisabled(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
rabbitmq:
  url: ""
outbox_publisher:
  enabled: false
`)

	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsInvalidEnabledConsumerConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
consumers:
  index_events:
    enabled: true
    queue_template: ""
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
	}
}

func TestLoadRejectsInvalidMetricsConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  token: "токен"
metrics:
  enabled: true
  path: metrics
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалось: ошибка")
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
