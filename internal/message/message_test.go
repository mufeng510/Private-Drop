package message_test

import (
	"context"
	"testing"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/message"
	"private-drop/internal/testutil"
)

func newService(t *testing.T) (*message.Service, *file.Service) {
	t.Helper()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	return message.New(d, config.Config{}, files, events.Noop{}, audit.New(d)), files
}

func TestMessageLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	m, err := svc.Create(ctx, "u1", "d1", "hello", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Text != "hello" || m.Pinned || m.Favorited || m.ExpiresAt == nil {
		t.Fatalf("unexpected created message: %+v", m)
	}
	if m.Attachments == nil || m.Tags == nil || m.Links == nil {
		t.Fatalf("message should be fully hydrated: %+v", m)
	}
	createdAt, expiresAt := m.CreatedAt, *m.ExpiresAt

	// Edit text keeps created time and retention.
	updated, err := svc.Update(ctx, m.ID, ptr("edited"), nil, nil, nil)
	if err != nil {
		t.Fatalf("update text: %v", err)
	}
	if updated.Text != "edited" || updated.CreatedAt != createdAt || *updated.ExpiresAt != expiresAt {
		t.Fatalf("edit changed immutable fields: %+v", updated)
	}

	// Pin.
	updated, err = svc.Update(ctx, m.ID, nil, ptr(true), nil, nil)
	if err != nil || !updated.Pinned {
		t.Fatalf("pin failed: %+v err=%v", updated, err)
	}

	// Favorite removes expiry; unfavorite restores it.
	updated, err = svc.Update(ctx, m.ID, nil, nil, ptr(true), nil)
	if err != nil || updated.ExpiresAt != nil {
		t.Fatalf("favorite should clear expiry: %+v err=%v", updated, err)
	}
	updated, err = svc.Update(ctx, m.ID, nil, nil, ptr(false), nil)
	if err != nil || updated.ExpiresAt == nil {
		t.Fatalf("unfavorite should restore expiry: %+v err=%v", updated, err)
	}

	// Soft delete hides from list, keeps detail readable.
	if err := svc.SoftDelete(ctx, m.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	list, _, err := svc.List(ctx, message.MessageFilter{})
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted message still listed: %d err=%v", len(list), err)
	}
	got, err := svc.Get(ctx, m.ID)
	if err != nil || got.DeletedAt == nil {
		t.Fatalf("deleted message detail lost: %+v err=%v", got, err)
	}

	// Restore with original retention.
	restored, err := svc.Restore(ctx, m.ID, "original")
	if err != nil || restored.DeletedAt != nil {
		t.Fatalf("restore failed: %+v err=%v", restored, err)
	}
	list, _, _ = svc.List(ctx, message.MessageFilter{})
	if len(list) != 1 {
		t.Fatalf("restored message not listed: %d", len(list))
	}
}

func TestBulkAndFavoriteFirstSort(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	a, _ := svc.Create(ctx, "u1", "d1", "a", nil)
	b, _ := svc.Create(ctx, "u1", "d1", "b", nil)
	res, err := svc.Bulk(ctx, []string{a.ID, b.ID}, "favorite", "", "", "")
	if err != nil || res.Affected != 2 {
		t.Fatalf("bulk favorite: %+v err=%v", res, err)
	}
	res, err = svc.Bulk(ctx, []string{a.ID, b.ID}, "pin", "", "", "")
	if err != nil || res.Affected != 2 {
		t.Fatalf("bulk pin: %+v err=%v", res, err)
	}
	list, _, err := svc.List(ctx, message.MessageFilter{Sort: "favorite"})
	if err != nil || len(list) != 2 || !list[0].Favorited {
		t.Fatalf("favorite sort failed: %+v err=%v", list, err)
	}
}

func TestExpiryFrom(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := message.ExpiryFrom(created, 30)
	want := created.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("ExpiryFrom = %v, want %v", got, want)
	}
	if !message.ExpiryFrom(created, 0).IsZero() {
		t.Fatal("non-positive retention should mean never expire")
	}
}

func ptr[T any](v T) *T { return &v }
