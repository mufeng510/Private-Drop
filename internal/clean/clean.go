// Package clean implements retention expiry and recycle-bin cleanup. It
// always removes database references before physical files and logs failures
// so the next run can retry.
package clean

import (
	"context"
	"strconv"
	"syscall"
	"time"

	"private-drop/internal/audit"
	"private-drop/internal/config"
	"private-drop/internal/db"
	"private-drop/internal/file"
	"private-drop/internal/message"
	"private-drop/internal/notify"
	"private-drop/internal/upload"
	"private-drop/internal/util"
)

type Service struct {
	db       *db.DB
	cfg      config.Config
	files    *file.Service
	messages *message.Service
	uploads  *upload.Service
	notify   *notify.Service
	audit    *audit.Service
}

func New(d *db.DB, cfg config.Config, f *file.Service, m *message.Service, u *upload.Service, n *notify.Service, a *audit.Service) *Service {
	return &Service{db: d, cfg: cfg, files: f, messages: m, uploads: u, notify: n, audit: a}
}

type Report struct {
	ExpiredMessages int `json:"expiredMessages"`
	ExpiredFiles    int `json:"expiredFiles"`
	PurgedMessages  int `json:"purgedMessages"`
	PurgedFiles     int `json:"purgedFiles"`
	RemovedEntities int `json:"removedEntities"`
	CleanedChunks   int `json:"cleanedChunks"`
	FailedUploads   int `json:"failedUploads"`
	Failures        int `json:"failures"`
}

func (s *Service) setting(ctx context.Context, key string, fallback int) int {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

// Run performs one cleanup pass. It is safe to call repeatedly.
func (s *Service) Run(ctx context.Context) (Report, error) {
	var rep Report
	now := time.Now().UTC()
	recycleDays := s.setting(ctx, "recycle_days", 30)

	// 1. Expired, non-favorited messages move to the recycle bin (DB first).
	expired, err := s.db.QueryContext(ctx, `
        SELECT id FROM messages
        WHERE deleted_at IS NULL AND favorited=0 AND expires_at IS NOT NULL AND expires_at < ?`,
		util.Format(now))
	if err != nil {
		return rep, err
	}
	var expiredIDs []string
	for expired.Next() {
		var id string
		if err := expired.Scan(&id); err != nil {
			expired.Close()
			return rep, err
		}
		expiredIDs = append(expiredIDs, id)
	}
	expired.Close()
	for _, id := range expiredIDs {
		if err := s.messages.SoftDelete(ctx, id); err != nil {
			rep.Failures++
			s.audit.AppLog(ctx, "clean", "过期消息清理失败 "+id+": "+err.Error())
			continue
		}
		rep.ExpiredMessages++
	}
	if rep.ExpiredMessages > 0 && s.notify.Enabled(ctx, "notify_expiring") {
		_, _ = s.notify.Create(ctx, "expiry", map[string]any{
			"count": rep.ExpiredMessages, "message": "内容已过期并移入回收站",
		})
	}

	// 2. Orphaned, non-favorited attachments past retention move to recycle.
	retentionDays := s.setting(ctx, "retention_days", 30)
	cutoff := util.Format(now.Add(-time.Duration(retentionDays) * 24 * time.Hour))
	orphans, err := s.db.QueryContext(ctx, `
        SELECT a.id FROM attachments a
        WHERE a.deleted_at IS NULL AND a.favorited=0 AND a.uploaded_at < ?
        AND NOT EXISTS (SELECT 1 FROM message_attachments ma WHERE ma.attachment_id=a.id)`, cutoff)
	if err != nil {
		return rep, err
	}
	var orphanIDs []string
	for orphans.Next() {
		var id string
		if err := orphans.Scan(&id); err != nil {
			orphans.Close()
			return rep, err
		}
		orphanIDs = append(orphanIDs, id)
	}
	orphans.Close()
	for _, id := range orphanIDs {
		if err := s.files.SoftDelete(ctx, id); err != nil {
			rep.Failures++
			s.audit.AppLog(ctx, "clean", "过期附件清理失败 "+id+": "+err.Error())
			continue
		}
		rep.ExpiredFiles++
	}

	// 3. Permanently purge recycled content older than the recycle period.
	recycleCutoff := util.Format(now.Add(-time.Duration(recycleDays) * 24 * time.Hour))
	oldMsgs, err := s.db.QueryContext(ctx, `SELECT id FROM messages WHERE deleted_at IS NOT NULL AND deleted_at < ?`, recycleCutoff)
	if err != nil {
		return rep, err
	}
	var oldMsgIDs []string
	for oldMsgs.Next() {
		var id string
		if err := oldMsgs.Scan(&id); err != nil {
			oldMsgs.Close()
			return rep, err
		}
		oldMsgIDs = append(oldMsgIDs, id)
	}
	oldMsgs.Close()
	for _, id := range oldMsgIDs {
		if err := s.messages.Purge(ctx, id); err != nil {
			rep.Failures++
			s.audit.AppLog(ctx, "clean", "回收站永久删除消息失败 "+id+": "+err.Error())
			continue
		}
		rep.PurgedMessages++
	}

	oldFiles, err := s.db.QueryContext(ctx, `SELECT id FROM attachments WHERE deleted_at IS NOT NULL AND deleted_at < ?`, recycleCutoff)
	if err != nil {
		return rep, err
	}
	var oldFileIDs []string
	for oldFiles.Next() {
		var id string
		if err := oldFiles.Scan(&id); err != nil {
			oldFiles.Close()
			return rep, err
		}
		oldFileIDs = append(oldFileIDs, id)
	}
	oldFiles.Close()
	for _, id := range oldFileIDs {
		if err := s.files.Purge(ctx, id); err != nil {
			rep.Failures++
			s.audit.AppLog(ctx, "clean", "回收站永久删除附件失败 "+id+": "+err.Error())
			continue
		}
		rep.PurgedFiles++
	}

	// 4. Physical entity files with zero references.
	entities, err := s.db.QueryContext(ctx, `SELECT id FROM entity_files WHERE ref_count=0`)
	if err != nil {
		return rep, err
	}
	var entityIDs []string
	for entities.Next() {
		var id string
		if err := entities.Scan(&id); err != nil {
			entities.Close()
			return rep, err
		}
		entityIDs = append(entityIDs, id)
	}
	entities.Close()
	for _, id := range entityIDs {
		removed, err := s.files.DeleteEntityIfUnreferenced(ctx, id)
		if err != nil {
			rep.Failures++
			s.audit.AppLog(ctx, "clean", "实体文件清理失败 "+id+": "+err.Error())
			continue
		}
		if removed {
			rep.RemovedEntities++
		}
	}

	// 5. Stale upload chunks and inactive in-progress uploads.
	if n, err := s.uploads.CleanupStale(ctx, 48*time.Hour); err != nil {
		rep.Failures++
		s.audit.AppLog(ctx, "clean", "临时分片清理失败: "+err.Error())
	} else {
		rep.CleanedChunks = n
	}
	if n, err := s.uploads.ReapInactive(ctx, 24*time.Hour); err != nil {
		rep.Failures++
		s.audit.AppLog(ctx, "clean", "停滞上传任务清理失败: "+err.Error())
	} else {
		rep.FailedUploads = n
	}

	// 5b. Low-storage warning (notify only; never auto-delete or block uploads).
	s.checkStorage(ctx, rep)

	// 6. Expired sessions and MFA grants.
	nowStr := util.Format(now)
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at IS NOT NULL AND expires_at < ?`, nowStr)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM mfa_tokens WHERE expires_at < ?`, nowStr)

	s.audit.AppLog(ctx, "clean", "清理完成: "+reportString(rep))
	return rep, nil
}

