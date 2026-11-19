// Package api implements the REST contract, middleware and static hosting.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	rootassets "private-drop"
	"private-drop/internal/audit"
	"private-drop/internal/auth"
	"private-drop/internal/clean"
	"private-drop/internal/config"
	"private-drop/internal/db"
	"private-drop/internal/file"
	"private-drop/internal/linkpreview"
	"private-drop/internal/message"
	"private-drop/internal/notify"
	"private-drop/internal/search"
	"private-drop/internal/upload"
	"private-drop/internal/ws"
)

type Server struct {
	cfg       config.Config
	db        *db.DB
	auth      *auth.Service
	messages  *message.Service
	files     *file.Service
	uploads   *upload.Service
	searchSvc *search.Service
	notify    *notify.Service
	clean     *clean.Service
	audit     *audit.Service
	links     *linkpreview.Previewer
	hub       *ws.Hub
	mux       *http.ServeMux
	trusted   []*net.IPNet
}

func New(d *db.DB, cfg config.Config) *Server {
	// The hub is created first with a validator closing over a later-assigned
	// auth service, breaking the auth<->ws initialization cycle.
	var authSvc *auth.Service
	eventStore := ws.NewDBEventStore(d)
	hub := ws.NewWithStore(func(token string) (string, bool) {
		if authSvc == nil {
			return "", false
		}
		p, err := authSvc.Authenticate(context.Background(), token)
		if err != nil {
			return "", false
		}
		return p.DeviceID, true
	}, eventStore)
	notifySvc := notify.New(d, hub)
	auditSvc := audit.New(d)
	authSvc = auth.New(d, cfg, hub, notifySvc, auditSvc)
	filesSvc := file.NewWithEvents(d, cfg, hub)
	links := linkpreview.New()
	msgSvc := message.New(d, cfg, filesSvc, hub, auditSvc)
	msgSvc.SetLinkPreviewer(links)
	uploadSvc := upload.New(d, cfg, filesSvc, hub, auditSvc, notifySvc)
	// Message deletion/purge/expiry must cancel bound uploads and clean chunks.
	msgSvc.SetUploads(uploadSvc)
	searchSvc := search.New(d, msgSvc, filesSvc)
	cleanSvc := clean.New(d, cfg, filesSvc, msgSvc, uploadSvc, notifySvc, auditSvc)

	s := &Server{
		cfg: cfg, db: d, auth: authSvc, messages: msgSvc, files: filesSvc,
		uploads: uploadSvc, searchSvc: searchSvc, notify: notifySvc,
		clean: cleanSvc, audit: auditSvc, links: links,
		hub: hub, mux: http.NewServeMux(),
	}
	for _, c := range cfg.TrustedProxyCIDR {
		if _, n, err := net.ParseCIDR(c); err == nil {
			s.trusted = append(s.trusted, n)
		}
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.recoverMiddleware(s.requestLog(s.mux))
}

// RunCleanup exposes one retention/recycle pass to the server scheduler.
func (s *Server) RunCleanup(ctx context.Context) (clean.Report, error) {
	rep, err := s.clean.Run(ctx)
	if err == nil && s.hub.Store() != nil {
		// Keep the durable reconnect log bounded (one week of history).
		if terr := s.hub.Store().Trim(ctx, 7*24*time.Hour); terr != nil {
			log.Printf("ws event trim: %v", terr)
		}
	}
	return rep, err
}

// Publish emits a real-time event through the WebSocket hub.
func (s *Server) Publish(eventType string, data any) { s.hub.Publish(eventType, data) }

// WSClientCount is used by tests to wait for connection registration.
func (s *Server) WSClientCount() int {
	n, _ := s.hub.Stats()["clients"].(int)
	return n
}

// --- response helpers ------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

type errBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	var b errBody
	b.Error.Code, b.Error.Message = code, msg
	writeJSON(w, status, b)
}

func badRequest(w http.ResponseWriter, msg string)   { writeErr(w, 400, "BAD_REQUEST", msg) }
func unauthorized(w http.ResponseWriter, msg string) { writeErr(w, 401, "UNAUTHORIZED", msg) }
func forbidden(w http.ResponseWriter, msg string)    { writeErr(w, 403, "FORBIDDEN", msg) }
func notFound(w http.ResponseWriter, msg string)     { writeErr(w, 404, "NOT_FOUND", msg) }
func serverErr(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	writeErr(w, 500, "INTERNAL_ERROR", "internal server error")
}

