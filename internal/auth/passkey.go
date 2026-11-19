package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"private-drop/internal/util"
)

const waCookieName = "pd_webauthn"

type waSession struct {
	data    webauthn.SessionData
	userID  string
	expires time.Time
}

var waSessions = struct {
	sync.Mutex
	m map[string]waSession
}{m: map[string]waSession{}}

func waSessionPut(id string, s waSession) {
	waSessions.Lock()
	waSessions.m[id] = s
	waSessions.Unlock()
}

func waSessionTake(id string) (waSession, bool) {
	waSessions.Lock()
	defer waSessions.Unlock()
	s, ok := waSessions.m[id]
	delete(waSessions.m, id)
	if !ok || time.Now().After(s.expires) {
		return waSession{}, false
	}
	return s, true
}

// webauthnUser adapts a user row to the webauthn.User interface.
type webauthnUser struct {
	ID          string
	Username    string
	Handle      []byte
	Credentials []webauthn.Credential
}

func (u webauthnUser) WebAuthnID() []byte                         { return u.Handle }
func (u webauthnUser) WebAuthnName() string                       { return u.Username }
func (u webauthnUser) WebAuthnDisplayName() string                { return u.Username }
func (u webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

func (s *Service) loadWebAuthnUser(ctx context.Context, userID string) (webauthnUser, error) {
	var u webauthnUser
	var handle []byte
	err := s.db.QueryRowContext(ctx, `SELECT id, username, webauthn_id FROM users WHERE id=?`, userID).
		Scan(&u.ID, &u.Username, &handle)
	if err != nil {
		return u, err
	}
	u.Handle = handle
	// If no handle exists yet, create a stable one.
	if len(u.Handle) == 0 {
		u.Handle = make([]byte, 32)
		if _, err := rand.Read(u.Handle); err != nil {
			return u, err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET webauthn_id=? WHERE id=?`, u.Handle, userID); err != nil {
			return u, err
		}
	}

	rows, err := s.db.QueryContext(ctx, `
        SELECT id, public_key, attestation_type, sign_count, transports
        FROM passkey_credentials WHERE user_id=?`, userID)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var pk []byte
		var att, transports string
		var signCount uint32
		if err := rows.Scan(&id, &pk, &att, &signCount, &transports); err != nil {
			return u, err
		}
		rawID, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil {
			continue
		}
		c := webauthn.Credential{
			ID:              rawID,
			PublicKey:       pk,
			AttestationType: att,
		}
		c.Authenticator.SignCount = signCount
		for _, t := range strings.Split(transports, ",") {
			if t = strings.TrimSpace(t); t != "" {
				c.Transport = append(c.Transport, protocol.AuthenticatorTransport(t))
			}
		}
		u.Credentials = append(u.Credentials, c)
	}
	return u, rows.Err()
}

func (s *Service) userIDByUsername(ctx context.Context, username string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&id)
	return id, err
}

// webauthnFor builds a WebAuthn relying party bound to the request origin.
func (s *Service) webauthnFor(r *http.Request) (*webauthn.WebAuthn, error) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	rpid := s.cfg.WebAuthnRPID
	if rpid == "" {
		rpid = host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	origins := s.cfg.WebAuthnOrigins
	if len(origins) == 0 {
		origins = []string{scheme + "://" + r.Host}
	}
	return webauthn.New(&webauthn.Config{
		RPID:          rpid,
		RPDisplayName: "Private Drop",
		RPOrigins:     origins,
	})
}

func setWACookie(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{
		Name: waCookieName, Value: id, Path: "/", HttpOnly: true,
		Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: 300,
	})
}

func readWACookie(r *http.Request) string {
	c, err := r.Cookie(waCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// PasskeyRegisterBegin starts a credential registration ceremony.
func (s *Service) PasskeyRegisterBegin(ctx context.Context, userID string, w http.ResponseWriter, r *http.Request) (any, error) {
	wa, err := s.webauthnFor(r)
	if err != nil {
		return nil, err
	}
	u, err := s.loadWebAuthnUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var exclude []protocol.CredentialDescriptor
	for _, c := range u.Credentials {
		exclude = append(exclude, c.Descriptor())
	}
	creation, session, err := wa.BeginRegistration(u,
		webauthn.WithExclusions(exclude),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(
			webauthn.SelectAuthenticator("", protocol.ResidentKeyRequired(), string(protocol.VerificationPreferred))),
	)
	if err != nil {
		return nil, err
	}
	id := util.RandomToken()
	waSessionPut(id, waSession{data: *session, userID: userID, expires: time.Now().Add(5 * time.Minute)})
	setWACookie(w, r, id)
	return creation, nil
}

// PasskeyRegisterFinish completes a credential registration ceremony.
func (s *Service) PasskeyRegisterFinish(ctx context.Context, userID string, w http.ResponseWriter, r *http.Request) error {
	wa, err := s.webauthnFor(r)
	if err != nil {
		return err
	}
	sess, ok := waSessionTake(readWACookie(r))
	if !ok || sess.userID != userID {
		return errors.New("passkey session expired")
	}
	u, err := s.loadWebAuthnUser(ctx, userID)
	if err != nil {
		return err
	}
	cred, err := wa.FinishRegistration(u, sess.data, r)
	if err != nil {
		return err
	}
	id := base64.RawURLEncoding.EncodeToString(cred.ID)
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	_, err = s.db.ExecContext(ctx, `
        INSERT INTO passkey_credentials(id,user_id,public_key,attestation_type,sign_count,transports,created_at)
        VALUES(?,?,?,?,?,?,?)
        ON CONFLICT(id) DO UPDATE SET public_key=excluded.public_key, sign_count=excluded.sign_count`,
		id, userID, cred.PublicKey, cred.AttestationType, cred.Authenticator.SignCount,
		strings.Join(transports, ","), util.Now())
	if err == nil {
		http.SetCookie(w, &http.Cookie{Name: waCookieName, Value: "", Path: "/", MaxAge: -1})
	}
	return err
}

// PasskeyLoginBegin starts an assertion ceremony. username may be empty for a
// discoverable (passkey) login.
func (s *Service) PasskeyLoginBegin(ctx context.Context, username string, w http.ResponseWriter, r *http.Request) (any, error) {
	wa, err := s.webauthnFor(r)
	if err != nil {
		return nil, err
	}
	var u webauthnUser
	var userID string
	if username != "" {
		userID, err = s.userIDByUsername(ctx, username)
		if err != nil {
			return nil, errors.New("unknown user")
		}
		u, err = s.loadWebAuthnUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if len(u.Credentials) == 0 {
			return nil, errors.New("no passkeys registered")
		}
	}
	var assertion any
	var session *webauthn.SessionData
	if username == "" {
		assertion, session, err = wa.BeginDiscoverableLogin(
			webauthn.WithUserVerification(protocol.VerificationPreferred))
	} else {
		assertion, session, err = wa.BeginLogin(u,
			webauthn.WithUserVerification(protocol.VerificationPreferred))
	}
	if err != nil {
		return nil, err
	}
	id := util.RandomToken()
	waSessionPut(id, waSession{data: *session, userID: userID, expires: time.Now().Add(5 * time.Minute)})
	setWACookie(w, r, id)
	return assertion, nil
}

// PasskeyLoginFinish verifies an assertion and returns the authenticated user.
func (s *Service) PasskeyLoginFinish(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, error) {
	wa, err := s.webauthnFor(r)
	if err != nil {
		return "", err
	}
	sess, ok := waSessionTake(readWACookie(r))
	if !ok {
		return "", errors.New("passkey session expired")
	}
	var userID string
	var cred *webauthn.Credential

	if sess.userID != "" {
		u, err := s.loadWebAuthnUser(ctx, sess.userID)
		if err != nil {
			return "", err
		}
		cred, err = wa.FinishLogin(u, sess.data, r)
		if err != nil {
			return "", err
		}
		userID = sess.userID
	} else {
		// Discoverable login: locate the user from the rawID in the request
		// by resolving through the parsed assertion's credential id.
		rawID, err := assertionCredentialID(r)
		if err != nil {
			return "", err
		}
		userID, err = s.userIDByCredentialID(ctx, rawID)
		if err != nil {
			return "", errors.New("unknown credential")
		}
		u, err := s.loadWebAuthnUser(ctx, userID)
		if err != nil {
			return "", err
		}
		handler := webauthn.DiscoverableUserHandler(func(_ []byte, _ []byte) (webauthn.User, error) {
			return u, nil
		})
		_, cred, err = wa.FinishPasskeyLogin(handler, sess.data, r)
		if err != nil {
			return "", err
		}
		userID = u.ID
	}

	// Persist the new signature counter.
	_, _ = s.db.ExecContext(ctx,
		`UPDATE passkey_credentials SET sign_count=? WHERE id=?`,
		cred.Authenticator.SignCount, base64.RawURLEncoding.EncodeToString(cred.ID))
	http.SetCookie(w, &http.Cookie{Name: waCookieName, Value: "", Path: "/", MaxAge: -1})
	return userID, nil
}

func (s *Service) userIDByCredentialID(ctx context.Context, rawID []byte) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM passkey_credentials WHERE id=?`, base64.RawURLEncoding.EncodeToString(rawID)).Scan(&id)
	return id, err
}

// assertionCredentialID extracts the credential id from a WebAuthn assertion
// request body ({ id, rawId, response: {...}, ... }).
func assertionCredentialID(r *http.Request) ([]byte, error) {
	var body struct {
		ID    string `json:"id"`
		RawID string `json:"rawId"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		return nil, err
	}
	enc := body.RawID
	if enc == "" {
		enc = body.ID
	}
	b, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// HasPasskey reports whether the user has at least one credential.
func (s *Service) HasPasskey(ctx context.Context, userID string) bool {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM passkey_credentials WHERE user_id=?`, userID).Scan(&n)
	return err == nil && n > 0
}
