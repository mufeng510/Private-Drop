// Package upload implements resumable, chunked uploads with server-persisted
// task state and SHA-256 deduplication.
package upload

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/db"
	"private-drop/internal/events"
	"private-drop/internal/file"
	"private-drop/internal/notify"
	"private-drop/internal/util"
)

var (
	ErrUploadNotFound     = errors.New("upload not found")
	ErrChunkOutOfRange    = errors.New("chunk index out of range")
	ErrUploadSize         = errors.New("file too large")
	ErrUploadState        = errors.New("upload in invalid state")
	ErrUploadConcurrency  = errors.New("upload concurrency limit reached")
	ErrMessageUnavailable = errors.New("message is no longer available; upload cancelled")
)

type Service struct {
	db      *db.DB
	cfg     config.Config
	files   *file.Service
	events  events.Sink
	audit   *audit.Service
	notify  *notify.Service
	mergeMu sync.Mutex
}

func New(d *db.DB, cfg config.Config, files *file.Service, sink events.Sink, a *audit.Service, n *notify.Service) *Service {
	return &Service{db: d, cfg: cfg, files: files, events: sink, audit: a, notify: n}
}

type Upload struct {
	ID             string  `json:"uploadId"`
	DeviceID       string  `json:"deviceId,omitempty"`
	MessageID      string  `json:"messageId,omitempty"`
	Filename       string  `json:"name"`
	Size           int64   `json:"size"`
	Mime           string  `json:"mime"`
	SHA256         string  `json:"sha256,omitempty"`
	TotalChunks    int     `json:"totalChunks"`
	ChunkSize      int64   `json:"chunkSize"`
	UploadedChunks []int   `json:"uploadedChunks"`
	Status         string  `json:"status"`
	AttachmentID   string  `json:"attachmentId,omitempty"`
	Error          string  `json:"error,omitempty"`
	OriginalPath   string  `json:"originalPath,omitempty"`
	BytesUploaded  int64   `json:"bytesUploaded"`
	BytesRemaining int64   `json:"bytesRemaining"`
	Speed          float64 `json:"speed"` // bytes/second (average since creation)
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

type InitResult struct {
	UploadID           string `json:"uploadId"`
	UploadedChunks     []int  `json:"uploadedChunks"`
	ReusedAttachmentID string `json:"reusedAttachmentId,omitempty"`
}

const uploadCols = `id,device_id,message_id,filename,size,mime,sha256,total_chunks,chunk_size,
       uploaded_chunks,status,attachment_id,error,original_path,created_at,updated_at`

func scanUpload(row interface{ Scan(...any) error }) (Upload, error) {
	var u Upload
	var chunks string
	var msgID, attID sql.NullString
	err := row.Scan(&u.ID, &u.DeviceID, &msgID, &u.Filename, &u.Size, &u.Mime, &u.SHA256,
		&u.TotalChunks, &u.ChunkSize, &chunks, &u.Status, &attID, &u.Error, &u.OriginalPath,
		&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return Upload{}, err
	}
	if msgID.Valid {
		u.MessageID = msgID.String
	}
	if attID.Valid {
		u.AttachmentID = attID.String
	}
	_ = json.Unmarshal([]byte(chunks), &u.UploadedChunks)
	if u.UploadedChunks == nil {
		u.UploadedChunks = []int{}
	}
	computeTransfer(&u)
	return u, nil
}

// computeTransfer derives the byte-level progress fields exposed by the API.
func computeTransfer(u *Upload) {
	if u.Status == "completed" && u.AttachmentID != "" {
		u.BytesUploaded = u.Size
	} else {
		last := int64(u.TotalChunks - 1)
		for _, idx := range u.UploadedChunks {
			if int64(idx) == last && last >= 0 {
				u.BytesUploaded += u.Size - u.ChunkSize*last
			} else {
				u.BytesUploaded += u.ChunkSize
			}
		}
	}
	if u.BytesUploaded < 0 {
		u.BytesUploaded = 0
	}
	if u.BytesUploaded > u.Size {
		u.BytesUploaded = u.Size
	}
	u.BytesRemaining = u.Size - u.BytesUploaded
	created, err := util.ParseTime(u.CreatedAt)
	if err != nil || u.BytesUploaded == 0 {
		u.Speed = 0
		return
	}
	elapsed := time.Since(created).Seconds()
	if elapsed < 1 {
		elapsed = 1
	}
	u.Speed = float64(u.BytesUploaded) / elapsed
}

func (s *Service) maxUploadSize(ctx context.Context) int64 {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='max_upload_size'`).Scan(&v); err != nil {
		return 100 * 1024 * 1024
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 100 * 1024 * 1024
	}
	return n
}

func (s *Service) chunksDir(id string) string { return filepath.Join(s.cfg.TmpDir(), id) }

// InitParams describes a new upload task. MessageID binds the task to a
// message (deleted/purged/expired messages cancel bound uploads); RelativePath
// preserves the client's original folder layout for ZIP downloads.
type InitParams struct {
	Name, Mime, SHA256, MessageID, RelativePath string
	Size                                        int64
	TotalChunks                                 int
	ChunkSize                                   int64
}

// Init creates an upload task. When sha256 matches an existing entity, a
// reusable attachment is returned and no data needs to be sent.
func (s *Service) Init(ctx context.Context, deviceID string, p InitParams) (InitResult, error) {
	var res InitResult
	name := file.SanitizeFileName(p.Name)
	if p.Size < 0 || p.Size > s.maxUploadSize(ctx) {
		return res, ErrUploadSize
	}
	if p.TotalChunks <= 0 || p.ChunkSize <= 0 {
		return res, errors.New("invalid chunk parameters")
	}
	mime := p.Mime
	if mime == "" {
		mime = file.MimeForName(name)
	}

	if reused, ok := s.files.FindReusableAttachment(ctx, p.SHA256); ok {
		u := Upload{
			ID: util.NewID(), DeviceID: deviceID, MessageID: p.MessageID, Filename: name,
			Mime: mime, SHA256: p.SHA256, Size: p.Size, TotalChunks: p.TotalChunks,
			ChunkSize: p.ChunkSize, UploadedChunks: []int{}, Status: "completed",
			AttachmentID: reused, OriginalPath: p.RelativePath,
			CreatedAt: util.Now(), UpdatedAt: util.Now(),
		}
		if err := s.insert(ctx, u); err != nil {
			return res, err
		}
		if u.MessageID != "" {
			_ = s.files.SetOriginalPath(ctx, reused, p.RelativePath)
		}
		s.events.Publish("upload.completed", map[string]any{
			"uploadId": u.ID, "deviceId": deviceID, "attachmentId": reused, "deduplicated": true,
		})
		return InitResult{UploadID: u.ID, UploadedChunks: []int{}, ReusedAttachmentID: reused}, nil
	}

	// Enforce the configured server-side active-upload limit.
	if limit := s.uploadConcurrency(ctx); limit > 0 {
		var active int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM uploads WHERE status IN ('uploading','paused')`).Scan(&active); err != nil {
			return res, err
		}
		if active >= limit {
			return res, ErrUploadConcurrency
		}
	}

	u := Upload{
		ID: util.NewID(), DeviceID: deviceID, MessageID: p.MessageID, Filename: name,
		Mime: mime, SHA256: p.SHA256, Size: p.Size, TotalChunks: p.TotalChunks,
		ChunkSize: p.ChunkSize, UploadedChunks: []int{}, Status: "uploading",
		OriginalPath: p.RelativePath,
		CreatedAt:    util.Now(), UpdatedAt: util.Now(),
	}
	if err := s.insert(ctx, u); err != nil {
		return res, err
	}
	if err := os.MkdirAll(s.chunksDir(u.ID), 0o750); err != nil {
		return res, err
	}
	return InitResult{UploadID: u.ID, UploadedChunks: []int{}}, nil
}

