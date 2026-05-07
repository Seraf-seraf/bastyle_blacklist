package config

import (
	"bytes"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

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
	Buffer int `yaml:"buffer"`
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
	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}

	duration, err := time.ParseDuration(raw)
	if err != nil {
		return err
	}

	*d = Duration(duration)
	return nil
}

func (d *Duration) Value() time.Duration {
	return time.Duration(*d)
}

type ByteSize int64

func (s *ByteSize) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}

	size, err := humanize.ParseBytes(raw)
	if err != nil {
		return err
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
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
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
	if c.Telegram.Token == "" {
		return errors.New("telegram token is required")
	}
	if c.Telegram.UpdateTimeoutSeconds <= 0 {
		return errors.New("telegram update timeout must be positive")
	}
	if c.Telegram.HTTPClient.Enabled && c.Telegram.HTTPClient.ProxyURL != "" {
		proxyURL, err := url.Parse(c.Telegram.HTTPClient.ProxyURL)
		if err != nil {
			return err
		}
		if proxyURL.Scheme == "" || proxyURL.Host == "" {
			return errors.New("telegram http client proxy url must include scheme and host")
		}
	}
	if c.Workers <= 0 {
		return errors.New("workers must be positive")
	}
	if c.Health.Enabled {
		if c.Health.Host == "" {
			return errors.New("health host is required")
		}
		if c.Health.Port <= 0 {
			return errors.New("health port must be positive")
		}
	}
	if c.JobsBuffer <= 0 {
		return errors.New("jobs buffer must be positive")
	}
	if c.Matching.Exact.Buffer <= 0 {
		return errors.New("exact matcher buffer must be positive")
	}
	if c.Matching.ImageHash.DBPath == "" {
		return errors.New("image hash db path is required")
	}
	if c.Matching.ImageHash.Threshold < 0 {
		return errors.New("image hash threshold must not be negative")
	}
	if c.Matching.ImageHash.Buffer <= 0 {
		return errors.New("image hash buffer must be positive")
	}
	if err := c.MediaConfig.validate("media config"); err != nil {
		return err
	}
	if err := c.Matching.AIVector.validate(); err != nil {
		return err
	}
	if c.Matching.VideoLike.DBPath == "" {
		return errors.New("video like db path is required")
	}
	if c.Matching.VideoLike.Threshold < 0 {
		return errors.New("video like threshold must not be negative")
	}
	if c.Matching.VideoLike.Buffer <= 0 {
		return errors.New("video like buffer must be positive")
	}
	if err := validateFrameMatchRule("video like", c.Matching.VideoLike.MinMatchedFrames, c.Matching.VideoLike.MinMatchedRatio); err != nil {
		return err
	}
	return nil
}

func (c AIVector) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.ModelName == "" {
		return errors.New("ai vector model name is required")
	}
	if c.ModelRevision == "" {
		return errors.New("ai vector model revision is required")
	}
	if c.Device == "" {
		return errors.New("ai vector device is required")
	}
	if c.DBPath == "" {
		return errors.New("ai vector db path is required")
	}
	if c.IndexPath == "" {
		return errors.New("ai vector index path is required")
	}
	if c.MaxFiles <= 0 {
		return errors.New("ai vector max files must be positive")
	}
	if c.Threshold < 0 || c.Threshold > 1 {
		return errors.New("ai vector threshold must be between 0 and 1")
	}
	if c.TopK <= 0 {
		return errors.New("ai vector top k must be positive")
	}
	if err := validateFrameMatchRule("ai vector", c.MinMatchedFrames, c.MinMatchedRatio); err != nil {
		return err
	}
	if c.RequestTimeout.Value() <= 0 {
		return errors.New("ai vector request timeout must be positive")
	}
	if c.Service.Host == "" {
		return errors.New("ai vector service host is required")
	}
	if c.Service.Port <= 0 {
		return errors.New("ai vector service port must be positive")
	}
	if c.HNSW.M <= 0 {
		return errors.New("ai vector hnsw m must be positive")
	}
	if c.HNSW.EFConstruction <= 0 {
		return errors.New("ai vector hnsw ef construction must be positive")
	}
	if c.HNSW.EFSearch <= 0 {
		return errors.New("ai vector hnsw ef search must be positive")
	}

	return nil
}

func validateFrameMatchRule(prefix string, minMatchedFrames int, minMatchedRatio float64) error {
	if minMatchedFrames <= 0 {
		return errors.New(prefix + " min matched frames must be positive")
	}
	if minMatchedRatio <= 0 || minMatchedRatio > 1 {
		return errors.New(prefix + " min matched ratio must be between 0 and 1")
	}

	return nil
}

func (c MediaConfig) validate(prefix string) error {
	if c.MaxFrames <= 0 {
		return errors.New(prefix + " max frames must be positive")
	}
	if c.MaxFrames > maxMediaConfigFrames {
		return errors.New(prefix + " max frames is too large")
	}
	if c.TargetWidth <= 0 {
		return errors.New(prefix + " target width must be positive")
	}
	if c.TargetWidth > maxMediaConfigTargetDimension {
		return errors.New(prefix + " target width is too large")
	}
	if c.TargetHeight <= 0 {
		return errors.New(prefix + " target height must be positive")
	}
	if c.TargetHeight > maxMediaConfigTargetDimension {
		return errors.New(prefix + " target height is too large")
	}
	if c.MaxUploadBytes.Bytes() <= 0 {
		return errors.New(prefix + " max upload bytes must be positive")
	}
	if c.MaxImagePixels <= 0 {
		return errors.New(prefix + " max image pixels must be positive")
	}
	if c.MaxAnimationDuration.Value() <= 0 {
		return errors.New(prefix + " max animation duration must be positive")
	}
	if c.MaxVideoStickerDuration.Value() <= 0 {
		return errors.New(prefix + " max video sticker duration must be positive")
	}
	if c.MaxAnimationSize.Bytes() <= 0 {
		return errors.New(prefix + " max animation size must be positive")
	}
	if c.MaxVideoStickerSize.Bytes() <= 0 {
		return errors.New(prefix + " max video sticker size must be positive")
	}
	if c.FFmpegBinary == "" {
		return errors.New(prefix + " ffmpeg binary is required")
	}
	if c.FFmpegTimeout.Value() <= 0 {
		return errors.New(prefix + " ffmpeg timeout must be positive")
	}

	return nil
}
