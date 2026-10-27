package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// GetGroupMemberSchedule reads one v7 schedule with member profile and
// attached overrides.
func (s *Store) GetGroupMemberSchedule(ctx context.Context, key schedule.GroupMemberKey) (*schedule.Member, bool, error) {
	if err := key.Validate(); err != nil {
		return nil, false, err
	}
	var member schedule.Member
	var revision int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT m.display_name, m.qq, s.source, s.source_file, s.uploader_openid,
		        s.import_group_openid, s.import_member_openid, s.import_union_openid,
		        s.ics, s.schedule_text, s.updated_at, s.schedule_updated_at,
		        s.last_modified_at, s.last_modified_by, s.revision
		 FROM group_member_schedules s
		 JOIN group_members m
		   ON m.group_openid = s.group_openid AND m.member_openid = s.member_openid
		 WHERE s.group_openid = ? AND s.member_openid = ?`,
		key.GroupOpenID, key.MemberOpenID,
	).Scan(
		&member.Name, &member.QQ, &member.Source, &member.SourceFile,
		&member.UploaderID, &member.ImportGroupOpenID, &member.ImportUserOpenID,
		&member.ImportUnionOpenID, &member.ICS, &member.Schedule,
		&member.UpdatedAt, &member.ScheduleUpdatedAt, &member.LastModifiedAt,
		&member.LastModifiedBy, &revision,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取成员课表失败: %w", err)
	}
	member.UserID = key.MemberOpenID
	member.Revision = revision
	events, err := s.groupMemberEvents(ctx, key)
	if err != nil {
		return nil, false, err
	}
	member.Events = events
	member.EventCount = len(events)
	overrides, err := s.groupOverrides(ctx, key.Group())
	if err != nil {
		return nil, false, err
	}
	member.DayOverrides = mergeOverrides(overrides[schedule.DayOverrideAll], overrides[key.MemberOpenID])
	return &member, true, nil
}

// ListGroupSchedules reads every v7 schedule in one group.
func (s *Store) ListGroupSchedules(ctx context.Context, group schedule.GroupKey) (map[string]*schedule.Member, error) {
	if err := group.Validate(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT m.member_openid, m.display_name, m.qq, s.source, s.source_file,
		        s.uploader_openid, s.import_group_openid, s.import_member_openid,
		        s.import_union_openid, s.ics, s.schedule_text, s.updated_at,
		        s.schedule_updated_at, s.last_modified_at, s.last_modified_by, s.revision
		 FROM group_member_schedules s
		 JOIN group_members m
		   ON m.group_openid = s.group_openid AND m.member_openid = s.member_openid
		 WHERE s.group_openid = ? ORDER BY m.member_openid`,
		group.GroupOpenID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取成员课表列表失败: %w", err)
	}
	defer rows.Close()
	members := make(map[string]*schedule.Member)
	for rows.Next() {
		var member schedule.Member
		if err := rows.Scan(
			&member.UserID, &member.Name, &member.QQ, &member.Source, &member.SourceFile,
			&member.UploaderID, &member.ImportGroupOpenID, &member.ImportUserOpenID,
			&member.ImportUnionOpenID, &member.ICS, &member.Schedule, &member.UpdatedAt,
			&member.ScheduleUpdatedAt, &member.LastModifiedAt, &member.LastModifiedBy,
			&member.Revision,
		); err != nil {
			return nil, fmt.Errorf("读取成员课表行失败: %w", err)
		}
		members[member.UserID] = &member
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return members, nil
	}
	events, err := s.allGroupMemberEvents(ctx, group)
	if err != nil {
		return nil, err
	}
	overrides, err := s.groupOverrides(ctx, group)
	if err != nil {
		return nil, err
	}
	scopeWide := overrides[schedule.DayOverrideAll]
	for memberID, member := range members {
		member.Events = events[memberID]
		member.EventCount = len(member.Events)
		member.DayOverrides = mergeOverrides(scopeWide, overrides[memberID])
	}
	return members, nil
}

