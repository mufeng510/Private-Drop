package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image/png"
	"sync"

	"github.com/pquerna/otp/totp"
)

var ErrInvalidTOTP = errors.New("invalid TOTP code")

// pendingTOTP holds secrets between setup and enable so a secret is only
// persisted once the user proves they can generate a valid code.
var pendingTOTP = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

type TOTPSetup struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauthUrl"`
	QR         string `json:"qr"` // data:image/png;base64,...
}

func (s *Service) TOTPSetup(userID, username string) (TOTPSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Private Drop",
		AccountName: username,
	})
	if err != nil {
		return TOTPSetup{}, err
	}
	pendingTOTP.Lock()
	pendingTOTP.m[userID] = key.Secret()
	pendingTOTP.Unlock()

	qr := ""
	if img, err := key.Image(256, 256); err == nil {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil {
			qr = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		}
	}
	return TOTPSetup{Secret: key.Secret(), OTPAuthURL: key.URL(), QR: qr}, nil
}

func (s *Service) TOTPEnable(ctx context.Context, userID, code string) error {
	pendingTOTP.Lock()
	secret, ok := pendingTOTP.m[userID]
	pendingTOTP.Unlock()
	if !ok {
		return errors.New("no pending TOTP setup")
	}
	if !totp.Validate(code, secret) {
		return ErrInvalidTOTP
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE users SET totp_secret=? WHERE id=?`, secret, userID); err != nil {
		return err
	}
	pendingTOTP.Lock()
	delete(pendingTOTP.m, userID)
	pendingTOTP.Unlock()
	return nil
}

func (s *Service) TOTPVerify(ctx context.Context, userID, code string) error {
	secret, err := s.totpSecret(ctx, userID)
	if err != nil || secret == "" {
		return ErrInvalidTOTP
	}
	if !totp.Validate(code, secret) {
		return ErrInvalidTOTP
	}
	return nil
}

func (s *Service) totpSecret(ctx context.Context, userID string) (string, error) {
	var secret *string
	err := s.db.QueryRowContext(ctx, `SELECT totp_secret FROM users WHERE id=?`, userID).Scan(&secret)
	if err != nil {
		return "", err
	}
	if secret == nil {
		return "", nil
	}
	return *secret, nil
}

func (s *Service) HasTOTP(ctx context.Context, userID string) bool {
	secret, err := s.totpSecret(ctx, userID)
	return err == nil && secret != ""
}

// DisableTOTP removes TOTP from the account (requires prior MFA at API layer).
func (s *Service) DisableTOTP(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET totp_secret=NULL WHERE id=?`, userID)
	return err
}
