// Package auth implements authentication, sessions, devices, TOTP and
// WebAuthn passkeys for the single-admin Private Drop instance.
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/db"
	"private-drop/internal/events"
	"private-drop/internal/notify"
	"private-drop/internal/util"
)

var (
	ErrAlreadyInitialized = errors.New("already initialized")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrMFARequired        = errors.New("mfa required")
	ErrRateLimited        = errors.New("too many login attempts")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrUsernameRequired   = errors.New("username required")
)

// DeviceActionRequired is returned by Login when the device policy requires
// the admin to authorize the new device before a session is issued.
type DeviceActionRequired struct{ Decision DeviceDecision }

func (e *DeviceActionRequired) Error() string {
	if e.Decision.Status == "denied" {
		return "device not allowed by policy"
	}
	return "device approval required"
}

// User is the public representation of the single admin account.
type User struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	CreatedAt  string `json:"createdAt"`
	HasTOTP    bool   `json:"hasTotp"`
	HasPasskey bool   `json:"hasPasskey"`

	totpSecret *string // internal only, never serialized
}

type Service struct {
	db      *db.DB
	cfg     config.Config
	notify  *notify.Service
	audit   *audit.Service
	events  events.Sink
	limiter *rateLimiter
}

func New(d *db.DB, cfg config.Config, sink events.Sink, n *notify.Service, a *audit.Service) *Service {
	return &Service{db: d, cfg: cfg, notify: n, audit: a, events: sink, limiter: newRateLimiter()}
}

// DecodeJSON reads a bounded JSON body into dst and restores r.Body so the
// same request can be decoded again (needed by the WebAuthn ceremony).
func DecodeJSON(r *http.Request, dst any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	return json.NewDecoder(bytes.NewReader(b)).Decode(dst)
}

