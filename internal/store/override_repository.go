package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// ListGroupDayOverrides returns group-wide and member-specific markers.
func (s *Store) ListGroupDayOverrides(ctx context.Context, group schedule.GroupKey) ([]schedule.DayOverrideRow, error) {
	if err := group.Validate(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT group_openid, COALESCE(member_openid, ''), day, kind,
		        source_day, actor_openid, created_at
		 FROM group_day_overrides WHERE group_openid = ? ORDER BY day, member_openid`,
		group.GroupOpenID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取群标记失败: %w", err)
	}
	defer rows.Close()
	var result []schedule.DayOverrideRow
	for rows.Next() {
		var row schedule.DayOverrideRow
		if err := rows.Scan(
			&row.ScopeID, &row.UserID, &row.Day, &row.Kind,
			&row.SourceDay, &row.CreatedBy, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("读取群标记行失败: %w", err)
		}
		row.ScopeID = group.ScopeID()
		if row.UserID == "" {
			row.UserID = schedule.DayOverrideAll
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// SetGroupDayOverride upserts one group-wide or member-specific marker.
func (s *Store) SetGroupDayOverride(ctx context.Context, row schedule.DayOverrideRow) error {
	kind, groupOpenID := schedule.ParseScope(row.ScopeID)
	if kind != "group" || strings.TrimSpace(groupOpenID) == "" {
		return schedule.ValidationErrorf("无效的群聊 scope_id")
	}
	memberOpenID := strings.TrimSpace(row.UserID)
	var member any
	if memberOpenID != "" && memberOpenID != schedule.DayOverrideAll {
		member = memberOpenID
	}
	if memberOpenID != "" && memberOpenID != schedule.DayOverrideAll {
		var exists int
		err := s.db.QueryRowContext(
			ctx,
			`SELECT 1 FROM group_members WHERE group_openid = ? AND member_openid = ?`,
			groupOpenID, memberOpenID,
		).Scan(&exists)
		if err == sql.ErrNoRows {
			return schedule.NotFoundErrorf("找不到该群成员")
		}
		if err != nil {
			return fmt.Errorf("检查群成员失败: %w", err)
		}
	}
	now := firstNonEmpty(row.CreatedAt, schedule.NowISO())
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO groups(group_openid, created_at, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(group_openid) DO NOTHING`,
		groupOpenID, now, now,
	); err != nil {
		return fmt.Errorf("保存群记录失败: %w", err)
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO group_day_overrides(
			group_openid, member_openid, day, kind, source_day, actor_openid, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT DO UPDATE SET
			kind = excluded.kind,
			source_day = excluded.source_day,
			actor_openid = excluded.actor_openid,
			created_at = excluded.created_at`,
		groupOpenID, member, row.Day, row.Kind, row.SourceDay, row.CreatedBy, now,
	)
	if err != nil {
		return fmt.Errorf("保存群标记失败: %w", err)
	}
	return nil
}

// DeleteGroupDayOverride removes one marker and reports whether it existed.
func (s *Store) DeleteGroupDayOverride(ctx context.Context, group schedule.GroupKey, memberOpenID, day string) (bool, error) {
	if err := group.Validate(); err != nil {
		return false, err
	}
	var member any
	if memberOpenID != "" && memberOpenID != schedule.DayOverrideAll {
		member = memberOpenID
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM group_day_overrides
		 WHERE group_openid = ? AND ((member_openid IS NULL AND ? IS NULL) OR member_openid = ?)
		   AND day = ?`,
		group.GroupOpenID, member, member, day,
	)
	if err != nil {
		return false, fmt.Errorf("删除群标记失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// DeleteAllGroupDayOverrides removes every marker in a group.
func (s *Store) DeleteAllGroupDayOverrides(ctx context.Context, group schedule.GroupKey) error {
	if err := group.Validate(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM group_day_overrides WHERE group_openid = ?`,
		group.GroupOpenID,
	); err != nil {
		return fmt.Errorf("清空群标记失败: %w", err)
	}
	return nil
}

// ListDayOverrides returns v7 group rows or v6 legacy rows.
func (s *Store) ListDayOverrides(scopeID string) ([]schedule.DayOverrideRow, error) {
	if ref, err := schedule.ParseLegacyScopeRef(scopeID); err != nil {
		return nil, err
	} else if ref.Kind == "group" {
		return s.ListGroupDayOverrides(context.Background(), schedule.GroupKey{GroupOpenID: ref.ID})
	}
	rows, err := s.db.Query(
		`SELECT scope_id, user_id, day, kind, source_day, created_by, created_at
		 FROM schedule_day_overrides WHERE scope_id = ? ORDER BY day, user_id`,
		scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取休假标记失败: %w", err)
	}
	defer rows.Close()
	var result []schedule.DayOverrideRow
	for rows.Next() {
		var row schedule.DayOverrideRow
		if err := rows.Scan(
			&row.ScopeID, &row.UserID, &row.Day, &row.Kind,
			&row.SourceDay, &row.CreatedBy, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("读取休假标记行失败: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// DeleteScopeDayOverrides removes all markers in a group or legacy scope.
func (s *Store) DeleteScopeDayOverrides(scopeID string) error {
	if ref, err := schedule.ParseLegacyScopeRef(scopeID); err != nil {
		return err
	} else if ref.Kind == "group" {
		return s.DeleteAllGroupDayOverrides(
			context.Background(),
			schedule.GroupKey{GroupOpenID: ref.ID},
		)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`DELETE FROM schedule_day_overrides WHERE scope_id = ?`,
		scopeID,
	); err != nil {
		return fmt.Errorf("清空休假标记失败: %w", err)
	}
	return nil
}

// SetDayOverride writes a group marker or legacy marker.
func (s *Store) SetDayOverride(scopeID, userID, day string, override schedule.DayOverride, createdBy, createdAt string) error {
	if ref, err := schedule.ParseLegacyScopeRef(scopeID); err != nil {
		return err
	} else if ref.Kind == "group" {
		return s.SetGroupDayOverride(context.Background(), schedule.DayOverrideRow{
			ScopeID: scopeID, UserID: userID, Day: day, Kind: override.Kind,
			SourceDay: override.SourceDay, CreatedBy: createdBy, CreatedAt: createdAt,
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO schedule_day_overrides(scope_id, user_id, day, kind, source_day, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope_id, user_id, day) DO UPDATE SET
			kind = excluded.kind,
			source_day = excluded.source_day,
			created_by = excluded.created_by,
			created_at = excluded.created_at`,
		scopeID, userID, day, override.Kind, override.SourceDay, createdBy, createdAt,
	)
	if err != nil {
		return fmt.Errorf("写入休假标记失败: %w", err)
	}
	return nil
}

// DeleteDayOverride removes one group or legacy marker.
func (s *Store) DeleteDayOverride(scopeID, userID, day string) (bool, error) {
	if ref, err := schedule.ParseLegacyScopeRef(scopeID); err != nil {
		return false, err
	} else if ref.Kind == "group" {
		return s.DeleteGroupDayOverride(
			context.Background(),
			schedule.GroupKey{GroupOpenID: ref.ID},
			userID,
			day,
		)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(
		`DELETE FROM schedule_day_overrides WHERE scope_id = ? AND user_id = ? AND day = ?`,
		scopeID, userID, day,
	)
	if err != nil {
		return false, fmt.Errorf("删除休假标记失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) overridesForScope(scopeID string) (map[string]map[string]schedule.DayOverride, error) {
	rows, err := s.ListDayOverrides(scopeID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]schedule.DayOverride)
	for _, row := range rows {
		if result[row.UserID] == nil {
			result[row.UserID] = make(map[string]schedule.DayOverride)
		}
		result[row.UserID][row.Day] = schedule.DayOverride{
			Kind: row.Kind, SourceDay: row.SourceDay,
		}
	}
	return result, nil
}

func mergeOverrides(scopeWide, member map[string]schedule.DayOverride) map[string]schedule.DayOverride {
	merged := make(map[string]schedule.DayOverride, len(scopeWide)+len(member))
	for day, override := range scopeWide {
		merged[day] = override
	}
	for day, override := range member {
		merged[day] = override
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