func (s *Service) uploadConcurrency(ctx context.Context) int {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='upload_concurrency'`).Scan(&v); err != nil {
		return 3
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 3
	}
	return n
}

func (s *Service) insert(ctx context.Context, u Upload) error {
	chunks, _ := json.Marshal(u.UploadedChunks)
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO uploads(id,device_id,message_id,filename,size,mime,sha256,total_chunks,chunk_size,
            uploaded_chunks,status,attachment_id,error,original_path,created_at,updated_at)
        VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID, u.DeviceID, nullIfEmpty(u.MessageID), u.Filename, u.Size, u.Mime, u.SHA256,
		u.TotalChunks, u.ChunkSize, string(chunks), u.Status, nullIfEmpty(u.AttachmentID),
		u.Error, u.OriginalPath, u.CreatedAt, u.UpdatedAt)
	return err
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *Service) Get(ctx context.Context, id string) (Upload, error) {
	u, err := scanUpload(s.db.QueryRowContext(ctx,
		`SELECT `+uploadCols+` FROM uploads WHERE id=?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Upload{}, ErrUploadNotFound
		}
		return Upload{}, err
	}
	return u, nil
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

// Chunk stores one binary chunk. The operation is idempotent.
func (s *Service) Chunk(ctx context.Context, id string, index int, data io.Reader) (Upload, error) {
	u, err := s.Get(ctx, id)
	if err != nil {
		return Upload{}, err
	}
	if u.Status != "uploading" && u.Status != "paused" {
		return Upload{}, ErrUploadState
	}
	if index < 0 || index >= u.TotalChunks {
		return Upload{}, ErrChunkOutOfRange
	}
	dir := s.chunksDir(id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Upload{}, err
	}
	final := filepath.Join(dir, strconv.Itoa(index)+".part")
	tmp := final + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Upload{}, err
	}
	_, copyErr := io.Copy(f, io.LimitReader(data, u.ChunkSize+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp)
		if copyErr != nil {
			return Upload{}, copyErr
		}
		return Upload{}, closeErr
	}
	if st, _ := os.Stat(tmp); st != nil && st.Size() > u.ChunkSize {
		os.Remove(tmp)
		return Upload{}, errors.New("chunk exceeds configured chunk size")
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return Upload{}, err
	}
	if !containsInt(u.UploadedChunks, index) {
		u.UploadedChunks = append(u.UploadedChunks, index)
		sort.Ints(u.UploadedChunks)
		b, _ := json.Marshal(u.UploadedChunks)
		if _, err := s.db.ExecContext(ctx,
			`UPDATE uploads SET uploaded_chunks=?,status='uploading',updated_at=? WHERE id=?`,
			string(b), util.Now(), id); err != nil {
			return Upload{}, err
		}
	}
	computeTransfer(&u)
	s.events.Publish("upload.progress", map[string]any{
		"uploadId": id, "deviceId": u.DeviceID, "messageId": u.MessageID,
		"uploaded": len(u.UploadedChunks), "total": u.TotalChunks,
		"percent":       percent(len(u.UploadedChunks), u.TotalChunks),
		"bytesUploaded": u.BytesUploaded, "bytesRemaining": u.BytesRemaining, "speed": u.Speed,
	})
	return u, nil
}

func percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return (done * 100) / total
}

// Complete merges chunks into an entity file, dedups by SHA-256 and creates an
// attachment. Concurrent completions of the same content are serialized.
func (s *Service) Complete(ctx context.Context, id string) (string, error) {
	s.mergeMu.Lock()
	defer s.mergeMu.Unlock()

	u, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if u.Status == "completed" && u.AttachmentID != "" {
		return u.AttachmentID, nil
	}
	if u.Status != "uploading" && u.Status != "paused" {
		return "", ErrUploadState
	}
	// If the bound message was deleted/purged while uploading, stop before
	// creating an orphaned entity file.
	if u.MessageID != "" {
		var deleted sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT deleted_at FROM messages WHERE id=?`, u.MessageID).Scan(&deleted)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && deleted.Valid) {
			_ = s.cancelTask(ctx, u.ID, "failed", "message unavailable", false)
			return "", ErrMessageUnavailable
		}
		if err != nil {
			return "", err
		}
	}
	if len(u.UploadedChunks) != u.TotalChunks {
		return "", errors.New("not all chunks received")
	}

	dir := s.chunksDir(id)
	merged := filepath.Join(s.cfg.TmpDir(), "merge-"+id)
	out, err := os.OpenFile(merged, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	var written int64
	for i := 0; i < u.TotalChunks; i++ {
		p := filepath.Join(dir, strconv.Itoa(i)+".part")
		f, err := os.Open(p)
		if err != nil {
			out.Close()
			os.Remove(merged)
			return "", err
		}
		n, err := io.Copy(io.MultiWriter(out, hasher), f)
		f.Close()
		if err != nil {
			out.Close()
			os.Remove(merged)
			return "", err
		}
		written += n
	}
	if err := out.Close(); err != nil {
		os.Remove(merged)
		return "", err
	}
	if written != u.Size {
		os.Remove(merged)
		return "", errors.New("uploaded size mismatch")
	}
	sha := u.SHA256
	if sha == "" {
		sha = hex.EncodeToString(hasher.Sum(nil))
	} else if hex.EncodeToString(hasher.Sum(nil)) != sha {
		os.Remove(merged)
		return "", errors.New("sha256 mismatch")
	}

	mergedFile, err := os.Open(merged)
	if err != nil {
		os.Remove(merged)
		return "", err
	}
	// Compute image dimensions before the merged temp file is removed.
	w, h := imageDimensions(merged, u.Mime)
	entityID, err := s.files.StoreEntity(ctx, mergedFile, sha)
	mergedFile.Close()
	os.Remove(merged)
	if err != nil {
		return "", err
	}

	att, err := s.files.CreateAttachment(ctx, entityID, u.Filename, u.Mime, sha, u.Size, w, h)
	if err != nil {
		return "", err
	}
	if u.OriginalPath != "" {
		_ = s.files.SetOriginalPath(ctx, att.ID, u.OriginalPath)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE uploads SET status='completed',attachment_id=?,updated_at=? WHERE id=?`,
		att.ID, util.Now(), id)
	if err != nil {
		return "", err
	}
	os.RemoveAll(dir)
	s.events.Publish("upload.completed", map[string]any{
		"uploadId": id, "deviceId": u.DeviceID, "messageId": u.MessageID,
		"attachmentId": att.ID, "name": u.Filename,
		"bytesUploaded": u.Size, "bytesRemaining": int64(0),
	})
	if s.notify.Enabled(ctx, "notify_upload_done") {
		_, _ = s.notify.Create(ctx, "upload_complete", map[string]any{"attachmentId": att.ID, "name": u.Filename})
	}
	return att.ID, nil
}

