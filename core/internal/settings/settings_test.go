package settings

import (
	"path/filepath"
	"testing"
)

func TestNewStoreDefaultsWhenNoFileExists(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store.Get() != Default() {
		t.Fatalf("expected defaults, got %+v", store.Get())
	}
}

func TestSetPersistsAcrossNewStore(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	want := Settings{AllowTorrents: false, RequireProxyForTorrents: true}
	if err := store.Set(want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore (reload): %v", err)
	}
	if reloaded.Get() != want {
		t.Fatalf("got %+v, want %+v", reloaded.Get(), want)
	}
}

func TestSetOverwritesPreviousValue(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Set(Settings{AllowTorrents: false}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(Settings{AllowTorrents: true, RequireProxyForTorrents: true}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := store.Get()
	if !got.AllowTorrents || !got.RequireProxyForTorrents {
		t.Fatalf("expected the second Set to fully replace the first, got %+v", got)
	}
}

func TestNewStoreCreatesDataDirIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "does-not-exist-yet")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Set(Settings{AllowTorrents: true}); err != nil {
		t.Fatalf("Set: %v", err)
	}
}
