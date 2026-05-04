package config

import (
	"os"
	"path/filepath"
	"testing"
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

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	return path
}
