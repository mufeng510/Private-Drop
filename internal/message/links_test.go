package message_test

import (
	"context"
	"testing"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/message"
	"private-drop/internal/testutil"
)

// TestLinksHydratedFromPersistence verifies that link_previews/message_links
// rows are surfaced in the message's links field.
func TestLinksHydratedFromPersistence(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := message.New(d, config.Config{}, files, events.Noop{}, audit.New(d))

	m, err := svc.Create(ctx, "u1", "d1", "look at https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.ExecContext(ctx,
		`INSERT INTO link_previews(id,url,title,description,image_url,created_at) VALUES(?,?,?,?,?,datetime('now'))`,
		"p1", "https://example.com", "Example", "Desc", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO message_links(message_id,preview_id) VALUES(?,?)`, m.ID, "p1"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Get(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Links) != 1 || got.Links[0].URL != "https://example.com" || got.Links[0].Title != "Example" {
		t.Fatalf("links not hydrated: %+v", got.Links)
	}
}
