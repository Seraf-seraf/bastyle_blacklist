package config

import (
	"bytes"
	"net"
	"net/url"
	"os"
	"strconv"
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
	Telegram Telegram `yaml:"telegram"`
	Workers  int      `yaml:"workers"`
	Health   Health   `yaml:"health"`

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

type Matching struct {
	Exact     Exact     `yaml:"exact"`
	ImageHash ImageHash `yaml:"image_hash"`
	VideoLike VideoLike `yaml:"video_like"`
	AIVector  AIVector  `yaml:"ai_vector"`
}

type Exact struct {
	DBPath string `yaml:"db_path"`
	Buffer int    `yaml:"buffer"`
}

type ImageHash struct {
	DBPath    string `yaml:"db_path"`
	Threshold int    `yaml:"threshold"`
	Buffer    int    `yaml:"buffer"`
}

type VideoLike struct {
	DBPath           string  `yaml:"db_path"`
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
	DBPath           string          `yaml:"db_path"`
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

	if err := cfg.validate(); err != nil {
		return Config{}, apperrors.Wrap(methodCtx, err)
	}

	return cfg, nil
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
		JobsBuffer:  100,
		MediaConfig: defaultMediaConfig(),
		Matching: Matching{
			Exact: Exact{
				DBPath: "bastyle.sqlite",
				Buffer: 500,
			},
			ImageHash: ImageHash{
				DBPath:    "bastyle.sqlite",
				Threshold: 12,
				Buffer:    500,
			},
			VideoLike: VideoLike{
				DBPath:           "bastyle.sqlite",
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
				DBPath:           "bastyle.sqlite",
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
	if c.Matching.Exact.DBPath == "" {
		return apperrors.New(methodCtx, "путь к БД exact обязателен")
	}
	if c.Matching.Exact.Buffer <= 0 {
		return apperrors.New(methodCtx, "буфер exact-матчера должен быть положительным")
	}
	if c.Matching.ImageHash.DBPath == "" {
		return apperrors.New(methodCtx, "путь к БД image-hash обязателен")
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
	if c.Matching.VideoLike.DBPath == "" {
		return apperrors.New(methodCtx, "путь к БД video-like обязателен")
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
	if c.DBPath == "" {
		return apperrors.New(methodCtx, "путь к БД AI-vector обязателен")
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
