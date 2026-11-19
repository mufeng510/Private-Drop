package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"private-drop/internal/file"
	"private-drop/internal/linkpreview"
	"private-drop/internal/message"
	"private-drop/internal/search"
)

var startedAt = time.Now()

// --- devices ---------------------------------------------------------------

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	list, err := s.auth.ListDevices(r.Context(), p.UserID)
	if err != nil {
		serverErr(w, err)
		return
	}
	for i := range list {
		list[i].Current = list[i].ID == p.DeviceID
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) renameDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	p := principal(r)
	dev, err := s.auth.RenameDevice(r.Context(), p.UserID, r.PathValue("id"), body.Name)
	if err != nil {
		if isNotFound(err) {
			notFound(w, "device not found")
		} else {
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, dev)
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	if err := s.auth.RevokeDevice(r.Context(), p.UserID, r.PathValue("id"), s.clientIP(r)); err != nil {
		if isNotFound(err) {
			notFound(w, "device not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	// Kick any live WebSocket for the revoked device immediately.
	s.hub.DisconnectDevice(r.PathValue("id"))
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) listDeviceRequests(w http.ResponseWriter, r *http.Request) {
	list, err := s.auth.ListRequests(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) approveDeviceRequest(w http.ResponseWriter, r *http.Request) {
	dev, err := s.auth.ApproveRequest(r.Context(), r.PathValue("id"), s.clientIP(r))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "request not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, dev)
}

func (s *Server) rejectDeviceRequest(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.RejectRequest(r.Context(), r.PathValue("id"), s.clientIP(r)); err != nil {
		if isNotFound(err) {
			notFound(w, "request not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) approveDeviceCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	dev, err := s.auth.ApproveByCode(r.Context(), body.Code, s.clientIP(r))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "invalid or expired code")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, dev)
}

// --- search ----------------------------------------------------------------

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minSize, _ := strconv.ParseInt(q.Get("minSize"), 10, 64)
	maxSize, _ := strconv.ParseInt(q.Get("maxSize"), 10, 64)
	items, next, err := s.searchSvc.Search(r.Context(), searchFilter(q, minSize, maxSize))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

// --- tags ------------------------------------------------------------------

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.messages.Tags(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": tags})
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Icon  string `json:"icon"`
		Sort  int    `json:"sort"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	t, err := s.messages.CreateTag(r.Context(), body.Name, body.Color, body.Icon, body.Sort)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) updateTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Icon  string `json:"icon"`
		Sort  *int   `json:"sort"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	t, err := s.messages.UpdateTag(r.Context(), r.PathValue("id"), body.Name, body.Color, body.Icon, body.Sort)
	if err != nil {
		notFound(w, "tag not found")
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	if err := s.messages.DeleteTag(r.Context(), r.PathValue("id")); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

// --- recycle ---------------------------------------------------------------

func (s *Server) listRecycle(w http.ResponseWriter, r *http.Request) {
	msgRows, err := s.db.QueryContext(r.Context(), `SELECT id FROM messages WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC`)
	if err != nil {
		serverErr(w, err)
		return
	}
	var ids []string
	for msgRows.Next() {
		var id string
		if err := msgRows.Scan(&id); err != nil {
			msgRows.Close()
			serverErr(w, err)
			return
		}
		ids = append(ids, id)
	}
	msgRows.Close()
	messages := []message.Message{}
	for _, id := range ids {
		if m, err := s.messages.Get(r.Context(), id); err == nil {
			messages = append(messages, m)
		}
	}
	fRows, err := s.db.QueryContext(r.Context(), `SELECT id FROM attachments WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC`)
	if err != nil {
		serverErr(w, err)
		return
	}
	var fids []string
	for fRows.Next() {
		var id string
		if err := fRows.Scan(&id); err != nil {
			fRows.Close()
			serverErr(w, err)
			return
		}
		fids = append(fids, id)
	}
	fRows.Close()
	files := []file.Attachment{}
	for _, id := range fids {
		if a, err := s.files.Get(r.Context(), id); err == nil {
			files = append(files, a)
		}
	}
	writeJSON(w, 200, map[string]any{"messages": messages, "files": files})
}

func (s *Server) restoreRecycle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	_ = decodeBody(r, &body)
	id := r.PathValue("id")
	m, err := s.messages.Restore(r.Context(), id, body.Mode)
	if err != nil {
		// It may be an attachment instead.
		if ferr := s.files.Restore(r.Context(), id); ferr != nil {
			notFound(w, "item not found")
			return
		}
		a, _ := s.files.Get(r.Context(), id)
		writeJSON(w, 200, a)
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) purgeRecycle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.messages.Purge(r.Context(), id); err != nil {
		if ferr := s.files.Purge(r.Context(), id); ferr != nil {
			notFound(w, "item not found")
			return
		}
	}
	s.audit.Log(r.Context(), "recycle.purge", "永久删除: "+id, s.clientIP(r), principal(r).DeviceID)
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) emptyRecycle(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id FROM messages WHERE deleted_at IS NOT NULL`)
	if err != nil {
		serverErr(w, err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		_ = s.messages.Purge(r.Context(), id)
	}
	frows, err := s.db.QueryContext(r.Context(), `SELECT id FROM attachments WHERE deleted_at IS NOT NULL`)
	if err != nil {
		serverErr(w, err)
		return
	}
	var fids []string
	for frows.Next() {
		var id string
		_ = frows.Scan(&id)
		fids = append(fids, id)
	}
	frows.Close()
	for _, id := range fids {
		_ = s.files.Purge(r.Context(), id)
	}
	s.audit.Log(r.Context(), "recycle.empty", "清空回收站", s.clientIP(r), principal(r).DeviceID)
	writeJSON(w, 200, map[string]any{})
}

// --- notifications ---------------------------------------------------------

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	items, unread, err := s.notify.List(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "unread": unread})
}

func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	if err := s.notify.ReadAll(r.Context()); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) deleteNotification(w http.ResponseWriter, r *http.Request) {
	if err := s.notify.Delete(r.Context(), r.PathValue("id")); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

// --- settings --------------------------------------------------------------

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.auth.Settings(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, settings)
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var values map[string]string
	if err := decodeBody(r, &values); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	changed, err := s.auth.UpdateSettings(r.Context(), values)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, k := range changed {
		s.audit.Log(r.Context(), "settings.changed", "修改设置: "+k, s.clientIP(r), principal(r).DeviceID)
	}
	s.hub.Publish("settings.changed", map[string]any{"keys": changed})
	writeJSON(w, 200, map[string]any{"changed": changed})
}

func (s *Server) securitySettings(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	settings, err := s.auth.Settings(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	u, _ := s.auth.CurrentUser(r.Context(), p.UserID)
	writeJSON(w, 200, map[string]any{
		"totpEnabled":    u.HasTOTP,
		"passkeyEnabled": s.auth.HasPasskey(r.Context(), p.UserID),
		"devicePolicy":   settings["device_policy"],
		"sessionTtlDays": settings["session_ttl_days"],
	})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	p := principal(r)
	if err := s.auth.ChangePassword(r.Context(), p.UserID, body.OldPassword, body.NewPassword); err != nil {
		badRequest(w, err.Error())
		return
	}
	s.audit.Log(r.Context(), "security.password_changed", "修改密码", s.clientIP(r), p.DeviceID)
	writeJSON(w, 200, map[string]any{})
}

// --- logs / audit / storage / system --------------------------------------

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	list, err := s.audit.Logs(r.Context(), r.URL.Query().Get("type"), queryInt(r, "limit", 200))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	list, err := s.audit.Audit(r.Context(), queryInt(r, "limit", 200))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) storageStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var entityCount, totalSize, messages, attachments, favorites, recycle int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(size),0) FROM entity_files`).Scan(&entityCount, &totalSize)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE deleted_at IS NULL`).Scan(&messages)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM attachments WHERE deleted_at IS NULL`).Scan(&attachments)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM attachments WHERE favorited=1 AND deleted_at IS NULL`).Scan(&favorites)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE deleted_at IS NOT NULL`).Scan(&recycle)

	var recycleBytes, favoriteBytes, duplicateBytes int64
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0) FROM attachments WHERE deleted_at IS NOT NULL`).Scan(&recycleBytes)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size),0) FROM attachments WHERE favorited=1 AND deleted_at IS NULL`).Scan(&favoriteBytes)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size*(ref_count-1)),0) FROM entity_files WHERE ref_count>1`).Scan(&duplicateBytes)

	// Aggregate by UI file category (image/video/audio/...), not raw MIME.
	type breakdown struct {
		Bytes int64
		Count int
	}
	grouped := map[string]*breakdown{}
	rows, err := s.db.QueryContext(ctx, `SELECT mime,COUNT(*),COALESCE(SUM(size),0) FROM entity_files GROUP BY mime`)
	if err == nil {
		for rows.Next() {
			var mime string
			var count int
			var size int64
			if rows.Scan(&mime, &count, &size) == nil {
				t := file.TypeFromMime(mime, "")
				b := grouped[t]
				if b == nil {
					b = &breakdown{}
					grouped[t] = b
				}
				b.Bytes += size
				b.Count += count
			}
		}
		rows.Close()
	}
	byType := []map[string]any{}
	for t, b := range grouped {
		byType = append(byType, map[string]any{"type": t, "bytes": b.Bytes, "count": b.Count})
	}

	var uploadTempBytes int64
	_ = filepath.WalkDir(s.cfg.TmpDir(), func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			uploadTempBytes += info.Size()
		}
		return nil
	})

	big, _, err := s.files.List(ctx, file.FileFilter{Limit: 10, Sort: "size"})
	if err != nil {
		big = []file.Attachment{}
	}
	expiring := []file.Attachment{}
	cutoff := time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	eids, err := s.db.QueryContext(ctx, `SELECT DISTINCT ma.attachment_id
		FROM message_attachments ma JOIN messages m ON m.id=ma.message_id
		WHERE m.deleted_at IS NULL AND m.expires_at IS NOT NULL AND m.expires_at<=? LIMIT 20`, cutoff)
	if err == nil {
		for eids.Next() {
			var id string
			if eids.Scan(&id) == nil {
				if a, e := s.files.Get(ctx, id); e == nil && !a.Favorited {
					expiring = append(expiring, a)
				}
			}
		}
		eids.Close()
	}

	warnings, suggestions := s.storageAdvice(ctx, duplicateBytes, recycleBytes, uploadTempBytes)
	uploadSummary := map[string]int{}
	for _, st := range []string{"uploading", "paused", "needs_source", "failed", "completed"} {
		var n int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM uploads WHERE status=?`, st).Scan(&n)
		uploadSummary[st] = n
	}
	writeJSON(w, 200, map[string]any{
		"totalBytes": totalSize, "fileCount": entityCount, "messageCount": messages,
		"byType": byType, "largestFiles": big, "expiringFiles": expiring,
		"recycleBytes": recycleBytes, "favoriteBytes": favoriteBytes,
		"uploadTempBytes": uploadTempBytes, "duplicateBytes": duplicateBytes,
		"warnings": warnings, "suggestions": suggestions, "uploadSummary": uploadSummary,
		// Kept for clients that consume the raw MIME breakdown.
		"attachments": attachments, "messages": messages, "favoriteFiles": favorites,
		"recycleItems": recycle,
	})
}

