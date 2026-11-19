package auth_test

import (
	"context"
	"testing"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/auth"
	"private-drop/internal/config"
	"private-drop/internal/events"
	"private-drop/internal/notify"
	"private-drop/internal/testutil"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	d, _ := testutil.NewDB(t)
	n := notify.New(d, events.Noop{})
	return auth.New(d, config.Config{SessionTTL: 24 * time.Hour}, events.Noop{}, n, audit.New(d))
}

func TestPasswordHashVerify(t *testing.T) {
	h, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !auth.VerifyPassword("correct horse battery staple", h) {
		t.Fatal("valid password rejected")
	}
	if auth.VerifyPassword("wrong", h) {
		t.Fatal("invalid password accepted")
	}
}

func TestInitOnceAndLogin(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	if svc.IsInitialized(ctx) {
		t.Fatal("should start uninitialized")
	}
	res, err := svc.Init(ctx, "admin", "strongpassword", "Test Phone", "", "", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if res.Token == "" || !svc.IsInitialized(ctx) {
		t.Fatalf("init result invalid: %+v", res)
	}
	if _, err := svc.Init(ctx, "admin2", "anotherpassword", "", "", "", "127.0.0.1", ""); err == nil {
		t.Fatal("second init must fail")
	}
	p, err := svc.Authenticate(ctx, res.Token)
	if err != nil || p.Username != "admin" {
		t.Fatalf("authenticate: %+v err=%v", p, err)
	}

	login, err := svc.Login(ctx, "admin", "strongpassword", "", "New iPhone", "Mozilla/5.0 (iPhone)", "127.0.0.1", 0, "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login.Token == "" || login.Device.Type != "mobile" {
		t.Fatalf("login result invalid: %+v", login)
	}
	if _, err := svc.Login(ctx, "admin", "bad", "", "", "", "127.0.0.1", 0, ""); err == nil {
		t.Fatal("bad password should fail")
	}
}

func TestDeviceAuthorizationPolicies(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	init, err := svc.Init(ctx, "admin", "strongpassword", "Main PC", "", "", "127.0.0.1", "")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// Approval mode: new device requires an explicit approval.
	d, decision, err := svc.RegisterDevice(ctx, init.User.ID, "New Phone", "Mozilla/5.0 (iPhone)", "10.0.0.5", "approval", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if decision.Status != "pending" || decision.Code == "" || d.Authorized {
		t.Fatalf("approval policy not applied: %+v %+v", decision, d)
	}
	if _, err := svc.LoginWithDevice(ctx, init.User.ID, d.ID, 0); err == nil {
		t.Fatal("unauthorized device should not get a session")
	}
	approved, err := svc.ApproveByCode(ctx, decision.Code, "127.0.0.1")
	if err != nil || !approved.Authorized {
		t.Fatalf("approve by code: %+v err=%v", approved, err)
	}
	token, err := svc.LoginWithDevice(ctx, init.User.ID, d.ID, 0)
	if err != nil || token == "" {
		t.Fatalf("session after approval: %v", err)
	}

	// Whitelist mode: brand new device is denied.
	_, decision2, err := svc.RegisterDevice(ctx, init.User.ID, "Unknown", "", "10.0.0.9", "whitelist", "")
	if err != nil || decision2.Status != "denied" {
		t.Fatalf("whitelist policy not applied: %+v err=%v", decision2, err)
	}

	// Revoking a device invalidates its sessions.
	if err := svc.RevokeDevice(ctx, init.User.ID, d.ID, "127.0.0.1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); err == nil {
		t.Fatal("session should be invalid after device revocation")
	}
}

func TestPrivateIPDetection(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.2", "172.16.0.1", "::1", "169.254.1.1"} {
		if !auth.IsPrivateIP(ip) {
			t.Errorf("%s should be considered LAN", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "not-an-ip"} {
		if auth.IsPrivateIP(ip) {
			t.Errorf("%s should not be considered LAN", ip)
		}
	}
}
