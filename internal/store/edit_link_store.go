package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// SaveEditLink persists one hashed edit token.
func (s *Store) SaveEditLink(ctx context.Context, record schedule.EditLinkRecord) error {
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO schedule_edit_links(
			token_hash, group_openid, member_openid, expires_at, created_at
		 ) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(token_hash) DO UPDATE SET
		 group_openid = excluded.group_openid,
		 member_openid = excluded.member_openid,
		 expires_at = excluded.expires_at,
		 created_at = excluded.created_at`,
		record.TokenHash, record.GroupOpenID, record.MemberOpenID,
		record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("保存课表修改链接失败: %w", err)
	}
	return nil
}

// GetEditLink reads one hashed edit token.
func (s *Store) GetEditLink(ctx context.Context, tokenHash string) (schedule.EditLinkRecord, bool, error) {
	var record schedule.EditLinkRecord
	var expiresAt, createdAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT token_hash, group_openid, member_openid, expires_at, created_at
		 FROM schedule_edit_links WHERE token_hash = ?`,
		tokenHash,
	).Scan(
		&record.TokenHash, &record.GroupOpenID, &record.MemberOpenID,
		&expiresAt, &createdAt,
	)
	if err == sql.ErrNoRows {
		return schedule.EditLinkRecord{}, false, nil
	}
	if err != nil {
		return schedule.EditLinkRecord{}, false, fmt.Errorf("读取课表修改链接失败: %w", err)
	}
	record.ExpiresAt = parseStoredTime(expiresAt)
	record.CreatedAt = parseStoredTime(createdAt)
	return record, true, nil
}

// DeleteEditLink removes one hashed edit token.
func (s *Store) DeleteEditLink(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM schedule_edit_links WHERE token_hash = ?`,
		tokenHash,
	); err != nil {
		return fmt.Errorf("删除课表修改链接失败: %w", err)
	}
	return nil
}