func (s *Service) Status(ctx context.Context, id string) (Upload, error) { return s.Get(ctx, id) }

func (s *Service) List(ctx context.Context) ([]Upload, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+uploadCols+` FROM uploads ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Upload{}
	for rows.Next() {
		u, err := scanUpload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	u, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if u.Status == "completed" {
		return ErrUploadState
	}
	return s.cancelTask(ctx, u.ID, "cancelled", "", true)
}

// cancelTask marks a task with a terminal status and removes its chunk dir.
func (s *Service) cancelTask(ctx context.Context, id, status, reason string, notifyFailure bool) error {
	u, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if reason != "" {
		if _, err := s.db.ExecContext(ctx, `UPDATE uploads SET error=? WHERE id=?`, reason, id); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE uploads SET status=?,updated_at=? WHERE id=?`, status, util.Now(), id); err != nil {
		return err
	}
	s.events.Publish("upload.cancelled", map[string]any{
		"uploadId": id, "deviceId": u.DeviceID, "messageId": u.MessageID, "reason": reason,
	})
	if notifyFailure && s.notify.Enabled(ctx, "notify_upload_failed") {
		_, _ = s.notify.Create(ctx, "upload_failed", map[string]any{
			"uploadId": id, "name": u.Filename, "reason": reason,
		})
	}
	return os.RemoveAll(s.chunksDir(id))
}