// PutGroupMemberSchedule writes one v7 schedule with revision CAS.
func (s *Store) PutGroupMemberSchedule(
	ctx context.Context,
	key schedule.GroupMemberKey,
	member *schedule.Member,
	expected schedule.Revision,
) (schedule.Revision, error) {
	if member == nil {
		return 0, schedule.ValidationErrorf("成员数据不能为空")
	}
	if err := key.Validate(); err != nil {
		return 0, err
	}
	if expected < 0 {
		return 0, schedule.ValidationErrorf("课表 revision 不能为负数")
	}
	if strings.TrimSpace(member.ImportGroupOpenID) == "" {
		member.ImportGroupOpenID = key.GroupOpenID
	}
	if strings.TrimSpace(member.ImportUserOpenID) == "" {
		member.ImportUserOpenID = key.MemberOpenID
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("开启课表事务失败: %w", err)
	}
	defer tx.Rollback()
	if err := ensureGroupTx(ctx, tx, key.GroupOpenID, now); err != nil {
		return 0, err
	}
	displayName := firstNonEmpty(member.Name, key.MemberOpenID)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO group_members(
			group_openid, member_openid, display_name, qq, union_openid,
			first_seen_at, last_seen_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_openid, member_openid) DO UPDATE SET
			display_name = excluded.display_name,
			qq = excluded.qq,
			union_openid = CASE
				WHEN excluded.union_openid IS NOT NULL AND excluded.union_openid <> ''
				THEN excluded.union_openid
				ELSE group_members.union_openid
			END,
			last_seen_at = excluded.last_seen_at`,
		key.GroupOpenID, key.MemberOpenID, displayName, member.QQ,
		nullableString(member.ImportUnionOpenID), now, now,
	); err != nil {
		return 0, fmt.Errorf("保存群成员资料失败: %w", err)
	}

	var current int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT revision FROM group_member_schedules
		 WHERE group_openid = ? AND member_openid = ?`,
		key.GroupOpenID, key.MemberOpenID,
	).Scan(&current)
	var next int64
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if expected > 0 {
			return 0, schedule.ErrConflict
		}
		next = 1
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO group_member_schedules(
				group_openid, member_openid, source, source_file, uploader_openid,
				import_group_openid, import_member_openid, import_union_openid,
				ics, schedule_text, created_at, updated_at, schedule_updated_at,
				last_modified_at, last_modified_by, revision
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			key.GroupOpenID, key.MemberOpenID, member.Source, member.SourceFile,
			member.UploaderID, member.ImportGroupOpenID, member.ImportUserOpenID,
			member.ImportUnionOpenID, member.ICS, member.Schedule,
			firstNonEmpty(member.UpdatedAt, now),
			firstNonEmpty(member.UpdatedAt, now),
			firstNonEmpty(member.ScheduleUpdatedAt, member.UpdatedAt, now),
			firstNonEmpty(member.LastModifiedAt, member.UpdatedAt, now),
			member.LastModifiedBy,
		); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return 0, schedule.ErrConflict
			}
			return 0, fmt.Errorf("创建成员课表失败: %w", err)
		}
	case err != nil:
		return 0, fmt.Errorf("读取课表版本失败: %w", err)
	default:
		if expected == 0 || int64(expected) != current {
			return 0, schedule.ErrConflict
		}
		next = current + 1
		result, err := tx.ExecContext(
			ctx,
			`UPDATE group_member_schedules SET
				source = ?, source_file = ?, uploader_openid = ?,
				import_group_openid = ?, import_member_openid = ?, import_union_openid = ?,
				ics = ?, schedule_text = ?, updated_at = ?, schedule_updated_at = ?,
				last_modified_at = ?, last_modified_by = ?, revision = ?
			 WHERE group_openid = ? AND member_openid = ? AND revision = ?`,
			member.Source, member.SourceFile, member.UploaderID,
			member.ImportGroupOpenID, member.ImportUserOpenID, member.ImportUnionOpenID,
			member.ICS, member.Schedule, firstNonEmpty(member.UpdatedAt, now),
			firstNonEmpty(member.ScheduleUpdatedAt, member.UpdatedAt, now),
			firstNonEmpty(member.LastModifiedAt, member.UpdatedAt, now),
			member.LastModifiedBy, next, key.GroupOpenID, key.MemberOpenID, current,
		)
		if err != nil {
			return 0, fmt.Errorf("更新成员课表失败: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("读取课表更新结果失败: %w", err)
		}
		if affected == 0 {
			return 0, schedule.ErrConflict
		}
	}
	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM group_member_course_events
		 WHERE group_openid = ? AND member_openid = ?`,
		key.GroupOpenID, key.MemberOpenID,
	); err != nil {
		return 0, fmt.Errorf("清理旧课程事件失败: %w", err)
	}
	for index, event := range member.Events {
		raw, err := json.Marshal(event)
		if err != nil {
			return 0, fmt.Errorf("编码课程事件失败: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO group_member_course_events(
				group_openid, member_openid, event_index, uid, summary, location,
				description, dtstart, dtend, dtstart_tzid, dtend_tzid, rrule,
				dtstamp, raw_json
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			key.GroupOpenID, key.MemberOpenID, index, event["UID"], event["SUMMARY"],
			event["LOCATION"], event["DESCRIPTION"], event["DTSTART"], event["DTEND"],
			event["DTSTART_TZID"], event["DTEND_TZID"], event["RRULE"],
			event["DTSTAMP"], string(raw),
		); err != nil {
			return 0, fmt.Errorf("保存课程事件失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交课表事务失败: %w", err)
	}
	member.Revision = next
	return schedule.Revision(next), nil
}

func (s *Store) groupMemberEvents(ctx context.Context, key schedule.GroupMemberKey) ([]schedule.Event, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT raw_json FROM group_member_course_events
		 WHERE group_openid = ? AND member_openid = ? ORDER BY event_index`,
		key.GroupOpenID, key.MemberOpenID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取课程事件失败: %w", err)
	}
	defer rows.Close()
	return scanRawEvents(rows)
}

