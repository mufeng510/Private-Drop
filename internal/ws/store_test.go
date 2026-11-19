package ws_test

import (
	"context"
	"testing"

	"private-drop/internal/testutil"
	"private-drop/internal/ws"
)

func TestDBEventStoreSeqAndTimestampCursors(t *testing.T) {
	d, _ := testutil.NewDB(t)
	store := ws.NewDBEventStore(d)

	e1, err := store.Append("message.created", "dev1", map[string]any{"id": "a"})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := store.Append("message.deleted", "dev2", map[string]any{"id": "b"})
	if err != nil {
		t.Fatal(err)
	}
	if e2.Seq <= e1.Seq {
		t.Fatalf("seq not monotonic: %d then %d", e1.Seq, e2.Seq)
	}

	// Numeric cursor.
	after, err := store.Since(itoa(e1.Seq), 10)
	if err != nil || len(after) != 1 || after[0].Seq != e2.Seq {
		t.Fatalf("numeric cursor: %+v err=%v", after, err)
	}
	if after[0].DeviceID != "dev2" {
		t.Fatalf("device id not persisted: %+v", after[0])
	}

	// RFC3339Nano timestamp cursor.
	afterTS, err := store.Since(e1.TS, 10)
	if err != nil || len(afterTS) != 1 || afterTS[0].Seq != e2.Seq {
		t.Fatalf("timestamp cursor: %+v err=%v", afterTS, err)
	}

	if err := store.Trim(context.Background(), -1); err != nil {
		t.Fatalf("trim: %v", err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