// CancelForMessage cancels every active upload bound to a message. Called when
// the message is deleted, purged or expires so no orphaned entity is created
// and temporary chunks are reclaimed immediately.
func (s *Service) CancelForMessage(ctx context.Context, messageID string) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM uploads WHERE message_id=? AND status IN ('uploading','paused','needs_source')`, messageID)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := s.cancelTask(ctx, id, "cancelled", "message unavailable", true); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *Service) Retry(ctx context.Context, id string) (Upload, error) {
	u, err := s.Get(ctx, id)
	if err != nil {
		return Upload{}, err
	}
	if u.Status != "failed" && u.Status != "cancelled" && u.Status != "needs_source" {
		return Upload{}, ErrUploadState
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE uploads SET status='uploading',error='',updated_at=? WHERE id=?`, util.Now(), id); err != nil {
		return Upload{}, err
	}
	return s.Get(ctx, id)
}

// MarkNeedsSource records that a resume could not re-open the original local
// file (cleared site data, revoked file handle, file moved). The user must
// re-select the source before the task can continue.
func (s *Service) MarkNeedsSource(ctx context.Context, id string) (Upload, error) {
	u, err := s.Get(ctx, id)
	if err != nil {
		return Upload{}, err
	}
	if u.Status == "completed" {
		return Upload{}, ErrUploadState
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE uploads SET status='needs_source',error=?,updated_at=? WHERE id=?`,
		"original source file unavailable", util.Now(), id); err != nil {
		return Upload{}, err
	}
	s.events.Publish("upload.needs_source", map[string]any{
		"uploadId": id, "deviceId": u.DeviceID, "messageId": u.MessageID, "name": u.Filename,
	})
	if s.notify.Enabled(ctx, "notify_upload_failed") {
		_, _ = s.notify.Create(ctx, "upload_needs_source", map[string]any{
			"uploadId": id, "name": u.Filename,
		})
	}
	return s.Get(ctx, id)
}

func (s *Service) MarkFailed(ctx context.Context, id, reason string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE uploads SET status='failed',error=?,updated_at=? WHERE id=?`, reason, util.Now(), id); err != nil {
		return err
	}
	if s.notify.Enabled(ctx, "notify_upload_failed") {
		u, err := s.Get(ctx, id)
		if err == nil {
			_, _ = s.notify.Create(ctx, "upload_failed", map[string]any{
				"uploadId": id, "name": u.Filename, "reason": reason,
			})
		}
	}
	return nil
}

// ReapInactive fails uploads that have not made progress for `inactivity`,
// removing their chunk directories. updated_at is touched on every chunk so
// genuinely stalled tasks are targeted rather than long-running active ones.
func (s *Service) ReapInactive(ctx context.Context, inactivity time.Duration) (int, error) {
	cutoff := util.Format(time.Now().UTC().Add(-inactivity))
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM uploads WHERE status IN ('uploading','paused') AND updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := s.cancelTask(ctx, id, "failed", "upload timed out due to inactivity", true); err == nil {
			n++
		}
	}
	return n, nil
}

// CleanupStale removes temporary chunk directories for tasks no longer active,
// plus orphan directories. Inactivity is based on updated_at, not created_at.
func (s *Service) CleanupStale(ctx context.Context, maxAge time.Duration) (int, error) {
	cutoff := util.Format(time.Now().UTC().Add(-maxAge))
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM uploads WHERE status NOT IN ('uploading','paused','needs_source') AND updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	removed := 0
	for _, id := range ids {
		if err := os.RemoveAll(s.chunksDir(id)); err == nil {
			removed++
		}
	}
	// Remove directories whose upload row no longer exists.
	entries, err := os.ReadDir(s.cfg.TmpDir())
	if err != nil {
		return removed, err
	}
	for _, e := range entries {
		if !e.IsDir() || filepath.Base(e.Name()) == "thumbs" {
			continue
		}
		var n int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM uploads WHERE id=?`, e.Name()).Scan(&n)
		if n == 0 {
			if os.RemoveAll(filepath.Join(s.cfg.TmpDir(), e.Name())) == nil {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Service) AttachToMessage(ctx context.Context, id, messageID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE uploads SET message_id=?,updated_at=? WHERE id=?`, messageID, util.Now(), id)
	return err
}

func (s *Service) AuditFailure(ctx context.Context, id, reason, ip string) {
	s.audit.Log(ctx, "upload.failed", "上传失败 "+id+": "+reason, ip, "")
}
