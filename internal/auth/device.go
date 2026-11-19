package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"private-drop/internal/db"
	"private-drop/internal/util"
)

type Device struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	IP         string  `json:"ip"`
	CreatedAt  string  `json:"createdAt"`
	LastActive string  `json:"lastActive"`
	Authorized bool    `json:"authorized"`
	RevokedAt  *string `json:"revokedAt"`
	Current    bool    `json:"current,omitempty"`
}

type DeviceRequest struct {
	ID        string `json:"id"`
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	IP        string `json:"ip"`
	Code      string `json:"code,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// DeviceDecision describes the outcome of registering a device during login.
type DeviceDecision struct {
	Status    string // "allowed", "pending", "denied"
	RequestID string
	Code      string
}

var (
	ErrDeviceNotFound  = errors.New("device not found")
	ErrRequestNotFound = errors.New("device request not found")
)

// DetectDeviceType heuristically classifies a User-Agent string.
func DetectDeviceType(ua string) string {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "ipad") || strings.Contains(u, "tablet"):
		return "tablet"
	case strings.Contains(u, "iphone") || strings.Contains(u, "android") || strings.Contains(u, "mobile"):
		return "mobile"
	case strings.Contains(u, "macintosh") || strings.Contains(u, "windows") || strings.Contains(u, "linux"):
		return "desktop"
	default:
		return "unknown"
	}
}

func deriveDeviceName(requested, ua, typ string) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	brand := "未知设备"
	u := strings.ToLower(ua)
	for _, b := range []struct{ key, name string }{
		{"edg", "Edge"}, {"chrome", "Chrome"}, {"firefox", "Firefox"},
		{"safari", "Safari"}, {"iphone", "iPhone"}, {"ipad", "iPad"},
		{"android", "Android"}, {"okhttp", "Android"},
	} {
		if strings.Contains(u, b.key) {
			brand = b.name
			break
		}
	}
	loc := map[string]string{"mobile": "手机", "tablet": "平板", "desktop": "电脑", "unknown": "设备"}[typ]
	return brand + " " + loc
}

// RegisterDevice finds or creates a device and applies the configured policy.
// clientKey is a stable secret generated and stored by the client. Device identity
// is based on it (not on the user-editable display name), so knowing a password
// and a device's name is not enough to skip approval/whitelist policies.
func (s *Service) RegisterDevice(ctx context.Context, userID, requestedName, ua, ip, policy, clientKey string) (Device, DeviceDecision, error) {
	typ := DetectDeviceType(ua)
	name := deriveDeviceName(requestedName, ua, typ)
	now := util.Now()

	var dev Device
	var err error
	if clientKey != "" {
		err = s.db.QueryRowContext(ctx, `
            SELECT id,name,type,ip,created_at,last_active,authorized,revoked_at
            FROM devices WHERE user_id=? AND client_key=? AND revoked_at IS NULL ORDER BY rowid DESC LIMIT 1`,
			userID, clientKey).Scan(&dev.ID, &dev.Name, &dev.Type, &dev.IP, &dev.CreatedAt, &dev.LastActive, &dev.Authorized, &dev.RevokedAt)
	} else {
		// Legacy clients without a key: fall back to a strict UA+IP match.
		// The mutable display name is deliberately NOT used as an identity.
		err = s.db.QueryRowContext(ctx, `
            SELECT id,name,type,ip,created_at,last_active,authorized,revoked_at
            FROM devices WHERE user_id=? AND user_agent=? AND ip=? AND revoked_at IS NULL ORDER BY rowid DESC LIMIT 1`,
			userID, ua, ip).Scan(&dev.ID, &dev.Name, &dev.Type, &dev.IP, &dev.CreatedAt, &dev.LastActive, &dev.Authorized, &dev.RevokedAt)
	}
	if err == nil {
		// Known device: refresh activity. Revocation is handled at auth time.
		_, _ = s.db.ExecContext(ctx, `UPDATE devices SET last_active=?, ip=? WHERE id=?`, now, ip, dev.ID)
		dev.LastActive, dev.IP = now, ip
		return dev, DeviceDecision{Status: "allowed"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Device{}, DeviceDecision{}, err
	}

	// New device.
	dev = Device{
		ID: util.NewID(), Name: name, Type: typ, IP: ip,
		CreatedAt: now, LastActive: now,
	}
	decision := DeviceDecision{Status: "allowed"}
	switch policy {
	case "approval":
		dev.Authorized = false
		decision.Status = "pending"
	case "whitelist":
		dev.Authorized = false
		decision.Status = "denied"
	default: // open or unknown
		dev.Authorized = true
	}

	code := ""
	if decision.Status != "allowed" {
		code = strings.ToUpper(util.RandomToken()[:8])
	}

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO devices(id,user_id,name,type,ip,user_agent,client_key,created_at,last_active,authorized)
            VALUES(?,?,?,?,?,?,?,?,?,?)`,
			dev.ID, userID, dev.Name, dev.Type, dev.IP, ua, clientKey, dev.CreatedAt, dev.LastActive, db.IntBool(dev.Authorized)); err != nil {
			return err
		}
		if decision.Status != "allowed" {
			decision.RequestID = util.NewID()
			if _, err := tx.ExecContext(ctx, `
                INSERT INTO device_requests(id,device_id,name,type,ip,device_info,code,status,created_at)
                VALUES(?,?,?,?,?,?,?,'pending',?)`,
				decision.RequestID, dev.ID, dev.Name, dev.Type, dev.IP, ua, code, now); err != nil {
				return err
			}
			decision.Code = code
		}
		return nil
	})
	if err != nil {
		return Device{}, DeviceDecision{}, err
	}
	if decision.Status != "allowed" {
		s.events.Publish("device.new", map[string]any{
			"id": dev.ID, "name": dev.Name, "type": dev.Type,
			"requestId": decision.RequestID,
		})
		if !strings.EqualFold(strings.TrimSpace(s.Setting(ctx, "notify_new_device")), "false") {
			s.notify.Create(ctx, "device_request", map[string]any{
				"deviceId": dev.ID, "name": dev.Name, "requestId": decision.RequestID,
			})
		}
		s.audit.Log(ctx, "device.request", "新设备请求授权: "+dev.Name, ip, dev.ID)
	}
	return dev, decision, nil
}