// storageAdvice produces real disk-space warnings and concrete cleanup
// suggestions. It only reports; it never deletes data or blocks uploads.
func (s *Server) storageAdvice(ctx context.Context, duplicate, recycle, temp int64) ([]string, []string) {
	warnings := []string{}
	suggestions := []string{}
	var st syscall.Statfs_t
	if err := syscall.Statfs(s.cfg.DataDir, &st); err == nil && st.Blocks > 0 {
		avail := int64(st.Bavail) * int64(st.Bsize)
		usedPct := float64(st.Blocks-st.Bavail) / float64(st.Blocks) * 100
		threshold := 90
		if v := s.auth.Setting(ctx, "storage_warning_percent"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				threshold = n
			}
		}
		if usedPct >= float64(threshold) {
			warnings = append(warnings, fmt.Sprintf("磁盘使用率 %.1f%% 已达到警戒值 %d%%（可用 %s）",
				usedPct, threshold, humanBytes(avail)))
		}
		if avail < 1<<30 {
			warnings = append(warnings, "磁盘可用空间不足 1 GiB，继续上传可能失败")
		}
	}
	if duplicate > 0 {
		suggestions = append(suggestions, fmt.Sprintf("重复文件占用约 %s，可复核后清理", humanBytes(duplicate)))
	}
	if recycle > 0 {
		suggestions = append(suggestions, fmt.Sprintf("回收站占用 %s，彻底删除可释放空间", humanBytes(recycle)))
	}
	if temp > 0 {
		suggestions = append(suggestions, fmt.Sprintf("临时分片/缩略图占用 %s，清理任务会定期回收", humanBytes(temp)))
	}
	var active, pending int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM uploads WHERE status IN ('uploading','paused')`).Scan(&active)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM uploads WHERE status IN ('failed','needs_source')`).Scan(&pending)
	if active > 0 {
		suggestions = append(suggestions, fmt.Sprintf("%d 个上传任务正在进行中", active))
	}
	if pending > 0 {
		warnings = append(warnings, fmt.Sprintf("%d 个上传任务失败或等待重新选择源文件", pending))
	}
	return warnings, suggestions
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, u := range []string{"KiB", "MiB", "GiB", "TiB"} {
		v /= unit
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, u)
		}
	}
	return fmt.Sprintf("%.1f PiB", v/unit)
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"version":     "1.0.0",
		"port":        s.cfg.Port,
		"dataDir":     s.cfg.DataDir,
		"initialized": s.auth.IsInitialized(r.Context()),
		"uptime":      time.Since(startedAt).String(),
		"goVersion":   runtime.Version(),
		"os":          runtime.GOOS + "/" + runtime.GOARCH,
		"websocket":   s.hub.Stats(),
	})
}

