// Package message implements message CRUD, tags, favorites, pinning, recycle
// bin operations and retention helpers.
package message

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/db"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/linkpreview"
	"private-drop/internal/upload"
	"private-drop/internal/util"
)

var (
	ErrMessageNotFound = errors.New("message not found")
	ErrTagNotFound     = errors.New("tag not found")
)

type DeviceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Tag struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Icon     string `json:"icon"`
	Sort     int    `json:"sort"`
	IsSystem bool   `json:"isSystem"`
}

type LinkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

type Message struct {
	ID          string            `json:"id"`
	Text        string            `json:"text"`
	Pinned      bool              `json:"pinned"`
	Favorited   bool              `json:"favorited"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
	ExpiresAt   *string           `json:"expiresAt"`
	DeletedAt   *string           `json:"deletedAt"`
	Device      DeviceRef         `json:"device"`
	Attachments []file.Attachment `json:"attachments"`
	Tags        []Tag             `json:"tags"`
	Links       []LinkPreview     `json:"links"`
}

type Service struct {
	db      *db.DB
	cfg     config.Config
	files   *file.Service
	events  events.Sink
	audit   *audit.Service
	uploads *upload.Service
	links   *linkpreview.Previewer
}

func New(d *db.DB, cfg config.Config, files *file.Service, sink events.Sink, a *audit.Service) *Service {
	return &Service{db: d, cfg: cfg, files: files, events: sink, audit: a}
}

// SetUploads wires the upload service so deleting/expiring a message cancels
// its bound uploads. Set after both services are constructed.
func (s *Service) SetUploads(u *upload.Service) { s.uploads = u }

// SetLinkPreviewer enables automatic link extraction/persistence on create
// and edit when the link_preview_enabled setting is on.
func (s *Service) SetLinkPreviewer(p *linkpreview.Previewer) { s.links = p }

// cancelBoundUploads cancels any active uploads bound to a message.
func (s *Service) cancelBoundUploads(ctx context.Context, messageID string) {
	if s.uploads != nil {
		_, _ = s.uploads.CancelForMessage(ctx, messageID)
	}
}

func (s *Service) retentionDays(ctx context.Context) int {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='retention_days'`).Scan(&v); err != nil {
		return 30
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 30
	}
	return n
}

// Create inserts a message and links its attachments.
func (s *Service) Create(ctx context.Context, userID, deviceID, text string, attachmentIDs []string) (Message, error) {
	var m Message
	now := time.Now().UTC()
	m = Message{
		ID: util.NewID(), Text: text, CreatedAt: util.Now(), UpdatedAt: util.Now(),
		Attachments: []file.Attachment{}, Tags: []Tag{}, Links: []LinkPreview{},
	}
	days := s.retentionDays(ctx)
	if exp := ExpiryFrom(now, days); !exp.IsZero() {
		v := util.Format(exp)
		m.ExpiresAt = &v
	}
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO messages(id,user_id,device_id,text,pinned,favorited,created_at,updated_at,expires_at)
            VALUES(?,?,?,?,0,0,?,?,?)`,
			m.ID, userID, deviceID, text, m.CreatedAt, m.UpdatedAt, m.ExpiresAt); err != nil {
			return err
		}
		for _, aid := range attachmentIDs {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM attachments WHERE id=?`, aid).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return file.ErrAttachmentNotFound
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO message_attachments(message_id,attachment_id) VALUES(?,?)`, m.ID, aid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	s.persistLinks(ctx, m.ID, text)
	full, err := s.Get(ctx, m.ID)
	if err != nil {
		return Message{}, err
	}
	s.events.Publish("message.created", full)
	return full, nil
}

func (s *Service) linkPreviewEnabled(ctx context.Context) bool {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='link_preview_enabled'`).Scan(&v); err != nil {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(v), "false")
}