func (s *Store) allGroupMemberEvents(ctx context.Context, group schedule.GroupKey) (map[string][]schedule.Event, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT member_openid, raw_json FROM group_member_course_events
		 WHERE group_openid = ? ORDER BY member_openid, event_index`,
		group.GroupOpenID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取群课程事件失败: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]schedule.Event)
	for rows.Next() {
		var memberOpenID, raw string
		if err := rows.Scan(&memberOpenID, &raw); err != nil {
			return nil, fmt.Errorf("读取群课程事件行失败: %w", err)
		}
		var event schedule.Event
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("解析群课程事件失败: %w", err)
		}
		result[memberOpenID] = append(result[memberOpenID], event)
	}
	return result, rows.Err()
}

func scanRawEvents(rows *sql.Rows) ([]schedule.Event, error) {
	var events []schedule.Event
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("读取课程事件行失败: %w", err)
		}
		var event schedule.Event
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("解析课程事件失败: %w", err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) groupOverrides(ctx context.Context, group schedule.GroupKey) (map[string]map[string]schedule.DayOverride, error) {
	rows, err := s.ListGroupDayOverrides(ctx, group)
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]schedule.DayOverride)
	for _, row := range rows {
		member := row.UserID
		if member == "" {
			member = schedule.DayOverrideAll
		}
		if result[member] == nil {
			result[member] = make(map[string]schedule.DayOverride)
		}
		result[member][row.Day] = schedule.DayOverride{
			Kind: row.Kind, SourceDay: row.SourceDay,
		}
	}
	return result, nil
}
