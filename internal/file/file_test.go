package file_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"private-drop/internal/file"
	"private-drop/internal/testutil"
	"private-drop/internal/util"
)

func TestDedupAndRefCount(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	svc := file.New(d, cfg)

	payload := []byte("deduplicate me")
	sha := util.SHA256Hex(payload)
	entity1, err := svc.StoreEntity(ctx, strings.NewReader(string(payload)), sha)
	if err != nil {
		t.Fatalf("store entity: %v", err)
	}
	var ref int
	_ = d.QueryRowContext(ctx, `SELECT ref_count FROM entity_files WHERE id=?`, entity1).Scan(&ref)
	if ref != 0 {
		t.Fatalf("new entity ref_count = %d, want 0", ref)
	}

	att1, err := svc.CreateAttachment(ctx, entity1, "a.txt", "text/plain", sha, int64(len(payload)), 0, 0)
	if err != nil {
		t.Fatalf("create attachment 1: %v", err)
	}
	// Same content reuses the same entity even under a different name.
	entity2, err := svc.StoreEntity(ctx, strings.NewReader(string(payload)), sha)
	if err != nil || entity2 != entity1 {
		t.Fatalf("dedup failed: %s vs %s err=%v", entity1, entity2, err)
	}
	att2, err := svc.CreateAttachment(ctx, entity2, "copy.txt", "text/plain", sha, int64(len(payload)), 0, 0)
	if err != nil {
		t.Fatalf("create attachment 2: %v", err)
	}
	_ = d.QueryRowContext(ctx, `SELECT ref_count FROM entity_files WHERE id=?`, entity1).Scan(&ref)
	if ref != 2 {
		t.Fatalf("ref_count = %d, want 2", ref)
	}
	if reused, ok := svc.FindReusableAttachment(ctx, sha); !ok || reused == "" {
		t.Fatal("expected reusable attachment for known sha")
	}

	path, err := svc.EntityPath(ctx, att1.ID)
	if err != nil {
		t.Fatalf("entity path: %v", err)
	}
	if err := svc.Purge(ctx, att1.ID); err != nil {
		t.Fatalf("purge 1: %v", err)
	}
	_ = d.QueryRowContext(ctx, `SELECT ref_count FROM entity_files WHERE id=?`, entity1).Scan(&ref)
	if ref != 1 {
		t.Fatalf("ref_count after first purge = %d, want 1", ref)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("physical file removed too early: %v", err)
	}

	if err := svc.Purge(ctx, att2.ID); err != nil {
		t.Fatalf("purge 2: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("physical file should be gone after last purge, stat err=%v", err)
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd": "passwd",
		"a/b/c.txt":        "c.txt",
		"normal.txt":       "normal.txt",
		"":                 "unnamed",
	}
	for in, want := range cases {
		if got := file.SanitizeFileName(in); got != want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
