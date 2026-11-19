package clean_test

import (
	"context"
	"testing"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/clean"
	"private-drop/internal/config"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/message"
	"private-drop/internal/notify"
	"private-drop/internal/testutil"
	"private-drop/internal/upload"
)

func TestRetentionProtectsFavoritesAndPurgesRecycle(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	n := notify.New(d, events.Noop{})
	a := audit.New(d)
	msgs := message.New(d, config.Config{}, files, events.Noop{}, a)
	ups := upload.New(d, config.Config{}, files, events.Noop{}, a, n)
	svc := clean.New(d, config.Config{}, files, msgs, ups, n, a)

	old := time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	m1, err := msgs.Create(ctx, "u1", "d1", "expired", nil)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := msgs.Create(ctx, "u1", "d1", "favorite", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Force both past their retention; favorite must still be protected.
	_, _ = d.ExecContext(ctx, `UPDATE messages SET expires_at=? WHERE id IN (?,?)`, old, m1.ID, m2.ID)
	_, _ = d.ExecContext(ctx, `UPDATE messages SET favorited=1 WHERE id=?`, m2.ID)

	rep, err := svc.Run(ctx)
	if err != nil {
		t.Fatalf("clean run: %v", err)
	}
	if rep.ExpiredMessages < 1 {
		t.Fatalf("expected expired message moved to recycle: %+v", rep)
	}
	if got, err := msgs.Get(ctx, m1.ID); err != nil || got.DeletedAt == nil {
		t.Fatalf("expired message should be in recycle: %+v err=%v", got, err)
	}
	if got, err := msgs.Get(ctx, m2.ID); err != nil || got.DeletedAt != nil {
		t.Fatalf("favorited message must not be cleaned: %+v err=%v", got, err)
	}

	// Age the recycled message past recycle retention and purge.
	_, _ = d.ExecContext(ctx, `UPDATE messages SET deleted_at=? WHERE id=?`, old, m1.ID)
	if _, err := svc.Run(ctx); err != nil {
		t.Fatalf("second clean run: %v", err)
	}
	if _, err := msgs.Get(ctx, m1.ID); err == nil {
		t.Fatal("recycled message should be permanently purged")
	}
}

func TestCleanupIsReentrantOnEmptyDB(t *testing.T) {
	d, _ := testutil.NewDB(t)
	files := file.New(d, config.Config{})
	n := notify.New(d, events.Noop{})
	a := audit.New(d)
	msgs := message.New(d, config.Config{}, files, events.Noop{}, a)
	ups := upload.New(d, config.Config{}, files, events.Noop{}, a, n)
	svc := clean.New(d, config.Config{}, files, msgs, ups, n, a)
	if _, err := svc.Run(context.Background()); err != nil {
		t.Fatalf("clean on empty db: %v", err)
	}
	if _, err := svc.Run(context.Background()); err != nil {
		t.Fatalf("second clean on empty db: %v", err)
	}
}
