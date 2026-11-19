// Package search implements SQL-based full-text search across message text
// and attachment filenames with filters, without an external index.
package search

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"private-drop/internal/db"
	"private-drop/internal/file"
	"private-drop/internal/message"
)

type Service struct {
	db       *db.DB
	messages *message.Service
	files    *file.Service
}

func New(d *db.DB, m *message.Service, f *file.Service) *Service {
	return &Service{db: d, messages: m, files: f}
}

type Result struct {
	Kind       string           `json:"kind"` // "message" | "file"
	ID         string           `json:"id"`
	CreatedAt  string           `json:"createdAt"`
	Message    *message.Message `json:"message,omitempty"`
	Attachment *file.Attachment `json:"attachment,omitempty"`
}

type Filter struct {
	Q, Type, Ext, Tag, Device, From, To string
	MinSize, MaxSize                    int64
	Favorite, Pinned                    *bool
	Cursor                              string
	Limit                               int
}

// searchCursor identifies the last result of a page by its full sort tuple so
// new/deleted items between pages cannot cause duplicates or skips.
type searchCursor struct {
	C  string `json:"c"` // createdAt
	K  string `json:"k"` // kind: message|file
	ID string `json:"i"`
}

func encodeSearchCursor(c searchCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeSearchCursor returns the keyset position and whether it is valid.
// Legacy base64-of-offset cursors are still accepted.
func decodeSearchCursor(s string) (searchCursor, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return searchCursor{}, false
	}
	var c searchCursor
	if json.Unmarshal(b, &c) == nil && c.ID != "" {
		return c, true
	}
	return searchCursor{}, false
}

func offsetFromLegacyCursor(c string) int {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(string(b))
	if n < 0 {
		return 0
	}
	return n
}