// persistLinks resolves URLs in a message's text and stores their metadata so
// hydrate() can include them. Failures are non-fatal: a message is always
// saved even when a preview cannot be fetched.
func (s *Service) persistLinks(ctx context.Context, messageID, text string) {
	if s.links == nil || strings.TrimSpace(text) == "" || !s.linkPreviewEnabled(ctx) {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, raw := range linkpreview.ExtractURLs(text, 3) {
		res, err := s.links.Preview(pctx, raw)
		if err != nil {
			continue
		}
		if res.ImageURL != "" && linkpreview.ValidateImageURL(res.ImageURL) != nil {
			res.ImageURL = ""
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO link_previews(id,url,title,description,image_url,created_at) VALUES(?,?,?,?,?,?)`,
			util.NewID(), raw, res.Title, res.Description, res.ImageURL, util.Now())
		if err != nil {
			continue
		}
		var pid string
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM link_previews WHERE url=?`, raw).Scan(&pid); err != nil {
			continue
		}
		_, _ = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO message_links(message_id,preview_id) VALUES(?,?)`, messageID, pid)
	}
}

const msgCols = `id,text,pinned,favorited,created_at,updated_at,expires_at,deleted_at,device_id`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var pinned, fav int
	var exp, del sql.NullString
	err := row.Scan(&m.ID, &m.Text, &pinned, &fav, &m.CreatedAt, &m.UpdatedAt, &exp, &del, &m.Device.ID)
	if err != nil {
		return Message{}, err
	}
	m.Pinned, m.Favorited = db.Bool(int64(pinned)), db.Bool(int64(fav))
	m.Attachments = []file.Attachment{}
	m.Tags = []Tag{}
	m.Links = []LinkPreview{}
	if exp.Valid {
		v := exp.String
		m.ExpiresAt = &v
	}
	if del.Valid {
		v := del.String
		m.DeletedAt = &v
	}
	return m, nil
}

// hydrate fills device info, attachments, tags and link previews.
func (s *Service) hydrate(ctx context.Context, list []Message) error {
	for i := range list {
		m := &list[i]
		_ = s.db.QueryRowContext(ctx, `SELECT name,type FROM devices WHERE id=?`, m.Device.ID).
			Scan(&m.Device.Name, &m.Device.Type)

		rows, err := s.db.QueryContext(ctx,
			`SELECT attachment_id FROM message_attachments WHERE message_id=? ORDER BY rowid`, m.ID)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		m.Attachments, err = s.files.GetMany(ctx, ids)
		if err != nil {
			return err
		}

		trows, err := s.db.QueryContext(ctx, `
            SELECT t.id,t.name,t.color,t.icon,t.sort,t.is_system FROM tags t
            JOIN message_tags mt ON mt.tag_id=t.id WHERE mt.message_id=? ORDER BY t.sort,t.name`, m.ID)
		if err != nil {
			return err
		}
		for trows.Next() {
			var t Tag
			var sys int
			if err := trows.Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &t.Sort, &sys); err != nil {
				trows.Close()
				return err
			}
			t.IsSystem = db.Bool(int64(sys))
			m.Tags = append(m.Tags, t)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return err
		}
		m.Tags = append(m.Tags, deriveSystemTags(m)...)

		lrows, err := s.db.QueryContext(ctx, `
            SELECT p.url,p.title,p.description,p.image_url FROM link_previews p
            JOIN message_links ml ON ml.preview_id=p.id WHERE ml.message_id=?`, m.ID)
		if err == nil {
			for lrows.Next() {
				var l LinkPreview
				if err := lrows.Scan(&l.URL, &l.Title, &l.Description, &l.ImageURL); err != nil {
					lrows.Close()
					return err
				}
				m.Links = append(m.Links, l)
			}
			lrows.Close()
		}
	}
	return nil
}

func deriveSystemTags(m *Message) []Tag {
	seen := map[string]bool{}
	out := []Tag{}
	for _, t := range m.Tags {
		seen[t.ID] = true
	}
	add := func(id, name string) {
		if !seen[id] {
			out = append(out, Tag{ID: id, Name: name, IsSystem: true})
		}
	}
	if len(m.Attachments) == 0 {
		add("sys-text", "文本")
	}
	for _, a := range m.Attachments {
		switch a.Type {
		case "image":
			add("sys-image", "图片")
		case "video":
			add("sys-video", "视频")
		case "audio":
			add("sys-audio", "音频")
		default:
			add("sys-file", "文件")
		}
	}
	return out
}

func (s *Service) Get(ctx context.Context, id string) (Message, error) {
	m, err := scanMessage(s.db.QueryRowContext(ctx,
		`SELECT `+msgCols+` FROM messages WHERE id=?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, ErrMessageNotFound
		}
		return Message{}, err
	}
	list := []Message{m}
	if err := s.hydrate(ctx, list); err != nil {
		return Message{}, err
	}
	return list[0], nil
}