// IsPrivateIP reports whether an IP belongs to a LAN/loopback/link-local
// range. Used to restrict the initialization endpoint to LAN clients.
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// IsInitialized reports whether the admin account has been created.
func (s *Service) IsInitialized(ctx context.Context) bool {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='initialized'`).Scan(&v); err != nil {
		return false
	}
	return strings.EqualFold(v, "true")
}

type InitResult struct {
	Token    string `json:"token"`
	User     User   `json:"user"`
	Device   Device `json:"device"`
	MFAToken string `json:"mfaToken,omitempty"`
}

// Init creates the single admin account. It is a no-op once initialized.
func (s *Service) Init(ctx context.Context, username, password, deviceName, policy, ua, ip, clientKey string) (InitResult, error) {
	var res InitResult
	if s.IsInitialized(ctx) {
		return res, ErrAlreadyInitialized
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return res, ErrUsernameRequired
	}
	if len(password) < 8 {
		return res, ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return res, err
	}
	handle := make([]byte, 32)
	if _, err := rand.Read(handle); err != nil {
		return res, err
	}
	u := User{ID: util.NewID(), Username: username, CreatedAt: util.Now()}
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Re-check inside the transaction: the pre-transaction IsInitialized
		// check alone is a TOCTOU race between concurrent init requests.
		var flag string
		if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='initialized'`).Scan(&flag); err != nil {
			return err
		}
		if strings.EqualFold(flag, "true") {
			return ErrAlreadyInitialized
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users(id,username,password_hash,webauthn_id,created_at) VALUES(?,?,?,?,?)`,
			u.ID, u.Username, hash, handle, u.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE settings SET value='true' WHERE key='initialized'`); err != nil {
			return err
		}
		if policy != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE settings SET value=? WHERE key='device_policy'`, policy); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	// The first device is always authorized so the admin cannot lock themselves out.
	dev, _, err := s.RegisterDevice(ctx, u.ID, deviceName, ua, ip, "open", clientKey)
	if err != nil {
		return res, err
	}
	token, err := s.CreateSession(ctx, u.ID, dev.ID, s.sessionTTL(ctx))
	if err != nil {
		return res, err
	}
	u.HasTOTP, u.HasPasskey = false, false
	// Bootstrap admin access until a second factor is enrolled.
	mfaToken, _ := s.IssueBootstrapMFA(ctx, u.ID, dev.ID)
	res = InitResult{Token: token, User: u, Device: dev, MFAToken: mfaToken}
	s.audit.Log(ctx, "auth.init", "初始化管理员账号: "+username, ip, dev.ID)
	return res, nil
}

type LoginResult struct {
	Token        string `json:"token,omitempty"`
	User         User   `json:"user"`
	Device       Device `json:"device"`
	MFARequired  string `json:"mfaRequired,omitempty"`
	NeedApproval bool   `json:"needApproval,omitempty"`
	RequestID    string `json:"deviceRequestId,omitempty"`
	Code         string `json:"code,omitempty"`
	MFAToken     string `json:"mfaToken,omitempty"`
}

// Login authenticates by username/password, enforcing TOTP and device policy.
func (s *Service) Login(ctx context.Context, username, password, totpCode, deviceName, ua, ip string, ttlDays int, clientKey string) (LoginResult, error) {
	var res LoginResult
	key := ip + "|" + strings.ToLower(username)
	if !s.limiter.Allow(key) {
		return res, ErrRateLimited
	}
	var id, hash, created string
	var totpSecret *string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,password_hash,created_at,totp_secret FROM users WHERE username=?`, username).
		Scan(&id, &hash, &created, &totpSecret)
	if err != nil || !VerifyPassword(password, hash) {
		s.limiter.Fail(key)
		s.audit.Log(ctx, "auth.login_failed", "登录失败: "+username, ip, "")
		return res, ErrInvalidCredentials
	}
	u := User{ID: id, Username: username, CreatedAt: created}
	u.HasTOTP = totpSecret != nil && *totpSecret != ""
	u.HasPasskey = s.HasPasskey(ctx, id)

	if u.HasTOTP {
		if totpCode == "" {
			res.User, res.MFARequired = u, "totp"
			return res, nil
		}
		if err := s.TOTPVerify(ctx, id, totpCode); err != nil {
			s.limiter.Fail(key)
			s.audit.Log(ctx, "auth.mfa_failed", "TOTP 验证失败: "+username, ip, "")
			return res, ErrMFARequired
		}
	}

	policy := s.Setting(ctx, "device_policy")
	dev, decision, err := s.RegisterDevice(ctx, id, deviceName, ua, ip, policy, clientKey)
	if err != nil {
		return res, err
	}
	res.User, res.Device = u, dev
	if decision.Status != "allowed" {
		res.NeedApproval = true
		res.RequestID = decision.RequestID
		res.Code = decision.Code
		s.audit.Log(ctx, "auth.device_pending", "新设备等待授权: "+dev.Name, ip, dev.ID)
		return res, &DeviceActionRequired{Decision: decision}
	}
	s.limiter.Reset(key)
	token, err := s.CreateSession(ctx, id, dev.ID, s.ttlFor(ctx, ttlDays))
	if err != nil {
		return res, err
	}
	res.Token = token
	// Before a second factor exists, issue a bootstrap admin grant so the user
	// can reach the admin UI to configure TOTP or a passkey.
	if !u.HasTOTP && !u.HasPasskey {
		res.MFAToken, _ = s.IssueBootstrapMFA(ctx, id, dev.ID)
	}
	s.audit.Log(ctx, "auth.login", "登录成功: "+username, ip, dev.ID)
	return res, nil
}

// LoginWithDevice finalizes a login after device approval by issuing a session
// to the (now authorized) device.
func (s *Service) LoginWithDevice(ctx context.Context, userID, deviceID string, ttlDays int) (string, error) {
	var authorized int
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT authorized,revoked_at FROM devices WHERE id=? AND user_id=?`, deviceID, userID).
		Scan(&authorized, &revoked)
	if err != nil || authorized == 0 || revoked.Valid {
		return "", errors.New("device not authorized")
	}
	return s.CreateSession(ctx, userID, deviceID, s.ttlFor(ctx, ttlDays))
}

// CurrentUser loads a user by id for API responses.
func (s *Service) CurrentUser(ctx context.Context, userID string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id,username,created_at,totp_secret FROM users WHERE id=?`, userID).
		Scan(&u.ID, &u.Username, &u.CreatedAt, &u.totpSecret)
	if err != nil {
		return User{}, err
	}
	u.HasTOTP = u.totpSecret != nil && *u.totpSecret != ""
	u.HasPasskey = s.HasPasskey(ctx, userID)
	u.totpSecret = nil
	return u, nil
}

