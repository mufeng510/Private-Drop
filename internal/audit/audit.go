// Package audit records security-relevant actions and application logs.
// It deliberately never stores secrets, tokens or file contents.
package audit

import (
	"context"
	"fmt"
	"time"

	"private-drop/internal/db"
	"private-drop/internal/util"
)

type Service struct{ db *db.DB }

func New(d *db.DB) *Service { return &Service{db: d} }

type Entry struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
	DeviceID  string `json:"deviceId"`
	CreatedAt string `json:"createdAt"`
}

// Log writes a structured audit entry.
func (s *Service) Log(ctx context.Context, action, detail, ip, deviceID string) {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs(action,detail,ip,device_id,created_at) VALUES(?,?,?,?,?)`,
		action, detail, ip, deviceID, util.Now()); err != nil {
		// Auditing must never break the main operation; mirror to stderr.
		fmt.Printf("audit: failed to write %q: %v\n", action, err)
	}
}

// AppLog writes a system log line of the given category
// (app|upload|clean|error|security).
func (s *Service) AppLog(ctx context.Context, typ, message string) {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO logs(type,message,created_at) VALUES(?,?,?)`,
		typ, message, util.Now()); err != nil {
		fmt.Printf("log: failed to write %q: %v\n", typ, err)
	}
}

type LogEntry struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

func (s *Service) Logs(ctx context.Context, typ string, limit int) ([]LogEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id,type,CASE type WHEN 'error' THEN 'error' WHEN 'security' THEN 'warn' ELSE 'info' END,message,created_at FROM logs`
	var args []any
	if typ != "" {
		q += ` WHERE type=?`
		args = append(args, typ)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.Level, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) Audit(ctx context.Context, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,action,detail,ip,device_id,created_at FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Action, &e.Detail, &e.IP, &e.DeviceID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) Cleanup(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM logs WHERE created_at < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	return err
}
