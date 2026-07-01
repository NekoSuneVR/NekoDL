// Package settings holds runtime-toggleable server policy — the kind of
// thing a user flips on/off from the dashboard without restarting the
// server, as opposed to config.Config's startup-time settings.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings is real policy enforced by the API, not just a UI preference:
// see addtorrent.go's use of RequireProxyForTorrents, which rejects a
// torrent task outright rather than merely warning about it.
type Settings struct {
	// AllowTorrents gates POST /api/v1/torrents entirely when false.
	AllowTorrents bool `json:"allow_torrents"`

	// RequireProxyForTorrents rejects a torrent task that has no
	// proxy_addr set, instead of letting it through with just a warning
	// (Task.Warning's "no proxy configured" notice still applies whenever
	// a torrent *is* allowed to run without one).
	RequireProxyForTorrents bool `json:"require_proxy_for_torrents"`
}

func Default() Settings {
	return Settings{
		AllowTorrents:           true,
		RequireProxyForTorrents: false,
	}
}

// Store holds the current Settings in memory, persisting to
// "settings.json" in dataDir on every change so they survive a restart.
type Store struct {
	path string

	mu      sync.RWMutex
	current Settings
}

func NewStore(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "settings.json"), current: Default()}

	loaded, err := s.load()
	if err != nil {
		return nil, err
	}
	if loaded != nil {
		s.current = *loaded
	}
	return s, nil
}

func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) Set(next Settings) error {
	s.mu.Lock()
	s.current = next
	s.mu.Unlock()
	return s.save(next)
}

func (s *Store) save(v Settings) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// load returns (nil, nil) if no settings file exists yet — that's the
// normal case on first run, not an error.
func (s *Store) load() (*Settings, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var v Settings
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
