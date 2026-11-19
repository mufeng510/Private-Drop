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
	"private-drop/internal/util"
)

// TestPaginationNoDuplicatesWithTimestampTies is the regression test for the
// broken timestamp-only cursor: when many messages share a created_at, keyset
// pagination must still cover every row exactly once.
func TestPaginationNoDuplicatesWithTimestampTies(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := message.New(d, config.Config{}, files, events.Noop{}, audit.New(d))

	for i := 0; i < 9; i++ {
		if _, err := svc.Create(ctx, "u1", "d1", "msg", nil); err != nil {
			t.Fatal(err)
		}
	}
	// Force an exact timestamp tie across all rows.
	tie := util.Format(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	if _, err := d.ExecContext(ctx, `UPDATE messages SET created_at=?`, tie); err != nil {
		t.Fatal(err)
	}

	for _, sort := range []string{"newest", "favorite", "pinned"} {
		seen := map[string]bool{}
		cursor := ""
		pages := 0
		for {
			items, next, err := svc.List(ctx, message.MessageFilter{Sort: sort, Cursor: cursor, Limit: 3})
			if err != nil {
				t.Fatalf("list %s: %v", sort, err)
			}
			pages++
			for _, m := range items {
				if seen[m.ID] {
					t.Fatalf("sort %s returned duplicate message %s", sort, m.ID)
				}
				seen[m.ID] = true
			}
			if next == nil {
				break
			}
			cursor = *next
			if pages > 20 {
				t.Fatalf("sort %s pagination did not terminate", sort)
			}
		}
		if len(seen) != 9 {
			t.Fatalf("sort %s covered %d messages, want 9", sort, len(seen))
		}
	}
}

// TestEditPreservesCustomExpiry is the regression test for text edits
// recomputing expires_at.
func TestEditPreservesCustomExpiry(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := message.New(d, config.Config{}, files, events.Noop{}, audit.New(d))

	m, err := svc.Create(ctx, "u1", "d1", "original", nil)
	if err != nil {
		t.Fatal(err)
	}
	custom := util.Format(time.Now().UTC().Add(3 * 24 * time.Hour))
	if _, err := d.ExecContext(ctx, `UPDATE messages SET expires_at=? WHERE id=?`, custom, m.ID); err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Update(ctx, m.ID, ptr("edited"), ptr(true), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ExpiresAt == nil || *updated.ExpiresAt != custom {
		t.Fatalf("edit changed expires_at: got %v want %s", updated.ExpiresAt, custom)
	}
}

// TestBulkRestoreMode verifies the restore action honors its mode parameter.
func TestBulkRestoreMode(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := message.New(d, config.Config{}, files, events.Noop{}, audit.New(d))

	old := util.Format(time.Now().UTC().Add(-40 * 24 * time.Hour))
	makeRecycled := func() string {
		m, err := svc.Create(ctx, "u1", "d1", "recycle", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.ExecContext(ctx, `UPDATE messages SET created_at=?,updated_at=? WHERE id=?`, old, old, m.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.SoftDelete(ctx, m.ID); err != nil {
			t.Fatal(err)
		}
		return m.ID
	}

	originalID := makeRecycled()
	resetID := makeRecycled()

	if _, err := svc.Bulk(ctx, []string{originalID}, "restore", "", "original", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Bulk(ctx, []string{resetID}, "restore", "", "reset", ""); err != nil {
		t.Fatal(err)
	}
	orig, _ := svc.Get(ctx, originalID)
	reset, _ := svc.Get(ctx, resetID)
	if orig.ExpiresAt == nil || reset.ExpiresAt == nil {
		t.Fatalf("restored messages need expiry: %+v %+v", orig, reset)
	}
	if *orig.ExpiresAt >= *reset.ExpiresAt {
		t.Fatalf("mode original should keep old expiry (%s), reset should push it to now (%s)", *orig.ExpiresAt, *reset.ExpiresAt)
	}
}

// TestTagCreateAttachList covers the tag unit flow required by the audit.
func TestTagCreateAttachList(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := message.New(d, config.Config{}, files, events.Noop{}, audit.New(d))

	tag, err := svc.CreateTag(ctx, "work", "#00f", "briefcase", 2)
	if err != nil || tag.ID == "" {
		t.Fatalf("create tag: %+v err=%v", tag, err)
	}
	m, err := svc.Create(ctx, "u1", "d1", "tagged", nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{tag.ID}
	if _, err := svc.Update(ctx, m.ID, nil, nil, nil, &ids); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tg := range got.Tags {
		if tg.ID == tag.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("tag not attached to message: %+v", got.Tags)
	}
	tags, err := svc.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, tg := range tags {
		if tg.ID == tag.ID {
			listed = true
		}
	}
	if !listed {
		t.Fatal("created tag missing from tag list")
	}
}
