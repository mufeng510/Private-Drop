package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"private-drop/internal/util"
)

var (
	ErrInvalidToken  = errors.New("invalid or expired token")
	ErrDeviceRevoked = errors.New("device revoked")
)

// Principal is the authenticated identity extracted from a bearer token.
type Principal struct {
	Token      string
	UserID     string
	Username   string
	DeviceID   string
	DeviceName string
	DeviceType string
}

// CreateSession stores a hashed session token. A ttl <= 0 creates a session
// that never expires (still remotely revocable).
func (s *Service) CreateSession(ctx context.Context, userID, deviceID string, ttl time.Duration) (string, error) {
	token := util.RandomToken()
	var expires any
	if ttl > 0 {
		expires = time.Now().UTC().Add(ttl).Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions(id,device_id,user_id,expires_at,created_at) VALUES(?,?,?,?,?)`,
		util.HashToken(token), deviceID, userID, expires, util.Now())
	if err != nil {
		return "", err
	}
	return token, nil
}

// Authenticate validates a token and returns the associated principal.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	var p Principal
	var expires sql.NullString
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `
        SELECT s.user_id, s.device_id, s.expires_at, u.username, d.name, d.type, d.revoked_at
        FROM sessions s
        JOIN users u ON u.id = s.user_id
        JOIN devices d ON d.id = s.device_id
        WHERE s.id = ?`, util.HashToken(token)).Scan(
		&p.UserID, &p.DeviceID, &expires, &p.Username, &p.DeviceName, &p.DeviceType, &revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if revoked.Valid {
		return nil, ErrDeviceRevoked
	}
	if expires.Valid && expires.String != "" {
		t, err := util.ParseTime(expires.String)
		if err != nil || !time.Now().UTC().Before(t) {
			return nil, ErrInvalidToken
		}
	}
	p.Token = token
	return &p, nil
}

func (s *Service) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, util.HashToken(token))
	return err
}

func (s *Service) DeleteDeviceSessions(ctx context.Context, deviceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE device_id=?`, deviceID)
	return err
}

func (s *Service) DeleteAllSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions`)
	return err
}

// IssueMFA creates a short-lived token proving the admin completed secondary
// verification. It is stored hashed and tied to the current device.
func (s *Service) IssueMFA(ctx context.Context, userID, deviceID string) (string, error) {
	token := util.RandomToken()
	expires := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_tokens(token_hash,user_id,device_id,expires_at,created_at) VALUES(?,?,?,?,?)`,
		util.HashToken(token), userID, deviceID, expires, util.Now())
	if err != nil {
		return "", err
	}
	return token, nil
}

// IssueBootstrapMFA grants an admin-verification token before the user has
// enrolled a second factor. Without this, a fresh installation could never
// open the admin UI to configure TOTP/Passkey (a bootstrap deadlock).
// The grant is long-lived enough to finish setup, and is no longer issued on
// login once a second factor exists.
func (s *Service) IssueBootstrapMFA(ctx context.Context, userID, deviceID string) (string, error) {
	token := util.RandomToken()
	expires := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_tokens(token_hash,user_id,device_id,expires_at,created_at) VALUES(?,?,?,?,?)`,
		util.HashToken(token), userID, deviceID, expires, util.Now())
	return token, err
}

// ValidateMFA checks a secondary-verification token. The token is bound to
// the device it was issued for, so a stolen pair cannot be replayed elsewhere.
func (s *Service) ValidateMFA(ctx context.Context, token, userID, deviceID string) bool {
	if token == "" {
		return false
	}
	var expires string
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at FROM mfa_tokens WHERE token_hash=? AND user_id=? AND device_id=?`,
		util.HashToken(token), userID, deviceID).Scan(&expires)
	if err != nil {
		return false
	}
	t, err := util.ParseTime(expires)
	return err == nil && time.Now().UTC().Before(t)
}

// RevokeMFAForDevice removes all secondary-verification grants for a device.
func (s *Service) RevokeMFAForDevice(ctx context.Context, userID, deviceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mfa_tokens WHERE user_id=? AND device_id=?`, userID, deviceID)
	return err
}

// CleanupExpiredSessions removes expired sessions and MFA grants.
func (s *Service) CleanupExpiredSessions(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at IS NOT NULL AND expires_at < ?`, now); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM mfa_tokens WHERE expires_at < ?`, now)
	return err
}
