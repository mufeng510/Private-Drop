// Package file manages entity-file storage on disk, SHA-256 deduplication,
// reference counting and attachment metadata.
package file

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"private-drop/internal/config"
	"private-drop/internal/db"
	"private-drop/internal/events"
	"private-drop/internal/util"
)

var (
	ErrAttachmentNotFound = errors.New("attachment not found")
	ErrEntityNotFound     = errors.New("entity file not found")
)

// Service owns entity-file operations. dedupMu serializes the
// check-then-insert of deduplicated entities so concurrent uploads of the
// same content cannot create two physical files.
type Service struct {
	db      *db.DB
	cfg     config.Config
	events  events.Sink
	dedupMu sync.Mutex
}

// New keeps the original signature for callers that do not need live events.
func New(d *db.DB, cfg config.Config) *Service { return NewWithEvents(d, cfg, events.Noop{}) }

// NewWithEvents creates a file service that publishes attachment state
// changes over the WebSocket hub.
func NewWithEvents(d *db.DB, cfg config.Config, sink events.Sink) *Service {
	if sink == nil {
		sink = events.Noop{}
	}
	return &Service{db: d, cfg: cfg, events: sink}
}

type Attachment struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Mime         string   `json:"mime"`
	Size         int64    `json:"size"`
	SHA256       string   `json:"sha256"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	Favorited    bool     `json:"favorited"`
	UploadedAt   string   `json:"uploadedAt"`
	Type         string   `json:"type"`
	MessageIDs   []string `json:"messageIds"`
	TagIDs       []string `json:"tagIds,omitempty"`
	OriginalPath string   `json:"originalPath,omitempty"`
	DeletedAt    *string  `json:"deletedAt,omitempty"`
}

// TypeFromMime classifies an attachment into a UI category.
func TypeFromMime(name, mimeType string) string {
	m := strings.ToLower(mimeType)
	e := strings.ToLower(filepath.Ext(name))
	switch {
	case strings.HasPrefix(m, "image/"):
		return "image"
	case strings.HasPrefix(m, "video/"):
		return "video"
	case strings.HasPrefix(m, "audio/"):
		return "audio"
	case m == "application/pdf" || e == ".pdf":
		return "pdf"
	case strings.Contains(m, "officedocument") || strings.Contains(m, "msword") ||
		strings.Contains(m, "ms-excel") || strings.Contains(m, "ms-powerpoint") ||
		e == ".doc" || e == ".docx" || e == ".xls" || e == ".xlsx" || e == ".ppt" || e == ".pptx":
		return "office"
	case strings.HasPrefix(m, "text/") || e == ".md" || e == ".txt" || e == ".json" || e == ".csv" || e == ".log":
		if e == ".md" {
			return "text"
		}
		return "text"
	case e == ".go" || e == ".js" || e == ".ts" || e == ".tsx" || e == ".jsx" || e == ".py" ||
		e == ".java" || e == ".c" || e == ".cpp" || e == ".h" || e == ".html" || e == ".css" ||
		e == ".sh" || e == ".rs" || e == ".sql" || e == ".yaml" || e == ".yml" || e == ".xml":
		return "code"
	default:
		return "other"
	}
}

// StoreEntity writes data to disk if the SHA-256 is new, else reuses the
// existing entity. It does NOT change ref_count; CreateAttachment does that.
func (s *Service) StoreEntity(ctx context.Context, src io.Reader, sha string) (string, error) {
	s.dedupMu.Lock()
	defer s.dedupMu.Unlock()

	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM entity_files WHERE sha256=?`, sha).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	id = util.NewID()
	final := filepath.Join(s.cfg.FilesDir(), id)
	tmp := filepath.Join(s.cfg.TmpDir(), "entity-"+id)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(f, src)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp)
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return "", err
	}
	var size int64
	if st, err := os.Stat(final); err == nil {
		size = st.Size()
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO entity_files(id,sha256,path,size,mime,ref_count,created_at) VALUES(?,?,?,?,'',0,?)`,
		id, sha, final, size, util.Now())
	if err != nil {
		os.Remove(final)
		return "", err
	}
	return id, nil
}

// CreateAttachment inserts an attachment and increments the entity ref count.
func (s *Service) CreateAttachment(ctx context.Context, entityID, name, mimeType, sha string, size int64, width, height int) (Attachment, error) {
	a := Attachment{
		ID: util.NewID(), Name: SanitizeFileName(name), Mime: mimeType, Size: size,
		SHA256: sha, Width: width, Height: height, UploadedAt: util.Now(),
		Type: TypeFromMime(name, mimeType), MessageIDs: []string{},
	}
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
            INSERT INTO attachments(id,entity_id,name,mime,size,sha256,width,height,favorited,uploaded_at)
            VALUES(?,?,?,?,?,?,?,?,0,?)`,
			a.ID, entityID, a.Name, a.Mime, a.Size, a.SHA256, a.Width, a.Height, a.UploadedAt); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE entity_files SET ref_count=ref_count+1, mime=? WHERE id=?`, mimeType, entityID)
		return err
	})
	return a, err
}

// FindReusableAttachment returns an existing attachment with the given SHA-256
// (used by upload init dedup). Only non-deleted, non-expired attachments are reused.
func (s *Service) FindReusableAttachment(ctx context.Context, sha string) (string, bool) {
	if sha == "" {
		return "", false
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM attachments WHERE sha256=? AND deleted_at IS NULL ORDER BY rowid DESC LIMIT 1`, sha).Scan(&id)
	return id, err == nil
}