// Search returns a mixed page of matching messages and files.
func (s *Service) Search(ctx context.Context, f Filter) ([]Result, *string, error) {
	like := "%" + strings.TrimSpace(f.Q) + "%"
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var results []Result

	// Messages.
	mq := `SELECT id,created_at FROM messages WHERE deleted_at IS NULL`
	var ma []any
	if f.Q != "" {
		mq += ` AND text LIKE ?`
		ma = append(ma, like)
	}
	if f.From != "" {
		mq += ` AND created_at>=?`
		ma = append(ma, f.From)
	}
	if f.To != "" {
		mq += ` AND created_at<=?`
		ma = append(ma, f.To)
	}
	if f.Favorite != nil {
		mq += ` AND favorited=?`
		ma = append(ma, db.IntBool(*f.Favorite))
	}
	if f.Pinned != nil {
		mq += ` AND pinned=?`
		ma = append(ma, db.IntBool(*f.Pinned))
	}
	if f.Device != "" {
		mq += ` AND device_id=?`
		ma = append(ma, f.Device)
	}
	if f.Tag != "" {
		mq += ` AND id IN (SELECT mt.message_id FROM message_tags mt JOIN tags t ON t.id=mt.tag_id WHERE t.name=? OR t.id=?)`
		ma = append(ma, f.Tag, f.Tag)
	}
	mrows, err := s.db.QueryContext(ctx, mq, ma...)
	if err != nil {
		return nil, nil, err
	}
	type hit struct{ id, created string }
	var mhits []hit
	for mrows.Next() {
		var h hit
		if err := mrows.Scan(&h.id, &h.created); err != nil {
			mrows.Close()
			return nil, nil, err
		}
		mhits = append(mhits, h)
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, nil, err
	}
	for _, h := range mhits {
		m, err := s.messages.Get(ctx, h.id)
		if err != nil {
			continue
		}
		if f.Type != "" && !messageMatchesType(m, f.Type) {
			continue
		}
		results = append(results, Result{Kind: "message", ID: m.ID, CreatedAt: m.CreatedAt, Message: &m})
	}

	// Files.
	fq := `SELECT id,uploaded_at FROM attachments WHERE deleted_at IS NULL`
	var fa []any
	if f.Q != "" {
		fq += ` AND name LIKE ?`
		fa = append(fa, like)
	}
	if f.From != "" {
		fq += ` AND uploaded_at>=?`
		fa = append(fa, f.From)
	}
	if f.To != "" {
		fq += ` AND uploaded_at<=?`
		fa = append(fa, f.To)
	}
	if f.Favorite != nil {
		fq += ` AND favorited=?`
		fa = append(fa, db.IntBool(*f.Favorite))
	}
	if f.MinSize > 0 {
		fq += ` AND size>=?`
		fa = append(fa, f.MinSize)
	}
	if f.MaxSize > 0 {
		fq += ` AND size<=?`
		fa = append(fa, f.MaxSize)
	}
	if f.Ext != "" {
		fq += ` AND LOWER(name) LIKE ?`
		fa = append(fa, "%."+strings.ToLower(strings.TrimPrefix(f.Ext, ".")))
	}
	if f.Type != "" {
		fq += ` AND (LOWER(mime) LIKE ? OR LOWER(name) LIKE ?)`
		fa = append(fa, "%"+strings.ToLower(f.Type)+"%", "%"+strings.ToLower(f.Type)+"%")
	}
	if f.Device != "" {
		fq += ` AND id IN (SELECT ma.attachment_id FROM message_attachments ma JOIN messages m ON m.id=ma.message_id WHERE m.device_id=?)`
		fa = append(fa, f.Device)
	}
	if f.Tag != "" {
		fq += ` AND id IN (SELECT at.attachment_id FROM attachment_tags at JOIN tags t ON t.id=at.tag_id WHERE t.name=? OR t.id=?)`
		fa = append(fa, f.Tag, f.Tag)
	}
	frows, err := s.db.QueryContext(ctx, fq, fa...)
	if err != nil {
		return nil, nil, err
	}
	var fhits []hit
	for frows.Next() {
		var h hit
		if err := frows.Scan(&h.id, &h.created); err != nil {
			frows.Close()
			return nil, nil, err
		}
		fhits = append(fhits, h)
	}
	frows.Close()
	if err := frows.Err(); err != nil {
		return nil, nil, err
	}
	for _, h := range fhits {
		a, err := s.files.Get(ctx, h.id)
		if err != nil {
			continue
		}
		results = append(results, Result{Kind: "file", ID: a.ID, CreatedAt: h.created, Attachment: &a})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].CreatedAt != results[j].CreatedAt {
			return results[i].CreatedAt > results[j].CreatedAt
		}
		if results[i].Kind != results[j].Kind {
			return results[i].Kind < results[j].Kind
		}
		return results[i].ID < results[j].ID
	})
	start := 0
	if f.Cursor != "" {
		if c, ok := decodeSearchCursor(f.Cursor); ok {
			for i, r := range results {
				if r.CreatedAt == c.C && r.Kind == c.K && r.ID == c.ID {
					start = i + 1
					break
				}
			}
		} else {
			start = offsetFromLegacyCursor(f.Cursor)
		}
	}
	if start >= len(results) {
		return []Result{}, nil, nil
	}
	end := start + limit
	if end > len(results) {
		end = len(results)
	}
	page := results[start:end]
	var next *string
	if end < len(results) {
		last := page[len(page)-1]
		c := encodeSearchCursor(searchCursor{C: last.CreatedAt, K: last.Kind, ID: last.ID})
		next = &c
	}
	return page, next, nil
}

func messageMatchesType(m message.Message, typ string) bool {
	t := strings.ToLower(typ)
	for _, a := range m.Attachments {
		if strings.Contains(strings.ToLower(a.Type), t) || strings.Contains(strings.ToLower(a.Mime), t) {
			return true
		}
	}
	if t == "text" || t == "文本" {
		return len(m.Attachments) == 0
	}
	return false
}
