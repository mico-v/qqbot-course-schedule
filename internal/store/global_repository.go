package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// FindGlobalSchedule resolves a member's most recent non-empty schedule across
// groups. Union matches are authoritative; local member ID lookup is used when
// the platform does not expose union_openid or no union match exists.
func (s *Store) FindGlobalSchedule(
	ctx context.Context,
	memberOpenID string,
	unionOpenID string,
	exclude *schedule.GroupMemberKey,
) (*schedule.Member, schedule.StorageTarget, bool, error) {
	memberOpenID = strings.TrimSpace(memberOpenID)
	unionOpenID = strings.TrimSpace(unionOpenID)
	if memberOpenID == "" && unionOpenID == "" {
		return nil, schedule.StorageTarget{}, false, nil
	}
	if unionOpenID != "" {
		if key, found, err := s.findGlobalV7ByUnion(ctx, unionOpenID, exclude); err != nil {
			return nil, schedule.StorageTarget{}, false, err
		} else if found {
			member, ok, err := s.GetGroupMemberSchedule(ctx, key)
			if err != nil || !ok {
				return nil, schedule.StorageTarget{}, false, err
			}
			return member, schedule.NewStorageTarget(key), true, nil
		}

		legacyScope := schedule.ScopeGlobalUser(unionOpenID)
		member, found, err := s.GetMember(legacyScope, unionOpenID)
		if err != nil {
			return nil, schedule.StorageTarget{}, false, err
		}
		if found && hasMemberEvents(member) {
			return member, schedule.StorageTarget{
				ScopeID: legacyScope, UserID: unionOpenID, LocalUserID: memberOpenID,
			}, true, nil
		}
	}
	if memberOpenID == "" {
		return nil, schedule.StorageTarget{}, false, nil
	}
	if key, found, err := s.findGlobalV7ByLocalMember(
		ctx, memberOpenID, exclude, unionOpenID != "",
	); err != nil {
		return nil, schedule.StorageTarget{}, false, err
	} else if found {
		member, ok, err := s.GetGroupMemberSchedule(ctx, key)
		if err != nil || !ok {
			return nil, schedule.StorageTarget{}, false, err
		}
		return member, schedule.NewStorageTarget(key), true, nil
	}

	legacyScope, legacyUser, found, err := s.findLegacyGlobalByLocalMember(ctx, memberOpenID)
	if err != nil || !found {
		return nil, schedule.StorageTarget{}, false, err
	}
	member, found, err := s.GetMember(legacyScope, legacyUser)
	if err != nil {
		return nil, schedule.StorageTarget{}, false, err
	}
	if !found || !hasMemberEvents(member) {
		return nil, schedule.StorageTarget{}, false, nil
	}
	return member, schedule.StorageTarget{
		ScopeID: legacyScope, UserID: legacyUser, LocalUserID: memberOpenID,
	}, true, nil
}

func (s *Store) findGlobalV7ByUnion(
	ctx context.Context,
	unionOpenID string,
	exclude *schedule.GroupMemberKey,
) (schedule.GroupMemberKey, bool, error) {
	query := `SELECT s.group_openid, s.member_openid
		FROM group_member_schedules s
		JOIN group_members m
		  ON m.group_openid = s.group_openid AND m.member_openid = s.member_openid
		WHERE (m.union_openid = ? OR s.import_union_openid = ?)
		  AND EXISTS (
		    SELECT 1 FROM group_member_course_events e
		    WHERE e.group_openid = s.group_openid
		      AND e.member_openid = s.member_openid
		  )`
	args := []any{unionOpenID, unionOpenID}
	if exclude != nil {
		query += ` AND NOT (s.group_openid = ? AND s.member_openid = ?)`
		args = append(args, exclude.GroupOpenID, exclude.MemberOpenID)
	}
	query += ` ORDER BY s.schedule_updated_at DESC, s.updated_at DESC,
		s.group_openid, s.member_openid LIMIT 1`
	return s.queryGlobalKey(ctx, query, args...)
}

