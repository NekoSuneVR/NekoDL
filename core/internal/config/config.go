package config

import (
	"encoding/json"
	"fmt"
	"os"
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
}

func Default() Config {
	return Config{
		ListenAddr:             ":6900",
		DataDir:                "./data",
		LogLevel:               "info",
		APIToken:               "",
		MaxConcurrentDownloads: 5,
	}
}

// Load reads a JSON config file at path, falling back to defaults for any
// field left unset. A missing file is not an error — it just means defaults apply.
func Load(path string) (Config, error) {
	cfg := Default()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