func scanDevice(row interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var authorized int
	var revokedNS sql.NullString
	err := row.Scan(&d.ID, &d.Name, &d.Type, &d.IP, &d.CreatedAt, &d.LastActive, &authorized, &revokedNS)
	if err != nil {
		return Device{}, err
	}
	d.Authorized = db.Bool(int64(authorized))
	if revokedNS.Valid {
		v := revokedNS.String
		d.RevokedAt = &v
	}
	return d, nil
}

func (s *Service) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id,name,type,ip,created_at,last_active,authorized,revoked_at
        FROM devices WHERE user_id=? ORDER BY last_active DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) RenameDevice(ctx context.Context, userID, deviceID, name string) (Device, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Device{}, errors.New("name required")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET name=? WHERE id=? AND user_id=?`, name, deviceID, userID)
	if err != nil {
		return Device{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Device{}, ErrDeviceNotFound
	}
	return s.getDevice(ctx, deviceID)
}

// Device returns a device by id (exported for API responses).
func (s *Service) Device(ctx context.Context, id string) (Device, error) {
	return s.getDevice(ctx, id)
}

func (s *Service) getDevice(ctx context.Context, id string) (Device, error) {
	return scanDevice(s.db.QueryRowContext(ctx, `
        SELECT id,name,type,ip,created_at,last_active,authorized,revoked_at FROM devices WHERE id=?`, id))
}

// RevokeDevice marks a device revoked and deletes all of its sessions.
func (s *Service) RevokeDevice(ctx context.Context, userID, deviceID, ip string) error {
	now := util.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE devices SET revoked_at=?, authorized=0 WHERE id=? AND user_id=? AND revoked_at IS NULL`,
		now, deviceID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDeviceNotFound
	}
	if err := s.DeleteDeviceSessions(ctx, deviceID); err != nil {
		return err
	}
	s.events.Publish("device.revoked", map[string]any{"id": deviceID})
	s.notify.Create(ctx, "device_revoked", map[string]any{"deviceId": deviceID})
	s.audit.Log(ctx, "device.revoke", "撤销设备: "+deviceID, ip, deviceID)
	return nil
}

func (s *Service) ListRequests(ctx context.Context) ([]DeviceRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id,device_id,name,type,ip,code,status,created_at
        FROM device_requests WHERE status='pending' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeviceRequest{}
	for rows.Next() {
		var r DeviceRequest
		if err := rows.Scan(&r.ID, &r.DeviceID, &r.Name, &r.Type, &r.IP, &r.Code, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) getRequest(ctx context.Context, id string) (DeviceRequest, error) {
	var r DeviceRequest
	err := s.db.QueryRowContext(ctx, `
        SELECT id,device_id,name,type,ip,code,status,created_at FROM device_requests WHERE id=?`, id).
		Scan(&r.ID, &r.DeviceID, &r.Name, &r.Type, &r.IP, &r.Code, &r.Status, &r.CreatedAt)
	return r, err
}

func (s *Service) approve(ctx context.Context, r DeviceRequest, ip string) (Device, error) {
	now := util.Now()
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE device_requests SET status='approved' WHERE id=?`, r.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE devices SET authorized=1, revoked_at=NULL, last_active=? WHERE id=?`, now, r.DeviceID)
		return err
	})
	if err != nil {
		return Device{}, err
	}
	s.events.Publish("device.authorized", map[string]any{"id": r.DeviceID, "requestId": r.ID})
	s.notify.Create(ctx, "device_authorized", map[string]any{"deviceId": r.DeviceID, "name": r.Name})
	s.audit.Log(ctx, "device.approve", "批准设备: "+r.Name, ip, r.DeviceID)
	return s.getDevice(ctx, r.DeviceID)
}

func (s *Service) ApproveRequest(ctx context.Context, id, ip string) (Device, error) {
	r, err := s.getRequest(ctx, id)
	if err != nil {
		return Device{}, ErrRequestNotFound
	}
	if r.Status != "pending" {
		return Device{}, ErrRequestNotFound
	}
	return s.approve(ctx, r, ip)
}

func (s *Service) RejectRequest(ctx context.Context, id, ip string) error {
	r, err := s.getRequest(ctx, id)
	if err != nil {
		return ErrRequestNotFound
	}
	res, err := s.db.ExecContext(ctx, `UPDATE device_requests SET status='rejected' WHERE id=? AND status='pending'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRequestNotFound
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM devices WHERE id=? AND authorized=0`, r.DeviceID)
	s.audit.Log(ctx, "device.reject", "拒绝设备: "+r.Name, ip, r.DeviceID)
	return nil
}

func (s *Service) ApproveByCode(ctx context.Context, code, ip string) (Device, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM device_requests WHERE code=? AND status='pending'`, strings.ToUpper(strings.TrimSpace(code))).Scan(&id)
	if err != nil {
		return Device{}, ErrRequestNotFound
	}
	return s.ApproveRequest(ctx, id, ip)
}
