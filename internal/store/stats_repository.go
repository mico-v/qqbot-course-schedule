package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// InsertMessageStats appends one handling record.
func (s *Store) InsertMessageStats(ctx context.Context, record schedule.MessageStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var groupOpenID, memberOpenID any
	if ref, err := schedule.ParseLegacyScopeRef(record.ScopeID); err == nil && ref.Kind == "group" {
		groupOpenID = ref.ID
		if strings.TrimSpace(record.UserID) != "" {
			memberOpenID = record.UserID
		}
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO message_stats(
			scope_id, group_openid, member_openid, origin, command, user_id, stage, received_at,
			render_ms, upload_ms, send_ms, server_ms, ok, err_code)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ScopeID, groupOpenID, memberOpenID, record.Origin, record.Command,
		record.UserID, string(record.Stage),
		record.ReceivedAt.UTC().Format(time.RFC3339Nano),
		record.RenderMS, record.UploadMS, record.SendMS, record.ServerMS,
		boolToInt(record.OK), record.ErrCode,
	)
	if err != nil {
		return fmt.Errorf("写入耗时统计失败: %w", err)
	}
	return nil
}

// ListMessageStats returns records received at or after since.
func (s *Store) ListMessageStats(ctx context.Context, since time.Time, group *schedule.GroupKey) ([]schedule.MessageStats, error) {
	query := `SELECT scope_id, origin, command, user_id, stage, received_at,
		render_ms, upload_ms, send_ms, server_ms, ok, err_code
		FROM message_stats WHERE received_at >= ?`
	args := []any{since.UTC().Format(time.RFC3339Nano)}
	if group != nil {
		query += ` AND group_openid = ?`
		args = append(args, group.GroupOpenID)
	}
	query += ` ORDER BY received_at`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("读取耗时统计失败: %w", err)
	}
	defer rows.Close()
	var records []schedule.MessageStats
	for rows.Next() {
		var record schedule.MessageStats
		var stage, receivedAt string
		var ok int
		if err := rows.Scan(
			&record.ScopeID, &record.Origin, &record.Command, &record.UserID, &stage, &receivedAt,
			&record.RenderMS, &record.UploadMS, &record.SendMS, &record.ServerMS, &ok, &record.ErrCode,
		); err != nil {
			return nil, fmt.Errorf("读取耗时统计行失败: %w", err)
		}
		record.Stage = schedule.StatsStage(stage)
		record.OK = ok != 0
		record.ReceivedAt, _ = time.Parse(time.RFC3339Nano, receivedAt)
		records = append(records, record)
	}
	return records, rows.Err()
}

// PruneMessageStats deletes records received before the cutoff.
func (s *Store) PruneMessageStats(ctx context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM message_stats WHERE received_at < ?`,
		before.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("清理耗时统计失败: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(removed), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
