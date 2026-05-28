package config

import (
	"bytes"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/dustin/go-humanize"
	"gopkg.in/yaml.v3"
)

const (
	maxMediaConfigFrames          = 20
	maxMediaConfigTargetDimension = 1024
)

type Config struct {
	Telegram        Telegram        `yaml:"telegram"`
	Workers         int             `yaml:"workers"`
	Health          Health          `yaml:"health"`
	Database        Database        `yaml:"database"`
	RabbitMQ        RabbitMQ        `yaml:"rabbitmq"`
	OutboxPublisher OutboxPublisher `yaml:"outbox_publisher"`
	Consumers       Consumers       `yaml:"consumers"`
	Metrics         Metrics         `yaml:"metrics"`

	JobsBuffer  int         `yaml:"jobs_buffer"`
	MediaConfig MediaConfig `yaml:"media_config"`
	Matching    Matching    `yaml:"matching"`
}

type Telegram struct {
	Token                string     `yaml:"token"`
	UpdateTimeoutSeconds int        `yaml:"update_timeout_seconds"`
	HTTPClient           HTTPClient `yaml:"http_client"`
}

type HTTPClient struct {
	Enabled  bool   `yaml:"enabled"`
	ProxyURL string `yaml:"proxy_url"`
}

type Health struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

type Database struct {
	DSN               string            `yaml:"dsn"`
	MaxConns          int               `yaml:"max_conns"`
	MinConns          int               `yaml:"min_conns"`
	MaxConnLifetime   Duration          `yaml:"max_conn_lifetime"`
	MaxConnIdleTime   Duration          `yaml:"max_conn_idle_time"`
	HealthCheckPeriod Duration          `yaml:"health_check_period"`
	ConnectTimeout    Duration          `yaml:"connect_timeout"`
	StatementTimeout  Duration          `yaml:"statement_timeout"`
	Migration         DatabaseMigration `yaml:"migration"`
}

type DatabaseMigration struct {
	Enabled bool `yaml:"enabled"`
}

type RabbitMQ struct {
	URL               string   `yaml:"url"`
	Exchange          string   `yaml:"exchange"`
	ExchangeType      string   `yaml:"exchange_type"`
	PublishTimeout    Duration `yaml:"publish_timeout"`
	ReconnectInterval Duration `yaml:"reconnect_interval"`
}

type OutboxPublisher struct {
	Enabled        bool     `yaml:"enabled"`
	InstanceID     string   `yaml:"instance_id"`
	BatchSize      int      `yaml:"batch_size"`
	PollInterval   Duration `yaml:"poll_interval"`
	IdleInterval   Duration `yaml:"idle_interval"`
	LockTTL        Duration `yaml:"lock_ttl"`
	RetryBaseDelay Duration `yaml:"retry_base_delay"`
	RetryMaxDelay  Duration `yaml:"retry_max_delay"`
	MaxAttempts    int      `yaml:"max_attempts"`
}

type Consumers struct {
	IndexEvents Consumer `yaml:"index_events"`
}

type Consumer struct {
	Enabled          bool     `yaml:"enabled"`
	ReplicaID        string   `yaml:"replica_id"`
	QueueTemplate    string   `yaml:"queue_template"`
	RoutingKeys      []string `yaml:"routing_keys"`
	Prefetch         int      `yaml:"prefetch"`
	RetryDelay       Duration `yaml:"retry_delay"`
	CatchUpInterval  Duration `yaml:"catch_up_interval"`
	CatchUpBatchSize int      `yaml:"catch_up_batch_size"`
}

type Metrics struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

type Matching struct {
	Exact     Exact     `yaml:"exact"`
	ImageHash ImageHash `yaml:"image_hash"`
	VideoLike VideoLike `yaml:"video_like"`
	AIVector  AIVector  `yaml:"ai_vector"`
}

type Exact struct {
	Buffer int `yaml:"buffer"`
}

type ImageHash struct {
	Threshold int `yaml:"threshold"`
	Buffer    int `yaml:"buffer"`
}

