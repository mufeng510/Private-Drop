// Package testutil provides shared setup helpers for backend tests.
package testutil

import (
	"testing"

	"private-drop/internal/config"
	"private-drop/internal/db"
)

// NewDB creates an isolated, migrated SQLite database in a temp directory.
func NewDB(t *testing.T) (*db.DB, config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{Port: "8080", DataDir: dir}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	database, err := db.Open(cfg.DBPath())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, cfg
}
