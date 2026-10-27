package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// GetGroupMember reads one durable group member profile.
func (s *Store) GetGroupMember(ctx context.Context, key schedule.GroupMemberKey) (schedule.GroupMember, bool, error) {
	if err := key.Validate(); err != nil {
		return schedule.GroupMember{}, false, err
	}
	var member schedule.GroupMember
	var union sql.NullString
	var firstSeen, lastSeen, lastRoster sql.NullString
	var inRoster int
	err := s.db.QueryRowContext(
		ctx,
		`SELECT display_name, qq, union_openid, first_seen_at, last_seen_at,
		        last_roster_at, is_in_roster
		 FROM group_members WHERE group_openid = ? AND member_openid = ?`,
		key.GroupOpenID, key.MemberOpenID,
	).Scan(
		&member.DisplayName, &member.QQ, &union, &firstSeen, &lastSeen,
		&lastRoster, &inRoster,
	)
	if err == sql.ErrNoRows {
		return schedule.GroupMember{}, false, nil
	}
	if err != nil {
		return schedule.GroupMember{}, false, fmt.Errorf("读取群成员失败: %w", err)
	}
	member.Key = key
	member.UnionOpenID = union.String
	member.FirstSeenAt = parseStoredTime(firstSeen.String)
	member.LastSeenAt = parseStoredTime(lastSeen.String)
	member.LastRosterAt = parseStoredTime(lastRoster.String)
	member.InRoster = inRoster != 0
	return member, true, nil
}

// ListGroupMembers reads all known member profiles in one group.
func (s *Store) ListGroupMembers(ctx context.Context, group schedule.GroupKey) ([]schedule.GroupMember, error) {
	if err := group.Validate(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT member_openid, display_name, qq, union_openid,
		        first_seen_at, last_seen_at, last_roster_at, is_in_roster
		 FROM group_members WHERE group_openid = ? ORDER BY member_openid`,
		group.GroupOpenID,
	)
	if err != nil {
		return nil, fmt.Errorf("读取群成员列表失败: %w", err)
	}
	defer rows.Close()
	var result []schedule.GroupMember
	for rows.Next() {
		var member schedule.GroupMember
		var union, firstSeen, lastSeen, lastRoster sql.NullString
		var inRoster int
		if err := rows.Scan(
			&member.Key.MemberOpenID, &member.DisplayName, &member.QQ, &union,
			&firstSeen, &lastSeen, &lastRoster, &inRoster,
		); err != nil {
			return nil, fmt.Errorf("读取群成员行失败: %w", err)
		}
		member.Key.GroupOpenID = group.GroupOpenID
		member.UnionOpenID = union.String
		member.FirstSeenAt = parseStoredTime(firstSeen.String)
		member.LastSeenAt = parseStoredTime(lastSeen.String)
		member.LastRosterAt = parseStoredTime(lastRoster.String)
		member.InRoster = inRoster != 0
		result = append(result, member)
	}
	return result, rows.Err()
}

// RecordGroupMemberSeen upserts one group member observation.
func (s *Store) RecordGroupMemberSeen(ctx context.Context, observation schedule.GroupMemberObservation) error {
	if err := observation.Key.Validate(); err != nil {
		return err
	}
	observedAt := observation.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	shownAt := observedAt.Format(time.RFC3339Nano)
	displayName := strings.TrimSpace(observation.DisplayName)
	insertName := firstNonEmpty(displayName, observation.Key.MemberOpenID)
	unionOpenID := nullableString(strings.TrimSpace(observation.UnionOpenID))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启群成员记录事务失败: %w", err)
	}
	defer tx.Rollback()
	if err := ensureGroupTx(ctx, tx, observation.Key.GroupOpenID, shownAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO group_members(
			group_openid, member_openid, display_name, union_openid,
			first_seen_at, last_seen_at, is_in_roster
		 ) VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_openid, member_openid) DO UPDATE SET
			display_name = CASE
				WHEN ? = 1 THEN excluded.display_name
				ELSE group_members.display_name
			END,
			union_openid = CASE
				WHEN excluded.union_openid IS NOT NULL THEN excluded.union_openid
				ELSE group_members.union_openid
			END,
			last_seen_at = excluded.last_seen_at,
			is_in_roster = CASE
				WHEN excluded.is_in_roster = 1 THEN 1
				ELSE group_members.is_in_roster
			END`,
		observation.Key.GroupOpenID, observation.Key.MemberOpenID, insertName,
		unionOpenID, shownAt, shownAt, boolToInt(observation.InRoster),
		boolToInt(displayName != ""),
	); err != nil {
		return fmt.Errorf("记录群成员失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交群成员记录失败: %w", err)
	}
	return nil
}