type MessageFilter struct {
	Cursor, Sort, Type, Tag, From, To string
	Favorite                          *bool
	Limit                             int
	IncludeDeleted                    bool
}

// msgCursor encodes the full sort tuple plus an id tiebreaker so pagination
// neither skips nor repeats rows when timestamps tie or state changes.
type msgCursor struct {
	F  int    `json:"f"` // favorited, for the "favorite" sort
	P  int    `json:"p"` // pinned, for the "pinned" sort
	C  string `json:"c"` // created_at
	ID string `json:"i"`
}

func encodeMsgCursor(c msgCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeMsgCursor accepts the new composite cursor and the legacy raw base64
// timestamp cursor so existing clients keep working.
func decodeMsgCursor(s string) (msgCursor, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return msgCursor{}, false
	}
	var c msgCursor
	if json.Unmarshal(b, &c) == nil && c.C != "" {
		return c, true
	}
	ts := string(b)
	if _, err := util.ParseTime(ts); err == nil {
		return msgCursor{C: ts}, true
	}
	return msgCursor{}, false
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

// List returns a page of messages using stable keyset pagination.
func (s *Service) List(ctx context.Context, f MessageFilter) ([]Message, *string, error) {
	q := `SELECT ` + msgCols + ` FROM messages WHERE 1=1`
	var args []any
	if !f.IncludeDeleted {
		q += ` AND deleted_at IS NULL`
	}
	if f.Favorite != nil {
		q += ` AND favorited=?`
		args = append(args, db.IntBool(*f.Favorite))
	}
	if f.From != "" {
		q += ` AND created_at>=?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND created_at<=?`
		args = append(args, f.To)
	}
	if f.Tag != "" {
		q += ` AND id IN (SELECT mt.message_id FROM message_tags mt JOIN tags t ON t.id=mt.tag_id WHERE t.name=? OR t.id=?)`
		args = append(args, f.Tag, f.Tag)
	}
	if f.Type != "" {
		q += ` AND EXISTS (SELECT 1 FROM message_attachments ma JOIN attachments a ON a.id=ma.attachment_id
              WHERE ma.message_id=messages.id AND (LOWER(a.mime) LIKE ? OR LOWER(a.name) LIKE ?))`
		t := strings.ToLower(f.Type)
		args = append(args, "%"+t+"%", "%"+t+"%")
	}
	if f.Cursor != "" {
		if c, ok := decodeMsgCursor(f.Cursor); ok {
			legacy := c.ID == ""
			switch {
			case legacy && f.Sort == "oldest":
				q += ` AND created_at>?`
				args = append(args, c.C)
			case legacy:
				q += ` AND created_at<?`
				args = append(args, c.C)
			case f.Sort == "oldest":
				q += ` AND (created_at>? OR (created_at=? AND id>?))`
				args = append(args, c.C, c.C, c.ID)
			case f.Sort == "favorite":
				q += ` AND (favorited<? OR (favorited=? AND (created_at<? OR (created_at=? AND id<?))))`
				args = append(args, c.F, c.F, c.C, c.C, c.ID)
			case f.Sort == "pinned":
				q += ` AND (pinned<? OR (pinned=? AND (created_at<? OR (created_at=? AND id<?))))`
				args = append(args, c.P, c.P, c.C, c.C, c.ID)
			default:
				q += ` AND (created_at<? OR (created_at=? AND id<?))`
				args = append(args, c.C, c.C, c.ID)
			}
		}
	}
	switch f.Sort {
	case "oldest":
		q += ` ORDER BY created_at ASC, id ASC`
	case "favorite":
		q += ` ORDER BY favorited DESC, created_at DESC, id DESC`
	case "pinned":
		q += ` ORDER BY pinned DESC, created_at DESC, id DESC`
	default:
		q += ` ORDER BY created_at DESC, id DESC`
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q += ` LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	out := []Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var next *string
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		c := encodeMsgCursor(msgCursor{C: last.CreatedAt, ID: last.ID, F: b2i(last.Favorited), P: b2i(last.Pinned)})
		next = &c
	}
	if err := s.hydrate(ctx, out); err != nil {
		return nil, nil, err
	}
	return out, next, nil
}

// Update edits text, pin/favorite state and tags. created_at/expires_at are
// preserved on a plain text edit; toggling favorite adjusts expiry.
func (s *Service) Update(ctx context.Context, id string, text *string, pinned, favorited *bool, tagIDs *[]string) (Message, error) {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return Message{}, err
	}
	oldText, oldFav := cur.Text, cur.Favorited
	// Editing text/pin/tags MUST preserve expires_at. Only a favorite toggle
	// recalculates it: favoriting clears expiry, unfavoriting restores the
	// creation-time retention window.
	textChanged := text != nil && *text != oldText
	favoriteToggled := favorited != nil && *favorited != oldFav
	var exp any = cur.ExpiresAt
	if favoriteToggled {
		if *favorited {
			exp = nil
		} else {
			created, _ := util.ParseTime(cur.CreatedAt)
			e := ExpiryFrom(created, s.retentionDays(ctx))
			if !e.IsZero() {
				exp = util.Format(e)
			}
		}
	}
	if text != nil {
		cur.Text = *text
	}
	if pinned != nil {
		cur.Pinned = *pinned
	}
	if favorited != nil {
		cur.Favorited = *favorited
	}
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE messages SET text=?,pinned=?,favorited=?,updated_at=?,expires_at=? WHERE id=?`,
			cur.Text, db.IntBool(cur.Pinned), db.IntBool(cur.Favorited), util.Now(), exp, id); err != nil {
			return err
		}
		if tagIDs != nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM message_tags WHERE message_id=?`, id); err != nil {
				return err
			}
			for _, tid := range *tagIDs {
				if _, err := tx.ExecContext(ctx,
					`INSERT OR IGNORE INTO message_tags(message_id,tag_id) VALUES(?,?)`, id, tid); err != nil {
					return err
				}
			}
		}
		if textChanged {
			if _, err := tx.ExecContext(ctx, `DELETE FROM message_links WHERE message_id=?`, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	if textChanged {
		s.persistLinks(ctx, id, cur.Text)
	}
	full, err := s.Get(ctx, id)
	if err != nil {
		return Message{}, err
	}
	s.events.Publish("message.updated", full)
	return full, nil
}

// SoftDelete moves a message to the recycle bin.
func (s *Service) SoftDelete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE messages SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, util.Now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMessageNotFound
	}
	s.cancelBoundUploads(ctx, id)
	s.events.Publish("message.deleted", map[string]any{"id": id, "deviceId": ""})
	return nil
}

// Restore brings a recycled message back. mode "reset" restarts the retention
// window; "original" keeps the expiry computed from the original created_at.
func (s *Service) Restore(ctx context.Context, id, mode string) (Message, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return Message{}, err
	}
	created, _ := util.ParseTime(m.CreatedAt)
	var exp any
	if m.Favorited {
		exp = nil
	} else if mode == "reset" {
		e := ExpiryFrom(time.Now().UTC(), s.retentionDays(ctx))
		if !e.IsZero() {
			exp = util.Format(e)
		}
	} else {
		e := ExpiryFrom(created, s.retentionDays(ctx))
		if !e.IsZero() {
			exp = util.Format(e)
		}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE messages SET deleted_at=NULL, expires_at=?, restore_mode=? WHERE id=?`, exp, mode, id)
	if err != nil {
		return Message{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Message{}, ErrMessageNotFound
	}
	full, err := s.Get(ctx, id)
	if err != nil {
		return Message{}, err
	}
	s.events.Publish("message.updated", full)
	return full, nil
}

// Purge permanently deletes a message and any now-unreferenced, unfavorited
// attachments it was the sole owner of.
func (s *Service) Purge(ctx context.Context, id string) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	aidSet := map[string]bool{}
	for _, a := range m.Attachments {
		aidSet[a.ID] = true
	}
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM message_tags WHERE message_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM message_attachments WHERE message_id=?`, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id=?`, id)
		return err
	})
	if err != nil {
		return err
	}
	for aid := range aidSet {
		var refs, fav int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_attachments WHERE attachment_id=?`, aid).Scan(&refs)
		_ = s.db.QueryRowContext(ctx, `SELECT favorited FROM attachments WHERE id=?`, aid).Scan(&fav)
		if refs == 0 && fav == 0 {
			_ = s.files.Purge(ctx, aid)
		}
	}
	// Also drop link rows for a purged message.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM message_links WHERE message_id=?`, id)
	s.cancelBoundUploads(ctx, id)
	s.events.Publish("message.deleted", map[string]any{"id": id, "purged": true})
	return nil
}

