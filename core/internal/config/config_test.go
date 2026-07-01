package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("expected defaults for a missing file, got %+v", cfg)
	}
}

func TestLoadReadsConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nekodl.json")
	if err := os.WriteFile(path, []byte(`{"listen_addr":":9999","api_token":"filetoken"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":9999" || cfg.APIToken != "filetoken" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	// Fields not present in the file should keep their defaults.
	if cfg.MaxConcurrentDownloads != Default().MaxConcurrentDownloads {
		t.Fatalf("expected default MaxConcurrentDownloads, got %d", cfg.MaxConcurrentDownloads)
	}
}

func TestEnvOverridesTakePrecedenceOverFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nekodl.json")
	if err := os.WriteFile(path, []byte(`{"api_token":"filetoken","max_concurrent_downloads":3}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("NEKODL_API_TOKEN", "envtoken")
	t.Setenv("NEKODL_MAX_CONCURRENT_DOWNLOADS", "9")
	t.Setenv("NEKODL_LISTEN_ADDR", ":1234")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIToken != "envtoken" {
		t.Fatalf("expected env override for APIToken, got %q", cfg.APIToken)
	}
	if cfg.MaxConcurrentDownloads != 9 {
		t.Fatalf("expected env override for MaxConcurrentDownloads, got %d", cfg.MaxConcurrentDownloads)
	}
	if cfg.ListenAddr != ":1234" {
		t.Fatalf("expected env override for ListenAddr, got %q", cfg.ListenAddr)
	}
}

func TestEnvOverrideIgnoresInvalidInt(t *testing.T) {
	t.Setenv("NEKODL_MAX_CONCURRENT_DOWNLOADS", "not-a-number")

	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MaxConcurrentDownloads != Default().MaxConcurrentDownloads {
		t.Fatalf("expected default to survive an invalid env override, got %d", cfg.MaxConcurrentDownloads)
	}
}