type VideoLike struct {
	Threshold        int     `yaml:"threshold"`
	Buffer           int     `yaml:"buffer"`
	MinMatchedFrames int     `yaml:"min_matched_frames"`
	MinMatchedRatio  float64 `yaml:"min_matched_ratio"`
}

type AIVector struct {
	Enabled          bool            `yaml:"enabled"`
	ModelName        string          `yaml:"model_name"`
	ModelRevision    string          `yaml:"model_revision"`
	Device           string          `yaml:"device"`
	IndexPath        string          `yaml:"index_path"`
	MaxFiles         int             `yaml:"max_files"`
	Threshold        float64         `yaml:"threshold"`
	TopK             int             `yaml:"top_k"`
	MinMatchedFrames int             `yaml:"min_matched_frames"`
	MinMatchedRatio  float64         `yaml:"min_matched_ratio"`
	RequestTimeout   Duration        `yaml:"request_timeout"`
	Service          AIVectorService `yaml:"service"`
	HNSW             HNSW            `yaml:"hnsw"`
}

type AIVectorService struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type MediaConfig struct {
	MaxFrames               int      `yaml:"max_frames"`
	TargetWidth             int      `yaml:"target_width"`
	TargetHeight            int      `yaml:"target_height"`
	MaxUploadBytes          ByteSize `yaml:"max_upload_bytes"`
	MaxImagePixels          int64    `yaml:"max_image_pixels"`
	MaxAnimationDuration    Duration `yaml:"max_animation_duration"`
	MaxVideoStickerDuration Duration `yaml:"max_video_sticker_duration"`
	MaxAnimationSize        ByteSize `yaml:"max_animation_size"`
	MaxVideoStickerSize     ByteSize `yaml:"max_video_sticker_size"`
	FFmpegBinary            string   `yaml:"ffmpeg_binary"`
	FFmpegTimeout           Duration `yaml:"ffmpeg_timeout"`
}

type HNSW struct {
	M              int `yaml:"m"`
	EFConstruction int `yaml:"ef_construction"`
	EFSearch       int `yaml:"ef_search"`
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	const methodCtx = "config/Duration.UnmarshalYAML"

	var raw string
	if err := value.Decode(&raw); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	duration, err := time.ParseDuration(raw)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	*d = Duration(duration)
	return nil
}

func (d *Duration) Value() time.Duration {
	return time.Duration(*d)
}

type ByteSize int64

func (s *ByteSize) UnmarshalYAML(value *yaml.Node) error {
	const methodCtx = "config/ByteSize.UnmarshalYAML"

	var raw string
	if err := value.Decode(&raw); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	size, err := humanize.ParseBytes(raw)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	*s = ByteSize(size)
	return nil
}

func (s *ByteSize) Bytes() int64 {
	return int64(*s)
}

func (h Health) Address() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

func (m Metrics) Address() string {
	return net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
}

