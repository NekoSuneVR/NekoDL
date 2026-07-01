package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Config holds NekoDL core settings, loaded from a JSON file (see nekodl.example.json).
type Config struct {
	ListenAddr string `json:"listen_addr"`
	DataDir    string `json:"data_dir"`
	LogLevel   string `json:"log_level"`

	// APIToken, if set, must be presented as "Authorization: Bearer <token>"
	// on every API request. Leaving it empty disables auth — fine for local
	// development, not for anything exposed beyond localhost.
	APIToken string `json:"api_token"`

	// MaxConcurrentDownloads caps how many tasks the scheduler runs at once,
	// across all engines. 0 means unlimited.
	MaxConcurrentDownloads int `json:"max_concurrent_downloads"`

	// StaticDir, if set, serves the built web dashboard (web/dist) from
	// this directory at "/". Empty disables static serving — the API still
	// works standalone. Set by the Docker image; not needed for `go run`
	// during core-only development.
	StaticDir string `json:"static_dir"`

	// BoothBinaryPath points at a BoothDownloader executable
	// (github.com/Myrkie/BoothDownloader). Empty disables the Booth engine
	// entirely (POST /api/v1/booth returns an error) — unlike yt-dlp, there's
	// no pip-style canonical install location to fall back to guessing.
	BoothBinaryPath string `json:"booth_binary_path"`
}

func Default() Config {
	return Config{
		ListenAddr:             ":6900",
		DataDir:                "./data",
		LogLevel:               "info",
		APIToken:               "",
		MaxConcurrentDownloads: 5,
		StaticDir:              "",
		BoothBinaryPath:        "",
	}
}

// Load reads a JSON config file at path, falling back to defaults for any
// field left unset, then applies NEKODL_* environment variable overrides —
// handy for Docker (e.g. `docker run -e NEKODL_API_TOKEN=...`) without
// rebuilding the image or mounting a custom config file. Precedence:
// environment variables > config file > defaults. A missing config file is
// not an error — it just means defaults (plus any env overrides) apply.
func Load(path string) (Config, error) {
	cfg := Default()

	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("open config: %w", err)
		}
	} else {
		defer f.Close()
		if err := json.NewDecoder(f).Decode(&cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	applyEnvOverrides(&cfg)
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("NEKODL_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("NEKODL_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("NEKODL_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("NEKODL_API_TOKEN"); v != "" {
		cfg.APIToken = v
	}
	if v := os.Getenv("NEKODL_STATIC_DIR"); v != "" {
		cfg.StaticDir = v
	}
	if v := os.Getenv("NEKODL_MAX_CONCURRENT_DOWNLOADS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxConcurrentDownloads = n
		}
	}
	if v := os.Getenv("NEKODL_BOOTH_BINARY_PATH"); v != "" {
		cfg.BoothBinaryPath = v
	}
}
