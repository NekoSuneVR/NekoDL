package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Store persists task Records to a JSON file so the task list survives a
// restart. It only persists metadata (ID, engine, status, progress) — it
// does not know how to reconstruct a live task.Task for a real engine.
// Re-attaching in-flight transfers after a restart is an engine-level
// concern for later phases; for now, restored records are read-only history.
type Store struct {
	path string
}

// NewStore returns a Store that reads/writes "tasks.json" inside dataDir.
func NewStore(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "tasks.json")}
}

func (s *Store) Save(records []Record) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Load returns the last saved records, or nil if no snapshot exists yet.
func (s *Store) Load() ([]Record, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}