func (s AIVectorService) URL() string {
	return "http://" + net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

func Load(path string) (Config, error) {
	const methodCtx = "config/Load"

	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, apperrors.Wrap(methodCtx, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, apperrors.Wrap(methodCtx, err)
	}

	applyEnvOverrides(&cfg)

	if err := cfg.validate(); err != nil {
		return Config{}, apperrors.Wrap(methodCtx, err)
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if value := strings.TrimSpace(os.Getenv("BASTYLE_DATABASE_DSN")); value != "" {
		cfg.Database.DSN = value
	}
	if value := strings.TrimSpace(os.Getenv("BASTYLE_RABBITMQ_URL")); value != "" {
		cfg.RabbitMQ.URL = value
	}
	if value := strings.TrimSpace(os.Getenv("BASTYLE_TELEGRAM_TOKEN")); value != "" {
		cfg.Telegram.Token = value
	}
}

func defaultConfig() Config {
	return Config{
		Telegram: Telegram{
			UpdateTimeoutSeconds: 60,
		},
		Workers: 5,
		Health: Health{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    8081,
		},
		Database: Database{
			DSN:               "postgres://bastyle:bastyle@bastyle-postgresql:5432/bastyle?sslmode=disable",
			MaxConns:          10,
			MinConns:          1,
			MaxConnLifetime:   Duration(time.Hour),
			MaxConnIdleTime:   Duration(15 * time.Minute),
			HealthCheckPeriod: Duration(30 * time.Second),
			ConnectTimeout:    Duration(5 * time.Second),
			StatementTimeout:  Duration(10 * time.Second),
			Migration: DatabaseMigration{
				Enabled: true,
			},
		},
		RabbitMQ: RabbitMQ{
			URL:               "amqp://guest:guest@bastyle-rabbitmq:5672/",
			Exchange:          "bastyle.events",
			ExchangeType:      "topic",
			PublishTimeout:    Duration(5 * time.Second),
			ReconnectInterval: Duration(5 * time.Second),
		},
		OutboxPublisher: OutboxPublisher{
			Enabled:        true,
			BatchSize:      50,
			PollInterval:   Duration(time.Second),
			IdleInterval:   Duration(5 * time.Second),
			LockTTL:        Duration(30 * time.Second),
			RetryBaseDelay: Duration(5 * time.Second),
			RetryMaxDelay:  Duration(10 * time.Minute),
			MaxAttempts:    20,
		},
		Consumers: Consumers{
			IndexEvents: Consumer{
				Enabled:          false,
				QueueTemplate:    "bastyle.replica.%s.events",
				RoutingKeys:      []string{"media.ban.#", "index.#"},
				Prefetch:         10,
				RetryDelay:       Duration(5 * time.Second),
				CatchUpInterval:  Duration(5 * time.Second),
				CatchUpBatchSize: 100,
			},
		},
		Metrics: Metrics{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    9090,
			Path:    "/metrics",
		},
		JobsBuffer:  100,
		MediaConfig: defaultMediaConfig(),
		Matching: Matching{
			Exact: Exact{
				Buffer: 500,
			},
			ImageHash: ImageHash{
				Threshold: 12,
				Buffer:    500,
			},
			VideoLike: VideoLike{
				Threshold:        12,
				Buffer:           500,
				MinMatchedFrames: 2,
				MinMatchedRatio:  0.4,
			},
			AIVector: AIVector{
				Enabled:          false,
				ModelName:        "nomic-ai/nomic-embed-vision-v1.5",
				ModelRevision:    "e3a725bce72db07ca4adb1d83da08903f3ee02f8",
				Device:           "cpu",
				IndexPath:        "faiss-image.index",
				MaxFiles:         10,
				Threshold:        0.92,
				TopK:             5,
				MinMatchedFrames: 2,
				MinMatchedRatio:  0.4,
				RequestTimeout:   Duration(10 * time.Second),
				Service: AIVectorService{
					Host: "127.0.0.1",
					Port: 8080,
				},
				HNSW: HNSW{
					M:              32,
					EFConstruction: 80,
					EFSearch:       64,
				},
			},
		},
	}
}

func defaultMediaConfig() MediaConfig {
	return MediaConfig{
		MaxFrames:               10,
		TargetWidth:             320,
		TargetHeight:            320,
		MaxUploadBytes:          ByteSize(20 << 20),
		MaxImagePixels:          4096 * 4096,
		MaxAnimationDuration:    Duration(10 * time.Second),
		MaxVideoStickerDuration: Duration(3 * time.Second),
		MaxAnimationSize:        ByteSize(20 << 20),
		MaxVideoStickerSize:     ByteSize(256 << 10),
		FFmpegBinary:            "ffmpeg",
		FFmpegTimeout:           Duration(10 * time.Second),
	}
}

func (c Config) validate() error {
	const methodCtx = "config/Config.validate"

	if c.Telegram.Token == "" {
		return apperrors.New(methodCtx, "токен Telegram обязателен")
	}
	if c.Telegram.UpdateTimeoutSeconds <= 0 {
		return apperrors.New(methodCtx, "таймаут обновлений Telegram должен быть положительным")
	}
	if c.Telegram.HTTPClient.Enabled && c.Telegram.HTTPClient.ProxyURL != "" {
		proxyURL, err := url.Parse(c.Telegram.HTTPClient.ProxyURL)
		if err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
		if proxyURL.Scheme == "" || proxyURL.Host == "" {
			return apperrors.New(methodCtx, "URL прокси HTTP-клиента Telegram должен содержать схему и хост")
		}
	}
	if c.Workers <= 0 {
		return apperrors.New(methodCtx, "количество воркеров должно быть положительным")
	}
	if c.Health.Enabled {
		if c.Health.Host == "" {
			return apperrors.New(methodCtx, "хост health-сервера обязателен")
		}
		if c.Health.Port <= 0 {
			return apperrors.New(methodCtx, "порт health-сервера должен быть положительным")
		}
	}
	if c.JobsBuffer <= 0 {
		return apperrors.New(methodCtx, "буфер задач должен быть положительным")
	}
	if err := c.Database.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if c.OutboxPublisher.Enabled || c.Consumers.IndexEvents.Enabled {
		if err := c.RabbitMQ.validate(); err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
	}
	if err := c.OutboxPublisher.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if err := c.Consumers.IndexEvents.validate("index_events"); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if err := c.Metrics.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if c.Matching.Exact.Buffer <= 0 {
		return apperrors.New(methodCtx, "буфер exact-матчера должен быть положительным")
	}
	if c.Matching.ImageHash.Threshold < 0 {
		return apperrors.New(methodCtx, "порог image-hash не должен быть отрицательным")
	}
	if c.Matching.ImageHash.Buffer <= 0 {
		return apperrors.New(methodCtx, "буфер image-hash должен быть положительным")
	}
	if err := c.MediaConfig.validate("настройки медиа"); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if err := c.Matching.AIVector.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if c.Matching.VideoLike.Threshold < 0 {
		return apperrors.New(methodCtx, "порог video-like не должен быть отрицательным")
	}
	if c.Matching.VideoLike.Buffer <= 0 {
		return apperrors.New(methodCtx, "буфер video-like должен быть положительным")
	}
	if err := validateFrameMatchRule("video-like matcher", c.Matching.VideoLike.MinMatchedFrames, c.Matching.VideoLike.MinMatchedRatio); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	return nil
}

func (c RabbitMQ) validate() error {
	const methodCtx = "config/RabbitMQ.validate"

	if c.URL == "" {
		return apperrors.New(methodCtx, "RabbitMQ URL обязателен")
	}
	if c.Exchange == "" {
		return apperrors.New(methodCtx, "RabbitMQ exchange обязателен")
	}
	switch c.ExchangeType {
	case "topic", "direct", "fanout", "headers":
	default:
		return apperrors.New(methodCtx, "тип RabbitMQ exchange должен быть topic, direct, fanout или headers")
	}
	if c.PublishTimeout.Value() <= 0 {
		return apperrors.New(methodCtx, "таймаут публикации RabbitMQ должен быть положительным")
	}
	if c.ReconnectInterval.Value() <= 0 {
		return apperrors.New(methodCtx, "интервал reconnect RabbitMQ должен быть положительным")
	}

	return nil
}

func (c OutboxPublisher) validate() error {
	const methodCtx = "config/OutboxPublisher.validate"

	if !c.Enabled {
		return nil
	}
	if c.BatchSize <= 0 {
		return apperrors.New(methodCtx, "размер пачки outbox publisher-а должен быть положительным")
	}
	if c.PollInterval.Value() <= 0 {
		return apperrors.New(methodCtx, "интервал опроса outbox publisher-а должен быть положительным")
	}
	if c.IdleInterval.Value() <= 0 {
		return apperrors.New(methodCtx, "idle-интервал outbox publisher-а должен быть положительным")
	}
	if c.LockTTL.Value() <= 0 {
		return apperrors.New(methodCtx, "TTL блокировки outbox publisher-а должен быть положительным")
	}
	if c.RetryBaseDelay.Value() <= 0 {
		return apperrors.New(methodCtx, "базовая задержка retry outbox publisher-а должна быть положительной")
	}
	if c.RetryMaxDelay.Value() < c.RetryBaseDelay.Value() {
		return apperrors.New(methodCtx, "максимальная задержка retry outbox publisher-а должна быть не меньше базовой")
	}
	if c.MaxAttempts <= 0 {
		return apperrors.New(methodCtx, "максимальное количество попыток outbox publisher-а должно быть положительным")
	}

	return nil
}

func (c Consumer) validate(name string) error {
	const methodCtx = "config/Consumer.validate"

	if !c.Enabled {
		return nil
	}
	if c.QueueTemplate == "" {
		return apperrors.New(methodCtx, name+": queue_template consumer-а обязателен")
	}
	if len(c.RoutingKeys) == 0 {
		return apperrors.New(methodCtx, name+": routing_keys consumer-а обязательны")
	}
	for _, routingKey := range c.RoutingKeys {
		if routingKey == "" {
			return apperrors.New(methodCtx, name+": routing key consumer-а не должен быть пустым")
		}
	}
	if c.Prefetch <= 0 {
		return apperrors.New(methodCtx, name+": prefetch consumer-а должен быть положительным")
	}
	if c.RetryDelay.Value() <= 0 {
		return apperrors.New(methodCtx, name+": retry delay consumer-а должен быть положительным")
	}
	if c.CatchUpInterval.Value() <= 0 {
		return apperrors.New(methodCtx, name+": catch-up interval consumer-а должен быть положительным")
	}
	if c.CatchUpBatchSize <= 0 {
		return apperrors.New(methodCtx, name+": catch-up batch size consumer-а должен быть положительным")
	}

	return nil
}

func (m Metrics) validate() error {
	const methodCtx = "config/Metrics.validate"

	if !m.Enabled {
		return nil
	}
	if m.Host == "" {
		return apperrors.New(methodCtx, "хост metrics-сервера обязателен")
	}
	if m.Port <= 0 {
		return apperrors.New(methodCtx, "порт metrics-сервера должен быть положительным")
	}
	if m.Path == "" || m.Path[0] != '/' {
		return apperrors.New(methodCtx, "путь metrics-сервера должен начинаться с /")
	}

	return nil
}

func (c Database) validate() error {
	const methodCtx = "config/Database.validate"

	if c.DSN == "" {
		return apperrors.New(methodCtx, "DSN базы данных обязателен")
	}
	if c.MaxConns <= 0 {
		return apperrors.New(methodCtx, "максимальное количество соединений с БД должно быть положительным")
	}
	if c.MinConns < 0 {
		return apperrors.New(methodCtx, "минимальное количество соединений с БД не должно быть отрицательным")
	}
	if c.MinConns > c.MaxConns {
		return apperrors.New(methodCtx, "минимальное количество соединений с БД не должно превышать максимальное")
	}
	if c.MaxConnLifetime.Value() <= 0 {
		return apperrors.New(methodCtx, "время жизни соединения с БД должно быть положительным")
	}
	if c.MaxConnIdleTime.Value() <= 0 {
		return apperrors.New(methodCtx, "время простоя соединения с БД должно быть положительным")
	}
	if c.HealthCheckPeriod.Value() <= 0 {
		return apperrors.New(methodCtx, "период проверки БД должен быть положительным")
	}
	if c.ConnectTimeout.Value() <= 0 {
		return apperrors.New(methodCtx, "таймаут подключения к БД должен быть положительным")
	}
	if c.StatementTimeout.Value() <= 0 {
		return apperrors.New(methodCtx, "таймаут SQL-запроса должен быть положительным")
	}

	return nil
}

func (c AIVector) validate() error {
	const methodCtx = "config/AIVector.validate"

	if !c.Enabled {
		return nil
	}
	if c.ModelName == "" {
		return apperrors.New(methodCtx, "имя модели AI-vector обязательно")
	}
	if c.ModelRevision == "" {
		return apperrors.New(methodCtx, "ревизия модели AI-vector обязательна")
	}
	if c.Device == "" {
		return apperrors.New(methodCtx, "устройство AI-vector обязательно")
	}
	if c.IndexPath == "" {
		return apperrors.New(methodCtx, "путь к индексу AI-vector обязателен")
	}
	if c.MaxFiles <= 0 {
		return apperrors.New(methodCtx, "максимальное количество файлов AI-vector должно быть положительным")
	}
	if c.Threshold < 0 || c.Threshold > 1 {
		return apperrors.New(methodCtx, "порог AI-vector должен быть от 0 до 1")
	}
	if c.TopK <= 0 {
		return apperrors.New(methodCtx, "AI-vector top_k должен быть положительным")
	}
	if err := validateFrameMatchRule("AI-vector", c.MinMatchedFrames, c.MinMatchedRatio); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if c.RequestTimeout.Value() <= 0 {
		return apperrors.New(methodCtx, "таймаут запроса к AI-vector должен быть положительным")
	}
	if c.Service.Host == "" {
		return apperrors.New(methodCtx, "хост AI-vector сервиса обязателен")
	}
	if c.Service.Port <= 0 {
		return apperrors.New(methodCtx, "порт AI-vector сервиса должен быть положительным")
	}
	if c.HNSW.M <= 0 {
		return apperrors.New(methodCtx, "параметр HNSW M для AI-vector должен быть положительным")
	}
	if c.HNSW.EFConstruction <= 0 {
		return apperrors.New(methodCtx, "параметр HNSW ef construction для AI-vector должен быть положительным")
	}
	if c.HNSW.EFSearch <= 0 {
		return apperrors.New(methodCtx, "параметр HNSW ef search для AI-vector должен быть положительным")
	}

	return nil
}

func validateFrameMatchRule(prefix string, minMatchedFrames int, minMatchedRatio float64) error {
	const methodCtx = "config/validateFrameMatchRule"

	if minMatchedFrames <= 0 {
		return apperrors.New(methodCtx, prefix+": минимальное количество совпавших кадров должно быть положительным")
	}
	if minMatchedRatio <= 0 || minMatchedRatio > 1 {
		return apperrors.New(methodCtx, prefix+": минимальная доля совпавших кадров должна быть от 0 до 1")
	}

	return nil
}

func (c MediaConfig) validate(prefix string) error {
	const methodCtx = "config/MediaConfig.validate"

	if c.MaxFrames <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальное количество кадров должно быть положительным")
	}
	if c.MaxFrames > maxMediaConfigFrames {
		return apperrors.New(methodCtx, prefix+": максимальное количество кадров слишком большое")
	}
	if c.TargetWidth <= 0 {
		return apperrors.New(methodCtx, prefix+": целевая ширина должна быть положительной")
	}
	if c.TargetWidth > maxMediaConfigTargetDimension {
		return apperrors.New(methodCtx, prefix+": целевая ширина слишком большая")
	}
	if c.TargetHeight <= 0 {
		return apperrors.New(methodCtx, prefix+": целевая высота должна быть положительной")
	}
	if c.TargetHeight > maxMediaConfigTargetDimension {
		return apperrors.New(methodCtx, prefix+": целевая высота слишком большая")
	}
	if c.MaxUploadBytes.Bytes() <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальный размер загрузки должен быть положительным")
	}
	if c.MaxImagePixels <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальное количество пикселей изображения должно быть положительным")
	}
	if c.MaxAnimationDuration.Value() <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальная длительность анимации должна быть положительной")
	}
	if c.MaxVideoStickerDuration.Value() <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальная длительность видеостикера должна быть положительной")
	}
	if c.MaxAnimationSize.Bytes() <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальный размер анимации должен быть положительным")
	}
	if c.MaxVideoStickerSize.Bytes() <= 0 {
		return apperrors.New(methodCtx, prefix+": максимальный размер видеостикера должен быть положительным")
	}
	if c.FFmpegBinary == "" {
		return apperrors.New(methodCtx, prefix+": бинарный файл ffmpeg обязателен")
	}
	if c.FFmpegTimeout.Value() <= 0 {
		return apperrors.New(methodCtx, prefix+": таймаут ffmpeg должен быть положительным")
	}

	return nil
}
