package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"private-drop/internal/auth"
)

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	initialized := s.auth.IsInitialized(r.Context())
	writeJSON(w, 200, map[string]any{"initialized": initialized, "lanOnly": !initialized})
}

func (s *Server) authInit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
		Policy     string `json:"policy"`
	}
	if err := auth.DecodeJSON(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	// Fail closed: a request carrying proxy headers from an untrusted peer may
	// be a public client arriving through Cloudflare Tunnel / a reverse proxy.
	// Without TRUSTED_PROXY_CIDRS, its original public IP cannot be trusted, so
	// initialization must not proceed even if the immediate peer is private.
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("CF-Connecting-IP") != "" || r.Header.Get("X-Real-IP") != "" {
		if !s.isTrustedProxyPeer(r) {
			forbidden(w, "initialization behind a proxy requires TRUSTED_PROXY_CIDRS to be configured")
			return
		}
	}
	if !auth.IsPrivateIP(s.clientIP(r)) {
		forbidden(w, "initialization is only allowed from a LAN address")
		return
	}
	res, err := s.auth.Init(r.Context(), body.Username, body.Password, body.DeviceName, body.Policy,
		r.UserAgent(), s.clientIP(r), r.Header.Get("X-Device-Key"))
	if err != nil {
		if errors.Is(err, auth.ErrAlreadyInitialized) {
			writeErr(w, 409, "ALREADY_INITIALIZED", "instance is already initialized")
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) || errors.Is(err, auth.ErrUsernameRequired) {
			badRequest(w, err.Error())
			return
		}
		serverErr(w, err)
		return
	}
	s.audit.Log(r.Context(), "auth.init", "初始化完成", s.clientIP(r), res.Device.ID)
	writeJSON(w, 200, res)
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
		TOTP       string `json:"totp"`
		TTLDays    int    `json:"ttlDays"`
	}
	if err := auth.DecodeJSON(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	res, err := s.auth.Login(r.Context(), body.Username, body.Password, body.TOTP, body.DeviceName,
		r.UserAgent(), s.clientIP(r), body.TTLDays, r.Header.Get("X-Device-Key"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			writeErr(w, 429, "RATE_LIMITED", "too many login attempts, try again later")
		case errors.Is(err, auth.ErrMFARequired):
			badRequest(w, "invalid TOTP code")
		case errors.Is(err, auth.ErrInvalidCredentials):
			unauthorized(w, "invalid username or password")
		default:
			var dar *auth.DeviceActionRequired
			if errors.As(err, &dar) {
				writeErr(w, 403, "DEVICE_APPROVAL_REQUIRED", err.Error())
				return
			}
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	if err := s.auth.DeleteSession(r.Context(), p.Token); err != nil {
		serverErr(w, err)
		return
	}
	_ = s.auth.RevokeMFAForDevice(r.Context(), p.UserID, p.DeviceID)
	s.audit.Log(r.Context(), "auth.logout", "退出登录", s.clientIP(r), p.DeviceID)
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	u, err := s.auth.CurrentUser(r.Context(), p.UserID)
	if err != nil {
		serverErr(w, err)
		return
	}
	dev, err := s.auth.Device(r.Context(), p.DeviceID)
	if err != nil {
		dev = auth.Device{ID: p.DeviceID, Name: p.DeviceName, Type: p.DeviceType}
	}
	dev.Current = true
	settings, err := s.auth.Settings(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": u, "device": dev, "settings": settings})
}

func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	if err := s.auth.DeleteSession(r.Context(), p.Token); err != nil {
		serverErr(w, err)
		return
	}
	token, err := s.auth.CreateSession(r.Context(), p.UserID, p.DeviceID, s.auth.SessionTTLFromDays(r.Context(), 0))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token})
}

func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	setup, err := s.auth.TOTPSetup(p.UserID, p.Username)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, setup)
}

func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := auth.DecodeJSON(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	p := principal(r)
	if err := s.auth.TOTPEnable(r.Context(), p.UserID, body.Code); err != nil {
		if errors.Is(err, auth.ErrInvalidTOTP) {
			badRequest(w, "invalid TOTP code")
		} else {
			badRequest(w, err.Error())
		}
		return
	}
	// Retire the bootstrap grant now that a real second factor exists.
	_ = s.auth.RevokeMFAForDevice(r.Context(), p.UserID, p.DeviceID)
	s.audit.Log(r.Context(), "security.totp_enable", "启用 TOTP", s.clientIP(r), p.DeviceID)
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	opts, err := s.auth.PasskeyRegisterBegin(r.Context(), p.UserID, w, r)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, opts)
}

func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	if err := s.auth.PasskeyRegisterFinish(r.Context(), p.UserID, w, r); err != nil {
		badRequest(w, "passkey registration failed: "+err.Error())
		return
	}
	_ = s.auth.RevokeMFAForDevice(r.Context(), p.UserID, p.DeviceID)
	s.audit.Log(r.Context(), "security.passkey_add", "注册 Passkey", s.clientIP(r), p.DeviceID)
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	_ = auth.DecodeJSON(r, &body) // optional body
	opts, err := s.auth.PasskeyLoginBegin(r.Context(), body.Username, w, r)
	if err != nil {
		badRequest(w, "passkey login unavailable: "+err.Error())
		return
	}
	writeJSON(w, 200, opts)
}

func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TTLDays int `json:"ttlDays"`
	}
	// Read ttlDays without consuming the assertion body.
	_ = auth.DecodeJSON(r, &body)
	userID, err := s.auth.PasskeyLoginFinish(r.Context(), w, r)
	if err != nil {
		unauthorized(w, "passkey verification failed")
		return
	}
	u, err := s.auth.CurrentUser(r.Context(), userID)
	if err != nil {
		serverErr(w, err)
		return
	}
	dev, decision, err := s.auth.RegisterDevice(r.Context(), userID, "", r.UserAgent(), s.clientIP(r),
		s.auth.Setting(r.Context(), "device_policy"), r.Header.Get("X-Device-Key"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if decision.Status != "allowed" {
		writeErr(w, 403, "DEVICE_APPROVAL_REQUIRED", "device approval required")
		return
	}
	token, err := s.auth.CreateSession(r.Context(), userID, dev.ID, s.auth.SessionTTLFromDays(r.Context(), body.TTLDays))
	if err != nil {
		serverErr(w, err)
		return
	}
	s.audit.Log(r.Context(), "auth.passkey_login", "Passkey 登录", s.clientIP(r), dev.ID)
	writeJSON(w, 200, map[string]any{"token": token, "user": u, "device": dev})
}

func (s *Server) mfaVerify(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var body struct {
		Type      string          `json:"type"`
		Code      string          `json:"code"`
		Assertion json.RawMessage `json:"assertion"`
	}
	if err := auth.DecodeJSON(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	switch body.Type {
	case "totp":
		if err := s.auth.TOTPVerify(r.Context(), p.UserID, body.Code); err != nil {
			badRequest(w, "invalid TOTP code")
			return
		}
	case "passkey":
		if len(body.Assertion) > 0 {
			// Provide the nested assertion as the WebAuthn ceremony body.
			r = r.Clone(r.Context())
			r.Body = io.NopCloser(bytes.NewReader(body.Assertion))
		}
		if _, err := s.auth.PasskeyLoginFinish(r.Context(), w, r); err != nil {
			unauthorized(w, "passkey verification failed")
			return
		}
	default:
		badRequest(w, "unsupported MFA type")
		return
	}
	token, err := s.auth.IssueMFA(r.Context(), p.UserID, p.DeviceID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"mfaToken": token})
}
