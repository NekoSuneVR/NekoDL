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
}

func Default() Config {
	return Config{
		ListenAddr: ":6900",
		DataDir:    "./data",
		LogLevel:   "info",
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