type BulkResult struct {
	Affected int `json:"affected"`
}

// Bulk applies an action to a set of messages. For "restore", mode selects
// "reset" (restart retention) or "original" (keep the creation-time expiry).
func (s *Service) Bulk(ctx context.Context, ids []string, action, tagID, mode, ip string) (BulkResult, error) {
	var res BulkResult
	if len(ids) == 0 {
		return res, nil
	}
	switch action {
	case "delete":
		for _, id := range ids {
			if err := s.SoftDelete(ctx, id); err == nil {
				res.Affected++
			}
		}
	case "restore":
		for _, id := range ids {
			if _, err := s.Restore(ctx, id, mode); err == nil {
				res.Affected++
			}
		}
	case "favorite", "unfavorite", "pin", "unpin":
		v := action == "favorite" || action == "pin"
		for _, id := range ids {
			if action == "pin" || action == "unpin" {
				_, err := s.Update(ctx, id, nil, &v, nil, nil)
				if err == nil {
					res.Affected++
				}
			} else {
				_, err := s.Update(ctx, id, nil, nil, &v, nil)
				if err == nil {
					res.Affected++
				}
			}
		}
	case "tag", "untag":
		for _, id := range ids {
			if tagID == "" {
				continue
			}
			var err error
			if action == "tag" {
				_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO message_tags(message_id,tag_id) VALUES(?,?)`, id, tagID)
			} else {
				_, err = s.db.ExecContext(ctx, `DELETE FROM message_tags WHERE message_id=? AND tag_id=?`, id, tagID)
			}
			if err == nil {
				res.Affected++
			}
		}
	default:
		return res, errors.New("unknown bulk action")
	}
	return res, nil
}

// Expiring returns messages expiring within the given number of days.
func (s *Service) Expiring(ctx context.Context, days int) ([]Message, error) {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `
        SELECT `+msgCols+` FROM messages
        WHERE deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at>? AND expires_at<=?
        ORDER BY expires_at ASC LIMIT 500`, now, cutoff)
	if err != nil {
		return nil, err
	}
	out := []Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.hydrate(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// --- tags ------------------------------------------------------------------

func scanTag(row interface{ Scan(...any) error }) (Tag, error) {
	var t Tag
	var sys int
	err := row.Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &t.Sort, &sys)
	t.IsSystem = db.Bool(int64(sys))
	return t, err
}

func (s *Service) Tags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,color,icon,sort,is_system FROM tags ORDER BY sort,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) CreateTag(ctx context.Context, name, color, icon string, sort int) (Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tag{}, errors.New("tag name required")
	}
	t := Tag{ID: util.NewID(), Name: name, Color: color, Icon: icon, Sort: sort}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tags(id,name,color,icon,sort,is_system) VALUES(?,?,?,?,?,0)`,
		t.ID, t.Name, t.Color, t.Icon, t.Sort)
	return t, err
}

func (s *Service) UpdateTag(ctx context.Context, id, name, color, icon string, sort *int) (Tag, error) {
	t, err := scanTag(s.db.QueryRowContext(ctx,
		`SELECT id,name,color,icon,sort,is_system FROM tags WHERE id=?`, id))
	if err != nil {
		return Tag{}, ErrTagNotFound
	}
	if name != "" {
		t.Name = name
	}
	if color != "" {
		t.Color = color
	}
	if icon != "" {
		t.Icon = icon
	}
	if sort != nil {
		t.Sort = *sort
	}
	_, err = s.db.ExecContext(ctx, `UPDATE tags SET name=?,color=?,icon=?,sort=? WHERE id=? AND is_system=0`,
		t.Name, t.Color, t.Icon, t.Sort, id)
	return t, err
}

func (s *Service) DeleteTag(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id=? AND is_system=0`, id)
	return err
}
