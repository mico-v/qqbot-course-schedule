package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// GetMember returns one member inside a legacy scope. Group scopes are routed
// to the v7 repository; non-group scopes remain read/write compatible.
func (s *Store) GetMember(scopeID, userID string) (*schedule.Member, bool, error) {
	if key, group, err := groupKeyFromScope(scopeID, userID); err != nil {
		return nil, false, err
	} else if group {
		return s.GetGroupMemberSchedule(context.Background(), key)
	}
	var dataJSON string
	var revision int64
	err := s.db.QueryRow(
		`SELECT data_json, revision FROM schedule_members WHERE scope_id = ? AND user_id = ?`,
		scopeID, userID,
	).Scan(&dataJSON, &revision)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取成员失败: %w", err)
	}
	events, err := s.memberEvents(scopeID, userID)
	if err != nil {
		return nil, false, err
	}
	member, err := decodeMember(scopeID, userID, dataJSON, revision, events)
	if err != nil {
		return nil, false, err
	}
	overrides, err := s.overridesForScope(scopeID)
	if err != nil {
		return nil, false, err
	}
	member.DayOverrides = mergeOverrides(overrides[schedule.DayOverrideAll], overrides[userID])
	return member, true, nil
}

// GetScopeMembers returns every member in a legacy scope, or v7 group rows.
func (s *Store) GetScopeMembers(scopeID string) (map[string]*schedule.Member, error) {
	if ref, err := schedule.ParseLegacyScopeRef(scopeID); err != nil {
		return nil, err
	} else if ref.Kind == "group" {
		return s.ListGroupSchedules(context.Background(), schedule.GroupKey{GroupOpenID: ref.ID})
	}
	rows, err := s.db.Query(
		`SELECT user_id, data_json, revision FROM schedule_members WHERE scope_id = ? ORDER BY user_id`,
		scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取成员列表失败: %w", err)
	}
	defer rows.Close()

	members := make(map[string]*schedule.Member)
	for rows.Next() {
		var userID, dataJSON string
		var revision int64
		if err := rows.Scan(&userID, &dataJSON, &revision); err != nil {
			return nil, fmt.Errorf("读取成员行失败: %w", err)
		}
		member, err := decodeMember(scopeID, userID, dataJSON, revision, nil)
		if err != nil {
			return nil, err
		}
		members[userID] = member
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return members, nil
	}

	eventsByMember, err := s.scopeEvents(scopeID)
	if err != nil {
		return nil, err
	}
	for userID, member := range members {
		member.Events = eventsByMember[userID]
	}

	overrides, err := s.overridesForScope(scopeID)
	if err != nil {
		return nil, err
	}
	scopeWide := overrides[schedule.DayOverrideAll]
	for userID, member := range members {
		member.DayOverrides = mergeOverrides(scopeWide, overrides[userID])
	}
	return members, nil
}

// ListMemberScopes returns legacy scopes containing a member row.
func (s *Store) ListMemberScopes(userID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT scope_id FROM schedule_members WHERE user_id = ? ORDER BY scope_id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取成员课表位置失败: %w", err)
	}
	defer rows.Close()
	var scopeIDs []string
	for rows.Next() {
		var scopeID string
		if err := rows.Scan(&scopeID); err != nil {
			return nil, fmt.Errorf("读取成员课表位置行失败: %w", err)
		}
		scopeIDs = append(scopeIDs, scopeID)
	}
	return scopeIDs, rows.Err()
}

