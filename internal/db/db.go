// Package db wraps the SQLite connection, applies ordered migrations and
// provides small helpers used by the domain packages.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"private-drop/migrations"

	_ "modernc.org/sqlite"
)

// DB is a thin wrapper around *sql.DB.
type DB struct {
	*sql.DB
	Path string
}

// Open opens (or creates) the SQLite database and configures it for a
// long-running single-writer workload: WAL journaling, foreign keys and a
// generous busy timeout.
func Open(path string) (*DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	u.RawQuery = q.Encode()

	raw, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	// A small pool avoids write-lock contention while still allowing nested
	// read queries. All writers are serialized by SQLite + busy_timeout.
	raw.SetMaxOpenConns(4)
	raw.SetMaxIdleConns(4)
	raw.SetConnMaxLifetime(0)

	d := &DB{DB: raw, Path: abs}
	if err := d.Ping(); err != nil {
		raw.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the database and checkpoints WAL so the file is consistent for
// NAS backups.
func (d *DB) Close() error {
	if _, err := d.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = d.DB.Close()
		return err
	}
	return d.DB.Close()
}

type migration struct {
	version string
	up      string
	down    string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	byVersion := map[string]*migration{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".sql")
		var suffix string
		var version string
		switch {
		case strings.HasSuffix(name, ".up"):
			suffix, version = "up", strings.TrimSuffix(name, ".up")
		case strings.HasSuffix(name, ".down"):
			suffix, version = "down", strings.TrimSuffix(name, ".down")
		default:
			continue
		}
		m := byVersion[version]
		if m == nil {
			m = &migration{version: version}
			byVersion[version] = m
		}
		b, err := migrations.FS.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		if suffix == "up" {
			m.up = string(b)
		} else {
			m.down = string(b)
		}
	}
	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.up == "" {
			return nil, fmt.Errorf("migration %s has no up file", m.version)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func (d *DB) ensureMigrationTable() error {
	_, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`)
	return err
}

// Migrate applies all pending up migrations in version order. Each migration
// runs in its own transaction so a failure leaves no partial schema changes.
func (d *DB) Migrate() error {
	if err := d.ensureMigrationTable(); err != nil {
		return err
	}
	list, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range list {
		var n int
		if err := d.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=?`, m.version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if err := d.execMigration(m.up, m.version, false); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
	}
	return nil
}

// Down rolls back the last applied migration (used for manual rollback).
func (d *DB) Down() error {
	if err := d.ensureMigrationTable(); err != nil {
		return err
	}
	list, err := loadMigrations()
	if err != nil {
		return err
	}
	for i := len(list) - 1; i >= 0; i-- {
		m := list[i]
		var n int
		if err := d.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=?`, m.version).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		if m.down == "" {
			return fmt.Errorf("migration %s has no down file", m.version)
		}
		if err := d.execMigration(m.down, m.version, true); err != nil {
			return fmt.Errorf("rollback migration %s: %w", m.version, err)
		}
		return nil
	}
	return errors.New("no applied migrations to roll back")
}

func (d *DB) execMigration(sqlText, version string, down bool) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(sqlText); err != nil {
		return err
	}
	if down {
		if _, err := tx.Exec(`DELETE FROM schema_migrations WHERE version=?`, version); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?,?)`, version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// WithTx runs fn inside a transaction, committing on success and rolling back
// on any error (including panics).
func (d *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Bool converts a SQLite integer to a Go bool.
func Bool(v int64) bool { return v != 0 }

// IntBool converts a Go bool to a SQLite integer.
func IntBool(v bool) int {
	if v {
		return 1
	}
	return 0
}
