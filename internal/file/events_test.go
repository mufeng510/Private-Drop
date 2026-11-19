package file_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"private-drop/internal/file"
	"private-drop/internal/testutil"
	"private-drop/internal/util"
)

type captureSink struct {
	mu     sync.Mutex
	events []string
}

func (c *captureSink) Publish(t string, _ any) {
	c.mu.Lock()
	c.events = append(c.events, t)
	c.mu.Unlock()
}

func (c *captureSink) has(t string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e == t {
			return true
		}
	}
	return false
}

func (c *captureSink) clear() {
	c.mu.Lock()
	c.events = nil
	c.mu.Unlock()
}

// TestFileStateEventsPublished is the regression test for the audit finding
// that attachment updates/deletes/restores published nothing over WS.
func TestFileStateEventsPublished(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	sink := &captureSink{}
	svc := file.NewWithEvents(d, cfg, sink)

	sha := util.SHA256Hex([]byte("event payload"))
	entity, err := svc.StoreEntity(ctx, strings.NewReader("event payload"), sha)
	if err != nil {
		t.Fatal(err)
	}
	att, err := svc.CreateAttachment(ctx, entity, "a.txt", "text/plain", sha, 13, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	sink.clear()
	fav := true
	if _, err := svc.Update(ctx, att.ID, &fav, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !sink.has("file.updated") {
		t.Fatal("Update must publish file.updated")
	}

	sink.clear()
	if err := svc.SoftDelete(ctx, att.ID); err != nil {
		t.Fatal(err)
	}
	if !sink.has("file.deleted") {
		t.Fatal("SoftDelete must publish file.deleted")
	}

	sink.clear()
	if err := svc.Restore(ctx, att.ID, "original"); err != nil {
		t.Fatal(err)
	}
	if !sink.has("file.restored") {
		t.Fatal("Restore must publish file.restored")
	}

	// Restore returns not-found for an attachment that is not in the bin.
	if err := svc.Restore(ctx, att.ID); err == nil {
		t.Fatal("Restore of a non-deleted attachment should fail")
	}
}