// ListMembersByImportUnionOpenID returns v7 and private compatibility rows.
func (s *Store) ListMembersByImportUnionOpenID(unionOpenID string) ([]schedule.MemberLocation, error) {
	unionOpenID = strings.TrimSpace(unionOpenID)
	if unionOpenID == "" {
		return nil, nil
	}
	locationsByKey := make(map[schedule.MemberLocation]bool)
	groupRows, err := s.db.Query(
		`SELECT group_openid, member_openid FROM group_member_schedules
		 WHERE import_union_openid = ? ORDER BY group_openid, member_openid`,
		unionOpenID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取群成员导入来源失败: %w", err)
	}
	for groupRows.Next() {
		var groupOpenID, memberOpenID string
		if err := groupRows.Scan(&groupOpenID, &memberOpenID); err != nil {
			groupRows.Close()
			return nil, fmt.Errorf("读取群成员导入来源行失败: %w", err)
		}
		locationsByKey[schedule.MemberLocation{
			ScopeID: schedule.ScopeGroup(groupOpenID), UserID: memberOpenID,
		}] = true
	}
	if err := groupRows.Close(); err != nil {
		return nil, fmt.Errorf("读取群成员导入来源失败: %w", err)
	}
	rows, err := s.db.Query(
		`SELECT scope_id, user_id, data_json FROM schedule_members
		 WHERE scope_id LIKE 'private:%' ORDER BY scope_id, user_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("读取成员导入来源失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scopeID, userID, dataJSON string
		if err := rows.Scan(&scopeID, &userID, &dataJSON); err != nil {
			return nil, fmt.Errorf("读取成员导入来源行失败: %w", err)
		}
		var meta memberMeta
		if err := json.Unmarshal([]byte(dataJSON), &meta); err != nil {
			continue
		}
		if strings.TrimSpace(meta.ImportUnionOpenID) == unionOpenID {
			locationsByKey[schedule.MemberLocation{ScopeID: scopeID, UserID: userID}] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	locations := make([]schedule.MemberLocation, 0, len(locationsByKey))
	for location := range locationsByKey {
		locations = append(locations, location)
	}
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].ScopeID != locations[j].ScopeID {
			return locations[i].ScopeID < locations[j].ScopeID
		}
		return locations[i].UserID < locations[j].UserID
	})
	return locations, nil
}

func (s *Store) memberEvents(scopeID, userID string) ([]schedule.Event, error) {
	rows, err := s.db.Query(
		`SELECT event_json FROM course_events WHERE scope_id = ? AND user_id = ? ORDER BY event_index`,
		scopeID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取课程事件失败: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) scopeEvents(scopeID string) (map[string][]schedule.Event, error) {
	rows, err := s.db.Query(
		`SELECT user_id, event_json FROM course_events WHERE scope_id = ? ORDER BY user_id, event_index`,
		scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取课程事件失败: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]schedule.Event)
	for rows.Next() {
		var userID, eventJSON string
		if err := rows.Scan(&userID, &eventJSON); err != nil {
			return nil, fmt.Errorf("读取课程事件行失败: %w", err)
		}
		event, err := decodeEvent(eventJSON)
		if err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], event)
	}
	return result, rows.Err()
}

func scanEvents(rows *sql.Rows) ([]schedule.Event, error) {
	var events []schedule.Event
	for rows.Next() {
		var eventJSON string
		if err := rows.Scan(&eventJSON); err != nil {
			return nil, fmt.Errorf("读取课程事件行失败: %w", err)
		}
		event, err := decodeEvent(eventJSON)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func decodeEvent(eventJSON string) (schedule.Event, error) {
	var event schedule.Event
	if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
		return nil, fmt.Errorf("解析课程事件失败: %w", err)
	}
	return event, nil
}

// PutMember writes a group v7 schedule or a legacy non-group row.
// expected == nil is rejected for groups and upserts for legacy compatibility.
func (s *Store) PutMember(scopeID, userID string, member *schedule.Member, expected *int64) error {
	if key, group, err := groupKeyFromScope(scopeID, userID); err != nil {
		return err
	} else if group {
		if expected == nil {
			return schedule.ValidationErrorf("成员课表写入必须携带 expected revision")
		}
		_, err := s.PutGroupMemberSchedule(
			context.Background(),
			key,
			member,
			schedule.Revision(*expected),
		)
		return err
	}
	if member == nil {
		return schedule.ValidationErrorf("成员数据不能为空")
	}
	dataJSON, events, err := encodeMember(member)
	if err != nil {
		return err
	}
	updatedAt := member.UpdatedAt
	if updatedAt == "" {
		updatedAt = schedule.NowISO()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	var currentRevision int64
	err = tx.QueryRow(
		`SELECT revision FROM schedule_members WHERE scope_id = ? AND user_id = ?`,
		scopeID, userID,
	).Scan(&currentRevision)
	switch {
	case err == sql.ErrNoRows:
		if expected != nil && *expected > 0 {
			return schedule.ErrConflict
		}
		if _, err := tx.Exec(
			`INSERT INTO schedule_members(scope_id, user_id, data_json, updated_at, revision)
			 VALUES (?, ?, ?, ?, 1)`,
			scopeID, userID, dataJSON, updatedAt,
		); err != nil {
			return fmt.Errorf("写入成员失败: %w", err)
		}
	case err != nil:
		return fmt.Errorf("读取成员版本失败: %w", err)
	default:
		if expected != nil && (*expected == 0 || *expected != currentRevision) {
			return schedule.ErrConflict
		}
		if _, err := tx.Exec(
			`UPDATE schedule_members SET data_json = ?, updated_at = ?, revision = ?
			 WHERE scope_id = ? AND user_id = ? AND revision = ?`,
			dataJSON, updatedAt, currentRevision+1, scopeID, userID, currentRevision,
		); err != nil {
			return fmt.Errorf("更新成员失败: %w", err)
		}
	}
	if _, err := tx.Exec(
		`DELETE FROM course_events WHERE scope_id = ? AND user_id = ?`,
		scopeID, userID,
	); err != nil {
		return fmt.Errorf("清理旧事件失败: %w", err)
	}
	for index, event := range events {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("编码课程事件失败: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO course_events(
				scope_id, user_id, event_index, uid, summary, location, description,
				dtstart, dtend, dtstart_tzid, dtend_tzid, rrule, dtstamp, event_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			scopeID, userID, index+1,
			event["UID"], event["SUMMARY"], event["LOCATION"], event["DESCRIPTION"],
			event["DTSTART"], event["DTEND"], event["DTSTART_TZID"], event["DTEND_TZID"],
			event["RRULE"], event["DTSTAMP"], string(eventJSON),
		); err != nil {
			return fmt.Errorf("写入课程事件失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	member.Revision = currentRevision + 1
	return nil
}
