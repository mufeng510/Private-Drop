package upload_test

import (
	"bytes"
	"context"
	"testing"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/notify"
	"private-drop/internal/testutil"
	"private-drop/internal/upload"
	"private-drop/internal/util"
)

func TestChunkedUploadDedupAndStatus(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	n := notify.New(d, events.Noop{})
	svc := upload.New(d, config.Config{}, files, events.Noop{}, audit.New(d), n)

	payload := bytes.Repeat([]byte("chunkdata"), 100)
	sha := util.SHA256Hex(payload)
	chunkSize := int64(256)
	total := (len(payload) + int(chunkSize) - 1) / int(chunkSize)

	res, err := svc.Init(ctx, "dev1", upload.InitParams{
		Name: "test.bin", Mime: "application/octet-stream", SHA256: sha,
		Size: int64(len(payload)), TotalChunks: total, ChunkSize: chunkSize,
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if res.ReusedAttachmentID != "" {
		t.Fatal("unexpected dedup hit on first upload")
	}
	for i := 0; i < total; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize
		if end > int64(len(payload)) {
			end = int64(len(payload))
		}
		if _, err := svc.Chunk(ctx, res.UploadID, i, bytes.NewReader(payload[start:end])); err != nil {
			t.Fatalf("chunk %d: %v", i, err)
		}
	}
	status, err := svc.Status(ctx, res.UploadID)
	if err != nil || len(status.UploadedChunks) != total {
		t.Fatalf("status: %+v err=%v", status, err)
	}
	attID, err := svc.Complete(ctx, res.UploadID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if attID == "" {
		t.Fatal("no attachment id returned")
	}
	got, err := files.Get(ctx, attID)
	if err != nil || got.Size != int64(len(payload)) {
		t.Fatalf("attachment: %+v err=%v", got, err)
	}

	// Second upload of identical content reuses the entity through init dedup.
	res2, err := svc.Init(ctx, "dev1", upload.InitParams{
		Name: "copy.bin", Mime: "application/octet-stream", SHA256: sha,
		Size: int64(len(payload)), TotalChunks: total, ChunkSize: chunkSize,
	})
	if err != nil || res2.ReusedAttachmentID == "" {
		t.Fatalf("dedup init: %+v err=%v", res2, err)
	}
}

func TestChunkOutOfRange(t *testing.T) {
	ctx := context.Background()
	d, cfg := testutil.NewDB(t)
	files := file.New(d, cfg)
	svc := upload.New(d, config.Config{}, files, events.Noop{}, audit.New(d), notify.New(d, events.Noop{}))
	res, err := svc.Init(ctx, "dev1", upload.InitParams{
		Name: "x.bin", Mime: "application/octet-stream", Size: 10, TotalChunks: 1, ChunkSize: 10,
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := svc.Chunk(ctx, res.UploadID, 5, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("out of range chunk should fail")
	}
}
