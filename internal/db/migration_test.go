package db_test

import (
	"testing"

	"private-drop/internal/db"
	"private-drop/internal/testutil"
)

func TestCompletionMigrationIsReversible(t *testing.T) {
	d, _ := testutil.NewDB(t) // applies all up migrations

	var wsEvents int
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ws_events'`).Scan(&wsEvents); err != nil || wsEvents != 1 {
		t.Fatalf("ws_events table missing after migrate: count=%d err=%v", wsEvents, err)
	}
	if !hasColumn(t, d, "uploads", "original_path") || !hasColumn(t, d, "attachments", "original_path") {
		t.Fatal("original_path columns missing after migrate")
	}

	if err := d.Down(); err != nil {
		t.Fatalf("rollback last migration: %v", err)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ws_events'`).Scan(&wsEvents); err != nil || wsEvents != 0 {
		t.Fatalf("ws_events should be dropped after rollback: count=%d err=%v", wsEvents, err)
	}
	if hasColumn(t, d, "uploads", "original_path") || hasColumn(t, d, "attachments", "original_path") {
		t.Fatal("original_path columns should be dropped after rollback")
	}

	// Re-applying is idempotent and restores the schema.
	if err := d.Migrate(); err != nil {
		t.Fatalf("re-migrate after rollback: %v", err)
	}
}

func hasColumn(t *testing.T, d *db.DB, table, column string) bool {
	t.Helper()
	rows, err := d.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	return false
}