type handler func(http.ResponseWriter, *http.Request)

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic serving %s: %v", r.URL.Path, rec)
				writeErr(w, 500, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// --- auth & client ip ------------------------------------------------------

type ctxKey int

const principalKey ctxKey = 1

func principal(r *http.Request) *auth.Principal {
	p, _ := r.Context().Value(principalKey).(*auth.Principal)
	return p
}

func (s *Server) withAuth(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			unauthorized(w, "missing bearer token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		p, err := s.auth.Authenticate(r.Context(), token)
		if err != nil {
			unauthorized(w, "invalid or expired token")
			return
		}
		s.auth.UpdateLastActive(r.Context(), p.DeviceID)
		ctx := context.WithValue(r.Context(), principalKey, p)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) withMFA(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) {
		p := principal(r)
		if p == nil {
			unauthorized(w, "authentication required")
			return
		}
		token := r.Header.Get("X-MFA-Token")
		if token == "" {
			token = r.URL.Query().Get("mfaToken")
		}
		if !s.auth.ValidateMFA(r.Context(), token, p.UserID, p.DeviceID) {
			writeErr(w, 403, "MFA_REQUIRED", "secondary verification required")
			return
		}
		next(w, r)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	remote := r.RemoteAddr
	if h, _, err := net.SplitHostPort(remote); err == nil {
		remote = h
	} else {
		remote = strings.Trim(remote, "[]")
	}
	// Only trust X-Forwarded-For when the direct peer is a configured proxy.
	ip := net.ParseIP(remote)
	for _, n := range s.trusted {
		if ip != nil && n.Contains(ip) {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				first := strings.TrimSpace(strings.Split(xff, ",")[0])
				if net.ParseIP(first) != nil {
					return first
				}
			}
		}
	}
	return remote
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func queryBool(r *http.Request, key string) *bool {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}
	return &b
}

// isTrustedProxyPeer reports whether the direct TCP peer is a configured proxy.
// Used to fail closed on initialization requests that arrive through a proxy.
func (s *Server) isTrustedProxyPeer(r *http.Request) bool {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		host = strings.Trim(host, "[]")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range s.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// --- routes ----------------------------------------------------------------

func (s *Server) routes() {
	m := s.mux
	wrap := func(h handler) http.Handler { return http.HandlerFunc(s.withAuth(h)) }
	wrapMFA := func(h handler) http.Handler { return http.HandlerFunc(s.withAuth(s.withMFA(h))) }

	// Public auth endpoints.
	m.HandleFunc("GET /api/auth/status", s.authStatus)
	m.HandleFunc("POST /api/auth/init", s.authInit)
	m.HandleFunc("POST /api/auth/login", s.authLogin)
	m.HandleFunc("POST /api/auth/passkey/login/begin", s.passkeyLoginBegin)
	m.HandleFunc("POST /api/auth/passkey/login/finish", s.passkeyLoginFinish)

	// Authenticated auth endpoints.
	m.Handle("POST /api/auth/logout", wrap(s.authLogout))
	m.Handle("GET /api/auth/me", wrap(s.authMe))
	m.Handle("POST /api/auth/refresh", wrap(s.authRefresh))
	m.Handle("POST /api/auth/totp/setup", wrap(s.totpSetup))
	m.Handle("POST /api/auth/totp/enable", wrap(s.totpEnable))
	m.Handle("POST /api/auth/passkey/register/begin", wrap(s.passkeyRegisterBegin))
	m.Handle("POST /api/auth/passkey/register/finish", wrap(s.passkeyRegisterFinish))
	m.Handle("POST /api/auth/mfa/verify", wrap(s.mfaVerify))

	// Devices. Administration requires secondary verification; one-time-code
	// approval stays auth-only because it is the new-device bootstrap flow.
	m.Handle("GET /api/devices", wrapMFA(s.listDevices))
	m.Handle("PATCH /api/devices/{id}", wrapMFA(s.renameDevice))
	m.Handle("POST /api/devices/{id}/revoke", wrapMFA(s.revokeDevice))
	m.Handle("GET /api/devices/requests", wrapMFA(s.listDeviceRequests))
	m.Handle("POST /api/devices/requests/{id}/approve", wrapMFA(s.approveDeviceRequest))
	m.Handle("POST /api/devices/requests/{id}/reject", wrapMFA(s.rejectDeviceRequest))
	m.Handle("POST /api/devices/requests/code", wrap(s.approveDeviceCode))

	// Messages.
	m.Handle("GET /api/messages", wrap(s.listMessages))
	m.Handle("POST /api/messages", wrap(s.createMessage))
	m.Handle("GET /api/messages/expiring", wrap(s.expiringMessages))
	m.Handle("POST /api/messages/bulk", wrap(s.bulkMessages))
	m.Handle("GET /api/messages/{id}", wrap(s.getMessage))
	m.Handle("PATCH /api/messages/{id}", wrap(s.updateMessage))
	m.Handle("DELETE /api/messages/{id}", wrap(s.deleteMessage))

	// Files.
	m.Handle("GET /api/files", wrap(s.listFiles))
	m.Handle("GET /api/files/{id}", wrap(s.getFile))
	m.Handle("PATCH /api/files/{id}", wrap(s.updateFile))
	m.Handle("POST /api/files/bulk", wrap(s.bulkFiles))
	m.Handle("DELETE /api/files/{id}", wrap(s.deleteFile))
	m.Handle("GET /api/files/{id}/download", wrap(s.downloadFile))
	m.Handle("GET /api/files/{id}/thumbnail", wrap(s.thumbnailFile))
	m.Handle("GET /api/files/{id}/preview", wrap(s.previewFile))
	m.Handle("POST /api/files/zip", wrap(s.zipFiles))

	// Uploads.
	m.Handle("POST /api/uploads/init", wrap(s.initUpload))
	m.Handle("GET /api/uploads", wrap(s.listUploads))
	m.Handle("PUT /api/uploads/{id}/chunk/{index}", wrap(s.putChunk))
	m.Handle("POST /api/uploads/{id}/complete", wrap(s.completeUpload))
	m.Handle("GET /api/uploads/{id}/status", wrap(s.uploadStatus))
	m.Handle("DELETE /api/uploads/{id}", wrap(s.cancelUpload))
	m.Handle("POST /api/uploads/{id}/retry", wrap(s.retryUpload))
	m.Handle("POST /api/uploads/{id}/needs-source", wrap(s.uploadNeedsSource))

	// Search, tags, recycle, notifications.
	m.Handle("GET /api/search", wrap(s.search))
	m.Handle("GET /api/tags", wrap(s.listTags))
	m.Handle("POST /api/tags", wrap(s.createTag))
	m.Handle("PATCH /api/tags/{id}", wrap(s.updateTag))
	m.Handle("DELETE /api/tags/{id}", wrap(s.deleteTag))
	m.Handle("GET /api/recycle", wrap(s.listRecycle))
	m.Handle("POST /api/recycle/{id}/restore", wrap(s.restoreRecycle))
	m.Handle("DELETE /api/recycle/{id}", wrap(s.purgeRecycle))
	m.Handle("DELETE /api/recycle", wrap(s.emptyRecycle))
	m.Handle("GET /api/notifications", wrap(s.listNotifications))
	m.Handle("POST /api/notifications/read-all", wrap(s.readAllNotifications))
	m.Handle("DELETE /api/notifications/{id}", wrap(s.deleteNotification))

	// Settings (require secondary verification).
	m.Handle("GET /api/settings", wrapMFA(s.getSettings))
	m.Handle("PATCH /api/settings", wrapMFA(s.patchSettings))
	m.Handle("GET /api/settings/security", wrapMFA(s.securitySettings))
	m.Handle("POST /api/settings/password", wrapMFA(s.changePassword))

	// Logs / audit / storage / system.
	m.Handle("GET /api/logs", wrap(s.logs))
	m.Handle("GET /api/audit", wrap(s.auditList))
	m.Handle("GET /api/storage/stats", wrap(s.storageStats))
	m.Handle("GET /api/system/info", wrap(s.systemInfo))
	m.Handle("GET /api/system/diagnose", wrap(s.diagnose))

	// Link preview + websocket.
	m.Handle("POST /api/link-preview", wrap(s.linkPreview))
	m.Handle("GET /api/ws", http.HandlerFunc(s.hub.ServeHTTP))

	// SPA fallback.
	sub, err := fs.Sub(rootassets.WebFS, "web/dist")
	if err != nil {
		log.Fatalf("embed web assets: %v", err)
	}
	m.Handle("/", http.HandlerFunc(s.spaHandler(sub)))
}

func (s *Server) spaHandler(sub fs.FS) handler {
	fileServer := http.FileServer(http.FS(sub))
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			notFound(w, "endpoint not found")
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := sub.Open(p); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		b, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, "frontend not built", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}
}