func (s *Store) findGlobalV7ByLocalMember(
	ctx context.Context,
	memberOpenID string,
	exclude *schedule.GroupMemberKey,
	onlyWithoutUnion bool,
) (schedule.GroupMemberKey, bool, error) {
	query := `SELECT s.group_openid, s.member_openid
		FROM group_member_schedules s
		JOIN group_members current_member
		  ON current_member.group_openid = s.group_openid
		 AND current_member.member_openid = s.member_openid
		WHERE s.member_openid = ?
		  AND EXISTS (
		    SELECT 1 FROM group_member_course_events e
		    WHERE e.group_openid = s.group_openid
		      AND e.member_openid = s.member_openid
		  )`
	args := []any{memberOpenID}
	if onlyWithoutUnion {
		query += ` AND s.import_union_openid = ''
			AND (current_member.union_openid IS NULL OR current_member.union_openid = '')`
	}
	if exclude != nil {
		query += ` AND NOT (s.group_openid = ? AND s.member_openid = ?)`
		args = append(args, exclude.GroupOpenID, exclude.MemberOpenID)
	}
	query += ` ORDER BY s.schedule_updated_at DESC, s.updated_at DESC,
		s.group_openid LIMIT 1`
	return s.queryGlobalKey(ctx, query, args...)
}

func (s *Store) queryGlobalKey(
	ctx context.Context,
	query string,
	args ...any,
) (schedule.GroupMemberKey, bool, error) {
	var key schedule.GroupMemberKey
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&key.GroupOpenID, &key.MemberOpenID)
	if err == sql.ErrNoRows {
		return schedule.GroupMemberKey{}, false, nil
	}
	if err != nil {
		return schedule.GroupMemberKey{}, false, fmt.Errorf("全局查找成员课表失败: %w", err)
	}
	return key, true, nil
}