func scanAttachment(rows interface{ Scan(...any) error }) (Attachment, error) {
	var a Attachment
	var fav, w, h int
	var deleted sql.NullString
	err := rows.Scan(&a.ID, &a.Name, &a.Mime, &a.Size, &a.SHA256, &w, &h, &fav, &a.UploadedAt, &deleted, &a.OriginalPath)
	if err != nil {
		return Attachment{}, err
	}
	a.Width, a.Height, a.Favorited = w, h, db.Bool(int64(fav))
	if deleted.Valid {
		v := deleted.String
		a.DeletedAt = &v
	}
	a.Type = TypeFromMime(a.Name, a.Mime)
	a.MessageIDs = []string{}
	a.TagIDs = []string{}
	return a, nil
}

const attachmentCols = `id,name,mime,size,sha256,width,height,favorited,uploaded_at,deleted_at,original_path`

func (s *Service) fillMessageIDs(ctx context.Context, list []Attachment) error {
	for i := range list {
		rows, err := s.db.QueryContext(ctx, `SELECT message_id FROM message_attachments WHERE attachment_id=?`, list[i].ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			list[i].MessageIDs = append(list[i].MessageIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		trows, err := s.db.QueryContext(ctx, `SELECT tag_id FROM attachment_tags WHERE attachment_id=? ORDER BY tag_id`, list[i].ID)
		if err != nil {
			return err
		}
		for trows.Next() {
			var id string
			if err := trows.Scan(&id); err != nil {
				trows.Close()
				return err
			}
			list[i].TagIDs = append(list[i].TagIDs, id)
		}
		trows.Close()
		if err := trows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// GetMany returns attachments for a set of ids, preserving input order and
// skipping ids that do not exist.
func (s *Service) GetMany(ctx context.Context, ids []string) ([]Attachment, error) {
	out := make([]Attachment, 0, len(ids))
	for _, id := range ids {
		a, err := scanAttachment(s.db.QueryRowContext(ctx,
			`SELECT `+attachmentCols+` FROM attachments WHERE id=?`, id))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out = append(out, a)
	}
	if err := s.fillMessageIDs(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (Attachment, error) {
	a, err := scanAttachment(s.db.QueryRowContext(ctx,
		`SELECT `+attachmentCols+` FROM attachments WHERE id=?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attachment{}, ErrAttachmentNotFound
		}
		return Attachment{}, err
	}
	list := []Attachment{a}
	if err := s.fillMessageIDs(ctx, list); err != nil {
		return Attachment{}, err
	}
	return list[0], nil
}

type FileFilter struct {
	Type, Tag, Sort string
	Favorite        *bool
	From, To        string
	Cursor          string
	Limit           int
	IncludeDeleted  bool
}

type fileCursor struct {
	K  string `json:"k"` // primary sort value
	U  string `json:"u"` // uploaded_at
	ID string `json:"i"`
}

func encodeFileCursor(c fileCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeFileCursor(s string) (fileCursor, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return fileCursor{}, false
	}
	var c fileCursor
	if json.Unmarshal(b, &c) == nil && c.ID != "" {
		return c, true
	}
	return fileCursor{}, false
}

func fileSortValue(a Attachment, sort string) string {
	switch sort {
	case "name":
		return a.Name
	case "size":
		return strconv.FormatInt(a.Size, 10)
	default:
		return a.UploadedAt
	}
}

// List returns attachments matching the filter. Cursors are stable keysets
// over the full sort tuple with an id tiebreaker; legacy offset cursors are
// still accepted.
func (s *Service) List(ctx context.Context, f FileFilter) ([]Attachment, *string, error) {
	q := `SELECT ` + attachmentCols + ` FROM attachments WHERE 1=1`
	var args []any
	if !f.IncludeDeleted {
		q += ` AND deleted_at IS NULL`
	}
	if f.Type != "" {
		q += ` AND (LOWER(mime) LIKE ? OR LOWER(name) LIKE ?)`
		t := strings.ToLower(f.Type)
		args = append(args, "%"+t+"%", "%"+t+"%")
	}
	if f.Favorite != nil {
		q += ` AND favorited=?`
		args = append(args, db.IntBool(*f.Favorite))
	}
	if f.From != "" {
		q += ` AND uploaded_at>=?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND uploaded_at<=?`
		args = append(args, f.To)
	}
	if f.Tag != "" {
		q += ` AND id IN (SELECT attachment_id FROM attachment_tags t JOIN tags x ON x.id=t.tag_id WHERE x.name=? OR x.id=?)`
		args = append(args, f.Tag, f.Tag)
	}
	legacyOffset := 0
	hasCursor := false
	var cur fileCursor
	if f.Cursor != "" {
		if c, ok := decodeFileCursor(f.Cursor); ok {
			cur, hasCursor = c, true
		} else if b, err := base64.RawURLEncoding.DecodeString(f.Cursor); err == nil {
			if n, err := strconv.Atoi(string(b)); err == nil && n > 0 {
				legacyOffset = n
			}
		}
	}
	if hasCursor {
		switch f.Sort {
		case "oldest":
			q += ` AND (uploaded_at>? OR (uploaded_at=? AND id>?))`
			args = append(args, cur.U, cur.U, cur.ID)
		case "name":
			q += ` AND (name>? OR (name=? AND id>?))`
			args = append(args, cur.K, cur.K, cur.ID)
		case "size":
			q += ` AND (size<CAST(? AS INTEGER) OR (size=CAST(? AS INTEGER) AND id<?))`
			args = append(args, cur.K, cur.K, cur.ID)
		default:
			q += ` AND (uploaded_at<? OR (uploaded_at=? AND id<?))`
			args = append(args, cur.U, cur.U, cur.ID)
		}
	}
	switch f.Sort {
	case "oldest":
		q += ` ORDER BY uploaded_at ASC, id ASC`
	case "name":
		q += ` ORDER BY name ASC, id ASC`
	case "size":
		q += ` ORDER BY size DESC, id DESC`
	default:
		q += ` ORDER BY uploaded_at DESC, id DESC`
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q += ` LIMIT ?`
	args = append(args, limit+1)
	if !hasCursor && legacyOffset > 0 {
		q += ` OFFSET ?`
		args = append(args, legacyOffset)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	out := []Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		out = append(out, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var next *string
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		c := encodeFileCursor(fileCursor{K: fileSortValue(last, f.Sort), U: last.UploadedAt, ID: last.ID})
		next = &c
	}
	if err := s.fillMessageIDs(ctx, out); err != nil {
		return nil, nil, err
	}
	return out, next, nil
}

// Update edits favorite state, display name and independent attachment tags.
func (s *Service) Update(ctx context.Context, id string, favorited *bool, name *string, tagIDs *[]string) (Attachment, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return Attachment{}, err
	}
	if favorited != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE attachments SET favorited=? WHERE id=?`, db.IntBool(*favorited), id); err != nil {
			return Attachment{}, err
		}
	}
	if name != nil {
		n := SanitizeFileName(*name)
		if n != "" {
			if _, err := s.db.ExecContext(ctx, `UPDATE attachments SET name=? WHERE id=?`, n, id); err != nil {
				return Attachment{}, err
			}
		}
	}
	if tagIDs != nil {
		if err := s.SetTags(ctx, id, *tagIDs); err != nil {
			return Attachment{}, err
		}
	}
	a, err := s.Get(ctx, id)
	if err != nil {
		return Attachment{}, err
	}
	s.events.Publish("file.updated", map[string]any{"id": id, "attachment": a})
	return a, nil
}

// SetTags replaces all tags on an attachment.
func (s *Service) SetTags(ctx context.Context, id string, tagIDs []string) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM attachment_tags WHERE attachment_id=?`, id); err != nil {
			return err
		}
		for _, tid := range tagIDs {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO attachment_tags(attachment_id,tag_id) VALUES(?,?)`, id, tid); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddTags adds tags without removing existing ones.
func (s *Service) AddTags(ctx context.Context, id string, tagIDs []string) error {
	for _, tid := range tagIDs {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO attachment_tags(attachment_id,tag_id) VALUES(?,?)`, id, tid); err != nil {
			return err
		}
	}
	return nil
}

// RemoveTags removes the given tags from an attachment.
func (s *Service) RemoveTags(ctx context.Context, id string, tagIDs []string) error {
	for _, tid := range tagIDs {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM attachment_tags WHERE attachment_id=? AND tag_id=?`, id, tid); err != nil {
			return err
		}
	}
	return nil
}

// SetOriginalPath records the client's original relative path, used as the ZIP
// entry name when "keep structure" is requested.
func (s *Service) SetOriginalPath(ctx context.Context, id, path string) error {
	if path == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE attachments SET original_path=? WHERE id=?`, path, id)
	return err
}

// SoftDelete moves an attachment to the recycle bin.
func (s *Service) SoftDelete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE attachments SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, util.Now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAttachmentNotFound
	}
	s.events.Publish("file.deleted", map[string]any{"id": id})
	return nil
}

// Restore brings a recycled attachment back. Attachments have no independent
// retention window, so restore mode is not meaningful for them.
func (s *Service) Restore(ctx context.Context, id string, mode ...string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE attachments SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAttachmentNotFound
	}
	a, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	s.events.Publish("file.restored", map[string]any{"id": id, "attachment": a, "mode": firstMode(mode)})
	return nil
}

func firstMode(mode []string) string {
	if len(mode) > 0 {
		return mode[0]
	}
	return "original"
}

// Purge permanently removes an attachment row, drops its references and
// physically deletes the entity file if nothing else references it.
func (s *Service) Purge(ctx context.Context, id string) error {
	var entityID string
	if err := s.db.QueryRowContext(ctx, `SELECT entity_id FROM attachments WHERE id=?`, id).Scan(&entityID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAttachmentNotFound
		}
		return err
	}
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM message_attachments WHERE attachment_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM attachment_tags WHERE attachment_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM attachments WHERE id=?`, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE entity_files SET ref_count=MAX(ref_count-1,0) WHERE id=?`, entityID)
		return err
	})
	if err != nil {
		return err
	}
	s.events.Publish("file.deleted", map[string]any{"id": id, "purged": true})
	_, err = s.DeleteEntityIfUnreferenced(ctx, entityID)
	return err
}

// Bulk applies an action to a set of attachments and publishes an event per
// changed attachment so other devices stay in sync.
func (s *Service) Bulk(ctx context.Context, ids []string, action, tagID string) (int, error) {
	affected := 0
	for _, id := range ids {
		switch action {
		case "favorite", "unfavorite":
			v := action == "favorite"
			if _, err := s.Update(ctx, id, &v, nil, nil); err == nil {
				affected++
			}
		case "delete":
			if err := s.SoftDelete(ctx, id); err == nil {
				affected++
			}
		case "restore":
			if err := s.Restore(ctx, id); err == nil {
				affected++
			}
		case "tag":
			if tagID == "" {
				continue
			}
			if err := s.AddTags(ctx, id, []string{tagID}); err == nil {
				affected++
				s.events.Publish("file.updated", map[string]any{"id": id, "action": "tag", "tagId": tagID})
			}
		case "untag":
			if tagID == "" {
				continue
			}
			if err := s.RemoveTags(ctx, id, []string{tagID}); err == nil {
				affected++
				s.events.Publish("file.updated", map[string]any{"id": id, "action": "untag", "tagId": tagID})
			}
		default:
			return affected, errors.New("unknown bulk action")
		}
	}
	return affected, nil
}

// DeleteEntityIfUnreferenced physically removes an entity when no attachment
// and no active upload still references it. Returns true when removed.
func (s *Service) DeleteEntityIfUnreferenced(ctx context.Context, entityID string) (bool, error) {
	s.dedupMu.Lock()
	defer s.dedupMu.Unlock()
	var refCount int
	var sha, path string
	err := s.db.QueryRowContext(ctx, `SELECT ref_count,sha256,path FROM entity_files WHERE id=?`, entityID).
		Scan(&refCount, &sha, &path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if refCount > 0 {
		return false, nil
	}
	// Active uploads with this SHA still need the temp chunks, not the entity,
	// but keep the entity as long as an upload is in flight to be safe.
	var active int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM uploads WHERE sha256=? AND status IN ('uploading','paused')`, sha).Scan(&active)
	if active > 0 {
		return false, nil
	}
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM entity_files WHERE id=? AND ref_count=0`, entityID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

// EntityPath returns the on-disk path of an attachment's entity file.
func (s *Service) EntityPath(ctx context.Context, id string) (string, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `
        SELECT e.path FROM attachments a JOIN entity_files e ON e.id=a.entity_id WHERE a.id=?`, id).Scan(&path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrAttachmentNotFound
		}
		return "", err
	}
	return path, nil
}

// Open opens an attachment's entity file for reading.
func (s *Service) Open(ctx context.Context, id string) (*os.File, Attachment, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return nil, Attachment{}, err
	}
	path, err := s.EntityPath(ctx, id)
	if err != nil {
		return nil, Attachment{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, Attachment{}, err
	}
	return f, a, nil
}

// SanitizeFileName strips path components and dangerous characters from a
// user-supplied display name (the real storage name is always a UUID).
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '/' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "unnamed"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

// MimeForName guesses a MIME type from a filename when the browser did not
// send one.
func MimeForName(name string) string {
	if m := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); m != "" {
		return m
	}
	return "application/octet-stream"
}
