package ws

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"private-drop/internal/db"
	"private-drop/internal/util"
)

// DBEventStore persists WS events so reconnecting clients can replay the
// exact events they missed, even across a server restart.
type DBEventStore struct {
	db *db.DB
}

func NewDBEventStore(d *db.DB) *DBEventStore { return &DBEventStore{db: d} }

// Append stores one event and returns it with its assigned sequence number.
func (s *DBEventStore) Append(typeName, deviceID string, data any) (Event, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	ts := util.Now()
	res, err := s.db.ExecContext(context.Background(),
		`INSERT INTO ws_events(type,data,ts,device_id) VALUES(?,?,?,?)`,
		typeName, string(b), ts, deviceID)
	if err != nil {
		return Event{}, err
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}
	return Event{Seq: seq, Type: typeName, Data: json.RawMessage(b), TS: ts, DeviceID: deviceID}, nil
}

// Since returns stored events after the given cursor, oldest first. The cursor
// may be a numeric sequence (seq) or an RFC3339/RFC3339Nano timestamp.
func (s *DBEventStore) Since(cursor string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Close() error
			Err() error
		}
		err error
	)
	if seq, e := parseInt64(cursor); e == nil {
		rows, err = s.db.QueryContext(context.Background(),
			`SELECT seq,type,data,ts,device_id FROM ws_events WHERE seq>? ORDER BY seq ASC LIMIT ?`, seq, limit)
	} else if t, e := util.ParseTime(cursor); e == nil {
		rows, err = s.db.QueryContext(context.Background(),
			`SELECT seq,type,data,ts,device_id FROM ws_events WHERE ts>? ORDER BY seq ASC LIMIT ?`, util.Format(t), limit)
	} else {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var data, dev string
		if err := rows.Scan(&e.Seq, &e.Type, &data, &e.TS, &dev); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		e.DeviceID = dev
		out = append(out, e)
	}
	return out, rows.Err()
}

// MaxSeq returns the highest persisted sequence number (0 when empty).
func (s *DBEventStore) MaxSeq() int64 {
	var n int64
	_ = s.db.QueryRowContext(context.Background(), `SELECT COALESCE(MAX(seq),0) FROM ws_events`).Scan(&n)
	return n
}

// Trim removes events older than the given duration, keeping the durable log
// bounded. A personal instance generates few events, but this prevents
// unbounded growth over years of use.
func (s *DBEventStore) Trim(ctx context.Context, olderThan time.Duration) error {
	cutoff := util.Format(time.Now().UTC().Add(-olderThan))
	_, err := s.db.ExecContext(ctx, `DELETE FROM ws_events WHERE ts<?`, cutoff)
	return err
}

func parseInt64(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }
