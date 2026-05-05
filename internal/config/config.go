package config

import (
	"bytes"
	"errors"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"gopkg.in/yaml.v3"
)

const (
	maxVideoLikeFrames          = 20
	maxVideoLikeTargetDimension = 1024
)

type Config struct {
	Telegram Telegram `yaml:"telegram"`
	Workers  int      `yaml:"workers"`

	JobsBuffer int      `yaml:"jobs_buffer"`
	Matching   Matching `yaml:"matching"`
}

type Telegram struct {
	Token                string `yaml:"token"`
	UpdateTimeoutSeconds int    `yaml:"update_timeout_seconds"`
}

type Matching struct {
	Exact     Exact     `yaml:"exact"`
	ImageHash ImageHash `yaml:"image_hash"`
	VideoLike VideoLike `yaml:"video_like"`
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
	Enabled                 bool     `yaml:"enabled"`
	MaxAnimationDuration    Duration `yaml:"max_animation_duration"`
	MaxVideoStickerDuration Duration `yaml:"max_video_sticker_duration"`
	MaxAnimationSize        ByteSize `yaml:"max_animation_size"`
	MaxVideoStickerSize     ByteSize `yaml:"max_video_sticker_size"`
	DBPath                  string   `yaml:"db_path"`
	Threshold               int      `yaml:"threshold"`
	Buffer                  int      `yaml:"buffer"`
	MinMatchedFrames        int      `yaml:"min_matched_frames"`
	MinMatchedRatio         float64  `yaml:"min_matched_ratio"`
	MaxFrames               int      `yaml:"max_frames"`
	TargetWidth             int      `yaml:"target_width"`
	TargetHeight            int      `yaml:"target_height"`
	FFmpegBinary            string   `yaml:"ffmpeg_binary"`
	FFmpegTimeout           Duration `yaml:"ffmpeg_timeout"`
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

func (d Duration) Value() time.Duration {
	return time.Duration(d)
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

func (s ByteSize) Bytes() int64 {
	return int64(s)
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
		Workers:    5,
		JobsBuffer: 100,
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
				Enabled:                 true,
				MaxAnimationDuration:    Duration(10 * time.Second),
				MaxVideoStickerDuration: Duration(3 * time.Second),
				MaxAnimationSize:        ByteSize(20 << 20),
				MaxVideoStickerSize:     ByteSize(256 << 10),
				DBPath:                  "bastyle.sqlite",
				Threshold:               12,
				Buffer:                  500,
				MinMatchedFrames:        2,
				MinMatchedRatio:         0.4,
				MaxFrames:               10,
				TargetWidth:             320,
				TargetHeight:            320,
				FFmpegBinary:            "ffmpeg",
				FFmpegTimeout:           Duration(10 * time.Second),
			},
		},
	}
}

func (c Config) validate() error {
	if c.Telegram.Token == "" {
		return errors.New("telegram token is required")
	}
	if c.Telegram.UpdateTimeoutSeconds <= 0 {
		return errors.New("telegram update timeout must be positive")
	}
	if c.Workers <= 0 {
		return errors.New("workers must be positive")
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
	if !c.Matching.VideoLike.Enabled {
		return nil
	}
	if c.Matching.VideoLike.MaxAnimationDuration.Value() <= 0 {
		return errors.New("video like max animation duration must be positive")
	}
	if c.Matching.VideoLike.MaxVideoStickerDuration.Value() <= 0 {
		return errors.New("video like max video sticker duration must be positive")
	}
	if c.Matching.VideoLike.MaxAnimationSize.Bytes() <= 0 {
		return errors.New("video like max animation size must be positive")
	}
	if c.Matching.VideoLike.MaxVideoStickerSize.Bytes() <= 0 {
		return errors.New("video like max video sticker size must be positive")
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
	if c.Matching.VideoLike.MinMatchedFrames <= 0 {
		return errors.New("video like min matched frames must be positive")
	}
	if c.Matching.VideoLike.MinMatchedRatio <= 0 || c.Matching.VideoLike.MinMatchedRatio > 1 {
		return errors.New("video like min matched ratio must be between 0 and 1")
	}
	if c.Matching.VideoLike.MaxFrames <= 0 {
		return errors.New("video like max frames must be positive")
	}
	if c.Matching.VideoLike.MaxFrames > maxVideoLikeFrames {
		return errors.New("video like max frames is too large")
	}
	if c.Matching.VideoLike.TargetWidth <= 0 {
		return errors.New("video like target width must be positive")
	}
	if c.Matching.VideoLike.TargetWidth > maxVideoLikeTargetDimension {
		return errors.New("video like target width is too large")
	}
	if c.Matching.VideoLike.TargetHeight <= 0 {
		return errors.New("video like target height must be positive")
	}
	if c.Matching.VideoLike.TargetHeight > maxVideoLikeTargetDimension {
		return errors.New("video like target height is too large")
	}
	if c.Matching.VideoLike.FFmpegBinary == "" {
		return errors.New("video like ffmpeg binary is required")
	}
	if c.Matching.VideoLike.FFmpegTimeout.Value() <= 0 {
		return errors.New("video like ffmpeg timeout must be positive")
	}

	return nil
}