// ReplaceGroupRoster atomically replaces one group's roster observation.
func (s *Store) ReplaceGroupRoster(ctx context.Context, group schedule.GroupKey, members []schedule.GroupMemberObservation) error {
	if err := group.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	shownAt := now.Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启群成员快照事务失败: %w", err)
	}
	defer tx.Rollback()
	if err := ensureGroupTx(ctx, tx, group.GroupOpenID, shownAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE group_members SET is_in_roster = 0 WHERE group_openid = ?`,
		group.GroupOpenID,
	); err != nil {
		return fmt.Errorf("重置群成员快照失败: %w", err)
	}
	seen := make(map[string]bool)
	for _, observation := range members {
		observation.Key.GroupOpenID = group.GroupOpenID
		observation.Key.MemberOpenID = strings.TrimSpace(observation.Key.MemberOpenID)
		if observation.Key.MemberOpenID == "" || seen[observation.Key.MemberOpenID] {
			continue
		}
		seen[observation.Key.MemberOpenID] = true
		displayName := firstNonEmpty(observation.DisplayName, observation.Key.MemberOpenID)
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO group_members(
				group_openid, member_openid, display_name, union_openid,
				first_seen_at, last_seen_at, last_roster_at, is_in_roster
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, 1)
			 ON CONFLICT(group_openid, member_openid) DO UPDATE SET
				display_name = CASE
					WHEN excluded.display_name <> '' THEN excluded.display_name
					ELSE group_members.display_name
				END,
				union_openid = CASE
					WHEN excluded.union_openid IS NOT NULL THEN excluded.union_openid
					ELSE group_members.union_openid
				END,
				last_seen_at = excluded.last_seen_at,
				last_roster_at = excluded.last_roster_at,
				is_in_roster = 1`,
			group.GroupOpenID, observation.Key.MemberOpenID, displayName,
			nullableString(observation.UnionOpenID), shownAt, shownAt, shownAt,
		); err != nil {
			return fmt.Errorf("保存群成员快照失败: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交群成员快照失败: %w", err)
	}
	return nil
}

func ensureGroupTx(ctx context.Context, tx *sql.Tx, groupOpenID, now string) error {
	if strings.TrimSpace(groupOpenID) == "" {
		return schedule.ValidationErrorf("group_openid 不能为空")
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO groups(group_openid, created_at, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(group_openid) DO UPDATE SET updated_at = excluded.updated_at`,
		groupOpenID, now, now,
	); err != nil {
		return fmt.Errorf("保存群记录失败: %w", err)
	}
	return nil
}

func parseStoredTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	if parsed.IsZero() {
		parsed, _ = time.Parse(time.RFC3339, value)
	}
	return parsed
}

func groupKeyFromScope(scopeID, memberOpenID string) (schedule.GroupMemberKey, bool, error) {
	ref, err := schedule.ParseLegacyScopeRef(scopeID)
	if err != nil {
		return schedule.GroupMemberKey{}, false, err
	}
	if ref.Kind != "group" {
		return schedule.GroupMemberKey{}, false, nil
	}
	key := schedule.GroupMemberKey{
		GroupOpenID:  ref.ID,
		MemberOpenID: strings.TrimSpace(memberOpenID),
	}
	if err := key.Validate(); err != nil {
		return schedule.GroupMemberKey{}, true, err
	}
	return key, true, nil
}

// ListScopeSummaries returns group schedules in the admin application DTO.
func (s *Store) ListScopeSummaries() ([]admin.ScopeSummary, error) {
	rows, err := s.db.Query(
		`SELECT m.group_openid, m.member_openid, m.display_name, s.revision,
		        (SELECT COUNT(*) FROM group_member_course_events e
		          WHERE e.group_openid = m.group_openid
		            AND e.member_openid = m.member_openid)
		 FROM group_member_schedules s
		 JOIN group_members m
		   ON m.group_openid = s.group_openid AND m.member_openid = s.member_openid
		 ORDER BY m.group_openid, m.member_openid`,
	)
	if err != nil {
		return nil, fmt.Errorf("读取会话列表失败: %w", err)
	}
	defer rows.Close()
	var summaries []admin.ScopeSummary
	currentScope := ""
	for rows.Next() {
		var groupOpenID, userID, name string
		var revision int64
		var eventCount int
		if err := rows.Scan(&groupOpenID, &userID, &name, &revision, &eventCount); err != nil {
			return nil, fmt.Errorf("读取会话行失败: %w", err)
		}
		scopeID := schedule.ScopeGroup(groupOpenID)
		if currentScope != scopeID {
			summaries = append(summaries, admin.ScopeSummary{ScopeID: scopeID})
			currentScope = scopeID
		}
		if name == "" {
			name = userID
		}
		last := &summaries[len(summaries)-1]
		last.Members = append(last.Members, admin.ScopeMemberSummary{
			UserID: userID, Name: name, EventCount: eventCount, Revision: revision,
		})
	}
	return summaries, rows.Err()
}

// ListKnownGroups returns every group observed through any member operation.
func (s *Store) ListKnownGroups() ([]string, error) {
	rows, err := s.db.Query(`SELECT group_openid FROM groups ORDER BY group_openid`)
	if err != nil {
		return nil, fmt.Errorf("读取群列表失败: %w", err)
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var groupOpenID string
		if err := rows.Scan(&groupOpenID); err != nil {
			return nil, fmt.Errorf("读取群列表行失败: %w", err)
		}
		groups = append(groups, groupOpenID)
	}
	return groups, rows.Err()
}
