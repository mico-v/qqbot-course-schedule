// Package store persists course schedules in SQLite using the schema inherited
// from the AstrBot plugin (members, events, day overrides) plus a KV table.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"

	_ "modernc.org/sqlite"
)

const schemaVersion = 2

// Store is the SQLite-backed implementation of schedule.Storage.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Open opens (and creates) the database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("数据库路径不能为空")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// One connection avoids SQLITE_BUSY between pooled writers.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 30000",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置 %s 失败: %w", pragma, err)
		}
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS schedule_members (
			scope_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (scope_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_members_scope ON schedule_members(scope_id)`,
		`CREATE TABLE IF NOT EXISTS course_events (
			scope_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			event_index INTEGER NOT NULL,
			uid TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			location TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			dtstart TEXT NOT NULL DEFAULT '',
			dtend TEXT NOT NULL DEFAULT '',
			dtstart_tzid TEXT NOT NULL DEFAULT '',
			dtend_tzid TEXT NOT NULL DEFAULT '',
			rrule TEXT NOT NULL DEFAULT '',
			dtstamp TEXT NOT NULL DEFAULT '',
			event_json TEXT NOT NULL,
			PRIMARY KEY (scope_id, user_id, event_index),
			FOREIGN KEY (scope_id, user_id)
				REFERENCES schedule_members(scope_id, user_id)
				ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_course_events_scope_start ON course_events(scope_id, dtstart)`,
		`CREATE TABLE IF NOT EXISTS schedule_day_overrides (
			scope_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			source_day TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (scope_id, user_id, day)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_day_overrides_scope ON schedule_day_overrides(scope_id, day)`,
		`CREATE TABLE IF NOT EXISTS kv_data (
			scope TEXT NOT NULL,
			namespace TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (scope, namespace, key)
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("初始化数据库失败: %w", err)
		}
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO metadata(key, value) VALUES ('schema_version', ?)`,
		fmt.Sprint(schemaVersion),
	)
	if err != nil {
		return fmt.Errorf("写入 schema_version 失败: %w", err)
	}
	return nil
}

type memberMeta struct {
	Name              string `json:"name"`
	Schedule          string `json:"schedule"`
	Source            string `json:"source"`
	SourceFile        string `json:"source_file,omitempty"`
	UploaderID        string `json:"uploader_id,omitempty"`
	EventCount        int    `json:"event_count"`
	UpdatedAt         string `json:"updated_at"`
	ScheduleUpdatedAt string `json:"schedule_updated_at"`
	LastModifiedAt    string `json:"last_modified_at,omitempty"`
	LastModifiedBy    string `json:"last_modified_by,omitempty"`
	ICS               string `json:"ics"`
}

func encodeMember(member *schedule.Member) (string, []schedule.Event, error) {
	meta := memberMeta{
		Name:              member.Name,
		Schedule:          member.Schedule,
		Source:            member.Source,
		SourceFile:        member.SourceFile,
		UploaderID:        member.UploaderID,
		EventCount:        len(member.Events),
		UpdatedAt:         member.UpdatedAt,
		ScheduleUpdatedAt: member.ScheduleUpdatedAt,
		LastModifiedAt:    member.LastModifiedAt,
		LastModifiedBy:    member.LastModifiedBy,
		ICS:               member.ICS,
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return "", nil, fmt.Errorf("编码成员数据失败: %w", err)
	}
	return string(encoded), member.Events, nil
}

func decodeMember(userID, dataJSON string, revision int64, events []schedule.Event) (*schedule.Member, error) {
	var meta memberMeta
	if err := json.Unmarshal([]byte(dataJSON), &meta); err != nil {
		return nil, fmt.Errorf("解析成员 %s 数据失败: %w", userID, err)
	}
	return &schedule.Member{
		UserID:            userID,
		Name:              meta.Name,
		Events:            events,
		ICS:               meta.ICS,
		Schedule:          meta.Schedule,
		Source:            meta.Source,
		SourceFile:        meta.SourceFile,
		UploaderID:        meta.UploaderID,
		EventCount:        meta.EventCount,
		UpdatedAt:         meta.UpdatedAt,
		ScheduleUpdatedAt: meta.ScheduleUpdatedAt,
		LastModifiedAt:    meta.LastModifiedAt,
		LastModifiedBy:    meta.LastModifiedBy,
		Revision:          revision,
	}, nil
}

// GetMember returns one member inside a scope.
func (s *Store) GetMember(scopeID, userID string) (*schedule.Member, bool, error) {
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
	member, err := decodeMember(userID, dataJSON, revision, events)
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

// GetScopeMembers returns every member of a scope with day overrides attached.
func (s *Store) GetScopeMembers(scopeID string) (map[string]*schedule.Member, error) {
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
		member, err := decodeMember(userID, dataJSON, revision, nil)
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

// PutMember writes a member with optimistic locking.
// expected == nil upserts; *expected == 0 inserts only; *expected == N is CAS.
func (s *Store) PutMember(scopeID, userID string, member *schedule.Member, expected *int64) error {
	if member == nil {
		return fmt.Errorf("成员数据不能为空")
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

// ListScopeSummaries returns every scope with its member rows.
func (s *Store) ListScopeSummaries() ([]schedule.ScopeSummary, error) {
	rows, err := s.db.Query(
		`SELECT scope_id, user_id, data_json, revision FROM schedule_members ORDER BY scope_id, user_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("读取会话列表失败: %w", err)
	}
	defer rows.Close()

	var summaries []schedule.ScopeSummary
	currentScope := ""
	for rows.Next() {
		var scopeID, userID, dataJSON string
		var revision int64
		if err := rows.Scan(&scopeID, &userID, &dataJSON, &revision); err != nil {
			return nil, fmt.Errorf("读取会话行失败: %w", err)
		}
		if currentScope != scopeID {
			summaries = append(summaries, schedule.ScopeSummary{ScopeID: scopeID})
			currentScope = scopeID
		}
		var meta memberMeta
		_ = json.Unmarshal([]byte(dataJSON), &meta)
		name := meta.Name
		if name == "" {
			name = userID
		}
		last := &summaries[len(summaries)-1]
		last.Members = append(last.Members, schedule.ScopeMemberSummary{
			UserID:     userID,
			Name:       name,
			EventCount: meta.EventCount,
			Revision:   revision,
		})
	}
	return summaries, rows.Err()
}

// ListDayOverrides returns every marker in a scope.
func (s *Store) ListDayOverrides(scopeID string) ([]schedule.DayOverrideRow, error) {
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
		if err := rows.Scan(&row.ScopeID, &row.UserID, &row.Day, &row.Kind, &row.SourceDay, &row.CreatedBy, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取休假标记行失败: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// DeleteScopeDayOverrides removes every marker in a scope (backup restore).
func (s *Store) DeleteScopeDayOverrides(scopeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM schedule_day_overrides WHERE scope_id = ?`, scopeID); err != nil {
		return fmt.Errorf("清空休假标记失败: %w", err)
	}
	return nil
}

// SetDayOverride upserts one marker.
func (s *Store) SetDayOverride(scopeID, userID, day string, override schedule.DayOverride, createdBy, createdAt string) error {
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

// DeleteDayOverride removes one marker, reporting whether it existed.
func (s *Store) DeleteDayOverride(scopeID, userID, day string) (bool, error) {
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
		result[row.UserID][row.Day] = schedule.DayOverride{Kind: row.Kind, SourceDay: row.SourceDay}
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

// ListKV returns every key/value in one namespace.
func (s *Store) ListKV(scope, namespace string) ([]schedule.KVEntry, error) {
	rows, err := s.db.Query(
		`SELECT key, value FROM kv_data WHERE scope = ? AND namespace = ? ORDER BY key`,
		scope, namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("读取 KV 列表失败: %w", err)
	}
	defer rows.Close()
	var entries []schedule.KVEntry
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("读取 KV 行失败: %w", err)
		}
		entries = append(entries, schedule.KVEntry{Key: key, Value: value})
	}
	return entries, rows.Err()
}

// GetKV reads a namespaced value.
func (s *Store) GetKV(scope, namespace, key string, out any) (bool, error) {
	var raw []byte
	err := s.db.QueryRow(
		`SELECT value FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?`,
		scope, namespace, key,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取 KV 失败: %w", err)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return false, fmt.Errorf("解析 KV 失败: %w", err)
		}
	}
	return true, nil
}

// SetKV writes a namespaced value.
func (s *Store) SetKV(scope, namespace, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("编码 KV 失败: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(
		`INSERT INTO kv_data(scope, namespace, key, value, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(scope, namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP`,
		scope, namespace, key, raw,
	)
	if err != nil {
		return fmt.Errorf("写入 KV 失败: %w", err)
	}
	return nil
}

// DeleteKV removes a namespaced value.
func (s *Store) DeleteKV(scope, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`DELETE FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?`,
		scope, namespace, key,
	); err != nil {
		return fmt.Errorf("删除 KV 失败: %w", err)
	}
	return nil
}