func (s *Server) diagnose(w http.ResponseWriter, r *http.Request) {
	checks := map[string]any{}
	if err := s.db.PingContext(r.Context()); err == nil {
		checks["database"] = "ok"
	} else {
		checks["database"] = "error: " + err.Error()
	}
	for _, d := range []string{s.cfg.DataDir, s.cfg.FilesDir(), s.cfg.TmpDir()} {
		if err := os.MkdirAll(d, 0o750); err == nil {
			test := d + "/.write-test"
			if err := os.WriteFile(test, []byte("ok"), 0o600); err == nil {
				os.Remove(test)
				checks["dir:"+d] = "ok"
			} else {
				checks["dir:"+d] = "not writable"
			}
		} else {
			checks["dir:"+d] = "error: " + err.Error()
		}
	}
	var uploads, pending int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM uploads WHERE status IN ('uploading','paused')`).Scan(&uploads)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM device_requests WHERE status='pending'`).Scan(&pending)
	checks["activeUploads"] = uploads
	checks["pendingDeviceRequests"] = pending
	checks["websocket"] = s.hub.Stats()
	writeJSON(w, 200, checks)
}

// --- link preview ----------------------------------------------------------

func (s *Server) linkPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	empty := map[string]any{"title": "", "description": "", "imageUrl": ""}
	if strings.EqualFold(s.auth.Setting(r.Context(), "link_preview_enabled"), "false") {
		writeJSON(w, 200, empty)
		return
	}
	res, err := s.links.Preview(r.Context(), body.URL)
	if err != nil {
		// Link preview failures must never break message sending.
		writeJSON(w, 200, empty)
		return
	}
	// The extracted og:image is attacker-controlled; apply the same SSRF
	// policy before handing it to clients.
	if res.ImageURL != "" && linkpreview.ValidateImageURL(res.ImageURL) != nil {
		res.ImageURL = ""
	}
	writeJSON(w, 200, res)
}

func searchFilter(q url.Values, minSize, maxSize int64) search.Filter {
	return search.Filter{
		Q: q.Get("q"), Type: q.Get("type"), Ext: q.Get("ext"),
		Tag: q.Get("tag"), Device: q.Get("device"), From: q.Get("from"), To: q.Get("to"),
		MinSize: minSize, MaxSize: maxSize,
		Favorite: queryBoolFrom(q, "favorite"), Pinned: queryBoolFrom(q, "pinned"),
		Cursor: q.Get("cursor"), Limit: queryIntFrom(q, "limit", 50),
	}
}

func queryBoolFrom(q url.Values, key string) *bool {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}
	return &b
}

func queryIntFrom(q url.Values, key string, def int) int {
	v := q.Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
