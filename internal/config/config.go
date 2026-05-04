package config

import (
	"bytes"
	"errors"
	"os"

	"gopkg.in/yaml.v3"
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
}

type Exact struct {
	Buffer int `yaml:"buffer"`
}

type ImageHash struct {
	DBPath    string `yaml:"db_path"`
	Threshold int    `yaml:"threshold"`
	Buffer    int    `yaml:"buffer"`
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

	return nil
}