// checkStorage warns once per day when the data volume passes the configured
// usage threshold. It never deletes data or blocks uploads.
func (s *Service) checkStorage(ctx context.Context, rep Report) {
	if !s.notify.Enabled(ctx, "notify_low_storage") {
		return
	}
	threshold := s.setting(ctx, "storage_warning_percent", 90)
	var st syscall.Statfs_t
	if err := syscall.Statfs(s.cfg.DataDir, &st); err != nil || st.Blocks == 0 {
		return
	}
	used := float64(st.Blocks-st.Bavail) / float64(st.Blocks) * 100
	if used < float64(threshold) {
		return
	}
	// Rate-limit: one low-storage notification per 24h.
	var recent int
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE type='low_storage' AND created_at>?`, cutoff).Scan(&recent)
	if recent > 0 {
		return
	}
	_, _ = s.notify.Create(ctx, "low_storage", map[string]any{
		"usedPercent": used, "threshold": threshold,
		"message": "存储空间已达到警戒值，请考虑清理回收站或旧文件",
	})
}

func reportString(r Report) string {
	return "过期消息=" + strconv.Itoa(r.ExpiredMessages) +
		" 过期附件=" + strconv.Itoa(r.ExpiredFiles) +
		" 永久删除消息=" + strconv.Itoa(r.PurgedMessages) +
		" 永久删除附件=" + strconv.Itoa(r.PurgedFiles) +
		" 释放实体=" + strconv.Itoa(r.RemovedEntities) +
		" 分片=" + strconv.Itoa(r.CleanedChunks) +
		" 停滞上传=" + strconv.Itoa(r.FailedUploads) +
		" 失败=" + strconv.Itoa(r.Failures)
}
