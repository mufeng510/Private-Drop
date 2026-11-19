package upload_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/notify"
	"private-drop/internal/testutil"
	"private-drop/internal/upload"
	"private-drop/internal/util"
)

func TestUploadConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := upload.New(d, cfg, files, events.Noop{}, audit.New(d), notify.New(d, events.Noop{}))
	if _, err := d.ExecContext(ctx, `UPDATE settings SET value='1' WHERE key='upload_concurrency'`); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Init(ctx, "dev1", upload.InitParams{Name: "a.bin", Size: 4, TotalChunks: 1, ChunkSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	if first.UploadID == "" {
		t.Fatal("first init failed")
	}
	if _, err := svc.Init(ctx, "dev1", upload.InitParams{Name: "b.bin", Size: 4, TotalChunks: 1, ChunkSize: 4}); err == nil {
		t.Fatal("second init should be rejected when concurrency limit is 1")
	}
}

func TestUploadBoundToMessageAndCancelled(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := upload.New(d, cfg, files, events.Noop{}, audit.New(d), notify.New(d, events.Noop{}))
	_, _ = d.ExecContext(ctx, `INSERT INTO messages(id,user_id,device_id,text,created_at,updated_at) VALUES(?,?,?,?,?,?)`,
		"m1", "u1", "d1", "x", util.Now(), util.Now())

	res, err := svc.Init(ctx, "dev1", upload.InitParams{
		Name: "bound.bin", Size: 4, TotalChunks: 1, ChunkSize: 4, MessageID: "m1", RelativePath: "a/b.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := svc.Get(ctx, res.UploadID)
	if err != nil || u.MessageID != "m1" || u.OriginalPath != "a/b.bin" {
		t.Fatalf("upload not bound: %+v err=%v", u, err)
	}
	n, err := svc.CancelForMessage(ctx, "m1")
	if err != nil || n != 1 {
		t.Fatalf("cancel for message: n=%d err=%v", n, err)
	}
	u, _ = svc.Get(ctx, res.UploadID)
	if u.Status != "cancelled" {
		t.Fatalf("bound upload should be cancelled, got %s", u.Status)
	}
}

func TestUploadProgressFieldsAndStaleReaping(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := upload.New(d, cfg, files, events.Noop{}, audit.New(d), notify.New(d, events.Noop{}))

	payload := []byte("abcdef")
	res, err := svc.Init(ctx, "dev1", upload.InitParams{
		Name: "p.bin", Size: int64(len(payload)), TotalChunks: 3, ChunkSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Chunk(ctx, res.UploadID, 0, bytes.NewReader(payload[:2])); err != nil {
		t.Fatal(err)
	}
	u, _ := svc.Get(ctx, res.UploadID)
	if u.BytesUploaded != 2 || u.BytesRemaining != 4 || u.Speed <= 0 {
		t.Fatalf("progress fields: %+v", u)
	}

	// Make the task look inactive and verify it is failed + chunks removed.
	old := util.Format(time.Now().UTC().Add(-48 * time.Hour))
	_, _ = d.ExecContext(ctx, `UPDATE uploads SET updated_at=? WHERE id=?`, old, res.UploadID)
	dir := cfg.TmpDir() + "/" + res.UploadID
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("chunk dir should exist before reap: %v", err)
	}
	n, err := svc.ReapInactive(ctx, 24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("reap inactive: n=%d err=%v", n, err)
	}
	u, _ = svc.Get(ctx, res.UploadID)
	if u.Status != "failed" {
		t.Fatalf("inactive upload should be failed, got %s", u.Status)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("chunk dir should be removed after reap: %v", err)
	}
}
