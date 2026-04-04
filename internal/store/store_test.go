package store

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))

	if err := store.Save(Config{LastWatchRowID: 42}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.LastWatchRowID != 42 {
		t.Fatalf("expected row id 42, got %d", cfg.LastWatchRowID)
	}
}