// ChangePassword verifies the old password and stores a new Argon2id hash.
func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrWeakPassword
	}
	var hash string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id=?`, userID).Scan(&hash); err != nil {
		return err
	}
	if !VerifyPassword(oldPassword, hash) {
		return ErrInvalidCredentials
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, newHash, userID)
	return err
}

// Setting reads a single setting value.
func (s *Service) Setting(ctx context.Context, key string) string {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

// Settings returns all settings (values are non-sensitive by contract).
func (s *Service) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

var mutableSettings = map[string]bool{
	"retention_days": true, "recycle_days": true, "max_upload_size": true,
	"upload_concurrency": true, "chunk_size": true, "storage_warning_percent": true,
	"device_policy": true, "link_preview_enabled": true, "language": true,
	"session_ttl_days": true, "login_attempt_limit": true,
	"notify_new_device": true, "notify_upload_done": true, "notify_upload_failed": true,
	"notify_low_storage": true, "notify_expiring": true,
}

// UpdateSettings writes only whitelisted keys. Returns the keys actually changed.
func (s *Service) UpdateSettings(ctx context.Context, values map[string]string) ([]string, error) {
	changed := []string{}
	for k, v := range values {
		if !mutableSettings[k] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE settings SET value=? WHERE key=?`, v, k); err != nil {
			return nil, err
		}
		changed = append(changed, k)
	}
	return changed, nil
}

// ttlFor applies a per-login override: positive = days, negative = forever,
// zero = instance default from settings.
func (s *Service) ttlFor(ctx context.Context, days int) time.Duration {
	if days > 0 {
		d := time.Duration(days) * 24 * time.Hour
		if d > s.cfg.MaxSessionTTL {
			d = s.cfg.MaxSessionTTL
		}
		return d
	}
	if days < 0 {
		return 0
	}
	return s.sessionTTL(ctx)
}

// SessionTTLFromDays resolves a session TTL from a day count supplied by the
// client after device approval.
func (s *Service) SessionTTLFromDays(ctx context.Context, days int) time.Duration {
	return s.ttlFor(ctx, days)
}

func (s *Service) sessionTTL(ctx context.Context) time.Duration {
	v := strings.TrimSpace(s.Setting(ctx, "session_ttl_days"))
	if v == "" {
		return s.cfg.SessionTTL
	}
	if strings.EqualFold(v, "forever") {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return s.cfg.SessionTTL
	}
	if n <= 0 {
		return 0
	}
	d := time.Duration(n) * 24 * time.Hour
	if d > s.cfg.MaxSessionTTL {
		d = s.cfg.MaxSessionTTL
	}
	return d
}

// UpdateLastActive touches a device's activity timestamp.
func (s *Service) UpdateLastActive(ctx context.Context, deviceID string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE devices SET last_active=? WHERE id=?`, util.Now(), deviceID)
}

// --- login rate limiter ---------------------------------------------------

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*rateAttempt
}
type rateAttempt struct {
	failures    int
	lockedUntil time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{attempts: map[string]*rateAttempt{}}
}

const (
	maxFailures  = 5
	lockDuration = 15 * time.Minute
)

func (r *rateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.attempts[key]
	if a == nil {
		return true
	}
	if !a.lockedUntil.IsZero() {
		if time.Now().Before(a.lockedUntil) {
			return false
		}
		delete(r.attempts, key)
	}
	return true
}

func (r *rateLimiter) Fail(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.attempts[key]
	if a == nil {
		a = &rateAttempt{}
		r.attempts[key] = a
	}
	a.failures++
	if a.failures >= maxFailures {
		a.lockedUntil = time.Now().Add(lockDuration)
		a.failures = 0
	}
}

func (r *rateLimiter) Reset(key string) {
	r.mu.Lock()
	delete(r.attempts, key)
	r.mu.Unlock()
}