func (s *Store) findLegacyGlobalByLocalMember(
	ctx context.Context,
	memberOpenID string,
) (string, string, bool, error) {
	var scopeID, userID string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT scope_id, user_id FROM schedule_members
		 WHERE scope_id LIKE 'global-user:%'
		   AND (user_id = ? OR json_extract(data_json, '$.import_user_openid') = ?)
		   AND EXISTS (
		     SELECT 1 FROM course_events e
		     WHERE e.scope_id = schedule_members.scope_id
		       AND e.user_id = schedule_members.user_id
		   )
		 ORDER BY updated_at DESC, scope_id, user_id LIMIT 1`,
		memberOpenID, memberOpenID,
	).Scan(&scopeID, &userID)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("读取历史全局课表失败: %w", err)
	}
	return scopeID, userID, true, nil
}

// DeleteMemberSchedules removes one exact schedule in isolated mode. In global
// mode it validates the selected target revision and then removes every
// schedule that could be selected for the same member. Member profiles and
// seen state are intentionally retained.
func (s *Store) DeleteMemberSchedules(
	ctx context.Context,
	target schedule.StorageTarget,
	expected schedule.Revision,
	currentKey schedule.GroupMemberKey,
	unionOpenID string,
	global bool,
) (int, error) {
	if expected < 1 {
		return 0, schedule.ValidationErrorf("删除课表必须携带有效 revision")
	}
	deleted, err := s.deleteExactMemberSchedule(ctx, target, expected)
	if err != nil {
		return 0, err
	}
	if deleted == 0 {
		return 0, nil
	}
	if !global {
		return deleted, nil
	}

	groupDeleted, err := s.deleteMatchingGroupSchedules(ctx, currentKey, unionOpenID)
	if err != nil {
		return deleted, err
	}
	legacyDeleted, err := s.deleteMatchingLegacyGlobalSchedules(
		ctx, currentKey.MemberOpenID, unionOpenID,
	)
	if err != nil {
		return deleted + groupDeleted, err
	}
	return deleted + groupDeleted + legacyDeleted, nil
}

func (s *Store) deleteExactMemberSchedule(
	ctx context.Context,
	target schedule.StorageTarget,
	expected schedule.Revision,
) (int, error) {
	var result sql.Result
	var err error
	if target.Key.GroupOpenID != "" && target.Key.MemberOpenID != "" {
		result, err = s.db.ExecContext(
			ctx,
			`DELETE FROM group_member_schedules
			 WHERE group_openid = ? AND member_openid = ? AND revision = ?`,
			target.Key.GroupOpenID, target.Key.MemberOpenID, expected,
		)
	} else {
		result, err = s.db.ExecContext(
			ctx,
			`DELETE FROM schedule_members
			 WHERE scope_id = ? AND user_id = ? AND revision = ?`,
			target.ScopeID, target.UserID, expected,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("删除成员课表失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取删除结果失败: %w", err)
	}
	if affected > 0 {
		return int(affected), nil
	}
	exists, err := s.memberScheduleExists(ctx, target)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, schedule.ErrConflict
	}
	return 0, nil
}

func (s *Store) deleteMatchingGroupSchedules(
	ctx context.Context,
	currentKey schedule.GroupMemberKey,
	unionOpenID string,
) (int, error) {
	memberOpenID := strings.TrimSpace(currentKey.MemberOpenID)
	unionOpenID = strings.TrimSpace(unionOpenID)
	if memberOpenID == "" && unionOpenID == "" {
		return 0, nil
	}
	query := `DELETE FROM group_member_schedules
		WHERE (group_openid = ? AND member_openid = ?)`
	args := []any{currentKey.GroupOpenID, memberOpenID}
	if unionOpenID != "" {
		query += ` OR EXISTS (
		     SELECT 1 FROM group_members m
		     WHERE m.group_openid = group_member_schedules.group_openid
		       AND m.member_openid = group_member_schedules.member_openid
		       AND m.union_openid = ?
		   ) OR import_union_openid = ?
		   OR (
		     member_openid = ? AND import_union_openid = ''
		     AND EXISTS (
		       SELECT 1 FROM group_members m
		       WHERE m.group_openid = group_member_schedules.group_openid
		         AND m.member_openid = group_member_schedules.member_openid
		         AND (m.union_openid IS NULL OR m.union_openid = '')
		     )
		   )`
		args = append(args, unionOpenID, unionOpenID, memberOpenID)
	} else {
		query += ` OR member_openid = ?`
		args = append(args, memberOpenID)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("清理全局成员课表失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取全局课表删除结果失败: %w", err)
	}
	return int(affected), nil
}

func (s *Store) deleteMatchingLegacyGlobalSchedules(
	ctx context.Context,
	memberOpenID string,
	unionOpenID string,
) (int, error) {
	memberOpenID = strings.TrimSpace(memberOpenID)
	unionOpenID = strings.TrimSpace(unionOpenID)
	if memberOpenID == "" && unionOpenID == "" {
		return 0, nil
	}
	query := `DELETE FROM schedule_members
		WHERE scope_id LIKE 'global-user:%' AND (`
	args := []any{}
	if unionOpenID != "" {
		query += `scope_id = 'global-user:' || ?`
		args = append(args, unionOpenID)
	} else {
		query += `user_id = ?`
		args = append(args, memberOpenID)
	}
	query += ` OR json_extract(data_json, '$.import_user_openid') = ?)`
	args = append(args, memberOpenID)
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("清理历史全局课表失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取历史全局课表删除结果失败: %w", err)
	}
	return int(affected), nil
}

func (s *Store) memberScheduleExists(ctx context.Context, target schedule.StorageTarget) (bool, error) {
	var exists int
	var err error
	if target.Key.GroupOpenID != "" && target.Key.MemberOpenID != "" {
		err = s.db.QueryRowContext(
			ctx,
			`SELECT 1 FROM group_member_schedules
			 WHERE group_openid = ? AND member_openid = ?`,
			target.Key.GroupOpenID, target.Key.MemberOpenID,
		).Scan(&exists)
	} else {
		err = s.db.QueryRowContext(
			ctx,
			`SELECT 1 FROM schedule_members WHERE scope_id = ? AND user_id = ?`,
			target.ScopeID, target.UserID,
		).Scan(&exists)
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("检查成员课表是否存在失败: %w", err)
	}
	return true, nil
}

func hasMemberEvents(member *schedule.Member) bool {
	return member != nil && len(member.Events) > 0
}
