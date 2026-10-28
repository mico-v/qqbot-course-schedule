package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

const schemaVersion = 7

var schemaStatements = []string{
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
	`CREATE INDEX IF NOT EXISTS idx_schedule_members_user ON schedule_members(user_id)`,
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
	`CREATE TABLE IF NOT EXISTS message_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scope_id TEXT NOT NULL DEFAULT '',
		origin TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL DEFAULT '',
		user_id TEXT NOT NULL DEFAULT '',
		stage TEXT NOT NULL DEFAULT '',
		received_at TEXT NOT NULL DEFAULT '',
		render_ms INTEGER NOT NULL DEFAULT 0,
		upload_ms INTEGER NOT NULL DEFAULT 0,
		send_ms INTEGER NOT NULL DEFAULT 0,
		server_ms INTEGER NOT NULL DEFAULT 0,
		ok INTEGER NOT NULL DEFAULT 1,
		err_code TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_message_stats_received ON message_stats(received_at)`,
	`CREATE TABLE IF NOT EXISTS kv_data (
		scope TEXT NOT NULL,
		namespace TEXT NOT NULL,
		key TEXT NOT NULL,
		value BLOB NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (scope, namespace, key)
	)`,
	`CREATE TABLE IF NOT EXISTS groups (
		group_openid TEXT PRIMARY KEY CHECK (group_openid <> ''),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS group_members (
		group_openid TEXT NOT NULL,
		member_openid TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		qq TEXT NOT NULL DEFAULT '',
		union_openid TEXT NULL,
		first_seen_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL,
		last_roster_at TEXT NULL,
		is_in_roster INTEGER NOT NULL DEFAULT 0 CHECK (is_in_roster IN (0, 1)),
		PRIMARY KEY (group_openid, member_openid),
		FOREIGN KEY (group_openid) REFERENCES groups(group_openid) ON DELETE CASCADE,
		CHECK (group_openid <> '' AND member_openid <> '')
	)`,
	`CREATE INDEX IF NOT EXISTS idx_group_members_union
		ON group_members(union_openid) WHERE union_openid IS NOT NULL`,
	`CREATE TABLE IF NOT EXISTS group_member_schedules (
		group_openid TEXT NOT NULL,
		member_openid TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT '',
		source_file TEXT NOT NULL DEFAULT '',
		uploader_openid TEXT NOT NULL DEFAULT '',
		import_group_openid TEXT NOT NULL DEFAULT '',
		import_member_openid TEXT NOT NULL DEFAULT '',
		import_union_openid TEXT NOT NULL DEFAULT '',
		ics TEXT NOT NULL DEFAULT '',
		schedule_text TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		schedule_updated_at TEXT NOT NULL,
		last_modified_at TEXT NOT NULL,
		last_modified_by TEXT NOT NULL DEFAULT '',
		revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
		PRIMARY KEY (group_openid, member_openid),
		FOREIGN KEY (group_openid, member_openid)
			REFERENCES group_members(group_openid, member_openid) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_group_member_schedules_updated
		ON group_member_schedules(group_openid, updated_at)`,
	`CREATE INDEX IF NOT EXISTS idx_group_member_schedules_member
		ON group_member_schedules(member_openid, schedule_updated_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_group_member_schedules_import_union
		ON group_member_schedules(import_union_openid)
		WHERE import_union_openid <> ''`,
	`CREATE TABLE IF NOT EXISTS group_member_course_events (
		group_openid TEXT NOT NULL,
		member_openid TEXT NOT NULL,
		event_index INTEGER NOT NULL CHECK (event_index >= 0),
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
		raw_json TEXT NOT NULL DEFAULT '{}',
		PRIMARY KEY (group_openid, member_openid, event_index),
		FOREIGN KEY (group_openid, member_openid)
			REFERENCES group_member_schedules(group_openid, member_openid) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_group_member_events_start
		ON group_member_course_events(group_openid, dtstart)`,
	`CREATE TABLE IF NOT EXISTS group_day_overrides (
		group_openid TEXT NOT NULL,
		member_openid TEXT NULL,
		day TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('holiday', 'shift')),
		source_day TEXT NOT NULL DEFAULT '',
		actor_openid TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		FOREIGN KEY (group_openid) REFERENCES groups(group_openid) ON DELETE CASCADE,
		FOREIGN KEY (group_openid, member_openid)
			REFERENCES group_members(group_openid, member_openid) ON DELETE CASCADE,
		CHECK (member_openid IS NULL OR member_openid <> '')
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS ux_group_day_overrides_all
		ON group_day_overrides(group_openid, day) WHERE member_openid IS NULL`,
	`CREATE UNIQUE INDEX IF NOT EXISTS ux_group_day_overrides_member
		ON group_day_overrides(group_openid, member_openid, day) WHERE member_openid IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_group_day_overrides_group_day
		ON group_day_overrides(group_openid, day)`,
	`CREATE TABLE IF NOT EXISTS bot_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		settings_json TEXT NOT NULL CHECK (json_valid(settings_json)),
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS schedule_edit_links (
		token_hash TEXT PRIMARY KEY CHECK (length(token_hash) = 64),
		group_openid TEXT NOT NULL,
		member_openid TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (group_openid, member_openid)
			REFERENCES group_members(group_openid, member_openid) ON DELETE CASCADE,
		CHECK (group_openid <> '' AND member_openid <> '')
	)`,
	`CREATE INDEX IF NOT EXISTS idx_schedule_edit_links_expires
		ON schedule_edit_links(expires_at)`,
	`CREATE TABLE IF NOT EXISTS bot_panels (
		panel_scope TEXT PRIMARY KEY CHECK (panel_scope <> ''),
		panel_id TEXT NOT NULL,
		items_hash TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
		updated_at TEXT NOT NULL
	)`,
}

func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开启数据库迁移事务失败: %w", err)
	}
	defer tx.Rollback()

	for _, statement := range schemaStatements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("初始化数据库失败: %w", err)
		}
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS checkin_records`); err != nil {
		return fmt.Errorf("删除旧签到表失败: %w", err)
	}
	if err := ensureMessageStatsIdentityColumns(tx); err != nil {
		return err
	}
	if err := migrateLegacyGroupData(tx); err != nil {
		return err
	}
	if err := migrateNamedState(tx); err != nil {
		return err
	}
	if err := checkForeignKeys(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO metadata(key, value) VALUES ('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		fmt.Sprint(schemaVersion),
	); err != nil {
		return fmt.Errorf("写入 schema_version 失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交数据库迁移失败: %w", err)
	}
	return nil
}

func ensureMessageStatsIdentityColumns(tx *sql.Tx) error {
	columns, err := tableColumns(tx, "message_stats")
	if err != nil {
		return err
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "group_openid", definition: "TEXT NULL"},
		{name: "member_openid", definition: "TEXT NULL"},
	} {
		if columns[column.name] {
			continue
		}
		if _, err := tx.Exec("ALTER TABLE message_stats ADD COLUMN " + column.name + " " + column.definition); err != nil {
			return fmt.Errorf("升级 message_stats.%s 失败: %w", column.name, err)
		}
	}
	if _, err := tx.Exec(
		`UPDATE message_stats
		 SET group_openid = substr(scope_id, 7),
		     member_openid = CASE
		       WHEN user_id <> '' THEN user_id
		       ELSE member_openid
		     END
		 WHERE group_openid IS NULL AND scope_id LIKE 'group:%'`,
	); err != nil {
		return fmt.Errorf("回填 message_stats 群坐标失败: %w", err)
	}
	return nil
}

func tableColumns(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, fmt.Errorf("读取 %s 表结构失败: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("读取 %s 表结构失败: %w", table, err)
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func migrateLegacyGroupData(tx *sql.Tx) error {
	done, err := metadataFlag(tx, "v7_group_migration_done")
	if err != nil || done {
		return err
	}
	type legacyRow struct {
		scopeID  string
		userID   string
		dataJSON string
		revision int64
	}
	rows, err := tx.Query(
		`SELECT scope_id, user_id, data_json, revision
		 FROM schedule_members WHERE scope_id LIKE 'group:%'
		 ORDER BY scope_id, user_id`,
	)
	if err != nil {
		return fmt.Errorf("读取 v6 群成员失败: %w", err)
	}
	var members []legacyRow
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.scopeID, &row.userID, &row.dataJSON, &row.revision); err != nil {
			rows.Close()
			return fmt.Errorf("读取 v6 群成员失败: %w", err)
		}
		members = append(members, row)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("读取 v6 群成员失败: %w", err)
	}
	for _, row := range members {
		ref, err := schedule.ParseLegacyScopeRef(row.scopeID)
		if err != nil || ref.Kind != "group" {
			return fmt.Errorf("迁移群成员 %s/%s 失败: 无效 scope", row.scopeID, row.userID)
		}
		var meta memberMeta
		if err := json.Unmarshal([]byte(row.dataJSON), &meta); err != nil {
			return fmt.Errorf("迁移群成员 %s/%s 失败: 解析 data_json: %w", row.scopeID, row.userID, err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		updatedAt := firstNonEmpty(meta.UpdatedAt, meta.LastModifiedAt, now)
		if _, err := tx.Exec(
			`INSERT INTO groups(group_openid, created_at, updated_at)
			 VALUES (?, ?, ?)
			 ON CONFLICT(group_openid) DO UPDATE SET updated_at = excluded.updated_at`,
			ref.ID, firstNonEmpty(meta.UpdatedAt, now), updatedAt,
		); err != nil {
			return fmt.Errorf("迁移群 %s 失败: %w", ref.ID, err)
		}
		displayName := firstNonEmpty(meta.Name, row.userID)
		unionOpenID := nullableString(firstNonEmpty(meta.ImportUnionOpenID))
		if _, err := tx.Exec(
			`INSERT INTO group_members(
				group_openid, member_openid, display_name, qq, union_openid,
				first_seen_at, last_seen_at, last_roster_at, is_in_roster
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, 0)
			 ON CONFLICT(group_openid, member_openid) DO NOTHING`,
			ref.ID, row.userID, displayName, meta.QQ, unionOpenID, updatedAt, updatedAt,
		); err != nil {
			return fmt.Errorf("迁移群成员 %s/%s 失败: %w", ref.ID, row.userID, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO group_member_schedules(
				group_openid, member_openid, source, source_file, uploader_openid,
				import_group_openid, import_member_openid, import_union_openid,
				ics, schedule_text, created_at, updated_at, schedule_updated_at,
				last_modified_at, last_modified_by, revision
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(group_openid, member_openid) DO NOTHING`,
			ref.ID, row.userID, meta.Source, meta.SourceFile, meta.UploaderID,
			firstNonEmpty(meta.ImportGroupOpenID, ref.ID),
			firstNonEmpty(meta.ImportUserOpenID, row.userID),
			meta.ImportUnionOpenID, meta.ICS, meta.Schedule,
			firstNonEmpty(meta.UpdatedAt, now), firstNonEmpty(meta.UpdatedAt, now),
			firstNonEmpty(meta.ScheduleUpdatedAt, meta.UpdatedAt, now),
			firstNonEmpty(meta.LastModifiedAt, meta.UpdatedAt, now),
			meta.LastModifiedBy, max(row.revision, 1),
		); err != nil {
			return fmt.Errorf("迁移群课表 %s/%s 失败: %w", ref.ID, row.userID, err)
		}
		if err := migrateLegacyEvents(tx, ref.ID, row.userID); err != nil {
			return err
		}
	}
	if err := migrateLegacyOverrides(tx); err != nil {
		return err
	}
	if err := migrateLegacyMemberState(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO metadata(key, value) VALUES ('v7_group_migration_done', '1')
		 ON CONFLICT(key) DO UPDATE SET value = '1'`,
	); err != nil {
		return fmt.Errorf("记录 v7 群迁移状态失败: %w", err)
	}
	return nil
}

func migrateLegacyEvents(tx *sql.Tx, groupOpenID, memberOpenID string) error {
	rows, err := tx.Query(
		`SELECT event_index, uid, summary, location, description, dtstart, dtend,
		        dtstart_tzid, dtend_tzid, rrule, dtstamp, event_json
		 FROM course_events WHERE scope_id = ? AND user_id = ?
		 ORDER BY event_index`,
		"group:"+groupOpenID, memberOpenID,
	)
	if err != nil {
		return fmt.Errorf("读取 v6 群事件失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		var uid, summary, location, description, dtstart, dtend string
		var startTZID, endTZID, rrule, dtstamp, rawJSON string
		if err := rows.Scan(
			&index, &uid, &summary, &location, &description, &dtstart, &dtend,
			&startTZID, &endTZID, &rrule, &dtstamp, &rawJSON,
		); err != nil {
			return fmt.Errorf("读取 v6 群事件失败: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO group_member_course_events(
				group_openid, member_openid, event_index, uid, summary, location,
				description, dtstart, dtend, dtstart_tzid, dtend_tzid, rrule,
				dtstamp, raw_json
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(group_openid, member_openid, event_index) DO NOTHING`,
			groupOpenID, memberOpenID, max(index-1, 0), uid, summary, location,
			description, dtstart, dtend, startTZID, endTZID, rrule, dtstamp, rawJSON,
		); err != nil {
			return fmt.Errorf("迁移群事件 %s/%s 失败: %w", groupOpenID, memberOpenID, err)
		}
	}
	return rows.Err()
}

func migrateLegacyOverrides(tx *sql.Tx) error {
	rows, err := tx.Query(
		`SELECT scope_id, user_id, day, kind, source_day, created_by, created_at
		 FROM schedule_day_overrides WHERE scope_id LIKE 'group:%'
		 ORDER BY scope_id, day, user_id`,
	)
	if err != nil {
		return fmt.Errorf("读取 v6 群标记失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scopeID, userID, day, kind, sourceDay, actor, createdAt string
		if err := rows.Scan(&scopeID, &userID, &day, &kind, &sourceDay, &actor, &createdAt); err != nil {
			return fmt.Errorf("读取 v6 群标记失败: %w", err)
		}
		ref, err := schedule.ParseLegacyScopeRef(scopeID)
		if err != nil || ref.Kind != "group" {
			return fmt.Errorf("迁移群标记 %s/%s 失败: 无效 scope", scopeID, userID)
		}
		var member any
		if userID != schedule.DayOverrideAll {
			member = userID
			if err := upsertMigratedMember(tx, ref.ID, userID, "", ""); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO groups(group_openid, created_at, updated_at)
			 VALUES (?, ?, ?)
			 ON CONFLICT(group_openid) DO NOTHING`,
			ref.ID, firstNonEmpty(createdAt, time.Now().UTC().Format(time.RFC3339Nano)),
			firstNonEmpty(createdAt, time.Now().UTC().Format(time.RFC3339Nano)),
		); err != nil {
			return fmt.Errorf("迁移群标记所属群 %s 失败: %w", ref.ID, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO group_day_overrides(
				group_openid, member_openid, day, kind, source_day, actor_openid, created_at
			 ) VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT DO NOTHING`,
			ref.ID, member, day, kind, sourceDay, actor, createdAt,
		); err != nil {
			return fmt.Errorf("迁移群标记 %s/%s 失败: %w", ref.ID, userID, err)
		}
	}
	return rows.Err()
}

func checkForeignKeys(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("检查外键失败: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID int64
		var parent string
		var constraint int
		if err := rows.Scan(&table, &rowID, &parent, &constraint); err != nil {
			return fmt.Errorf("读取外键检查结果失败: %w", err)
		}
		return fmt.Errorf("数据库迁移外键校验失败: table=%s rowid=%d parent=%s", table, rowID, parent)
	}
	return rows.Err()
}

func migrateLegacyMemberState(tx *sql.Tx) error {
	state, err := loadLegacyKVMap(tx, schedule.KVNamespaceSeen)
	if err != nil {
		return err
	}
	identities, err := loadLegacyKVMap(tx, schedule.KVNamespaceIdentity)
	if err != nil {
		return err
	}
	rosters, err := loadLegacyKVMap(tx, schedule.KVNamespaceRoster)
	if err != nil {
		return err
	}
	for scopeID, raw := range state {
		ref, err := schedule.ParseLegacyScopeRef(scopeID)
		if err != nil || ref.Kind != "group" {
			continue
		}
		var seen map[string]string
		if err := json.Unmarshal(raw, &seen); err != nil {
			return fmt.Errorf("迁移 seen %s 失败: %w", scopeID, err)
		}
		for memberID, name := range seen {
			if err := upsertMigratedMember(tx, ref.ID, memberID, name, ""); err != nil {
				return err
			}
		}
	}
	for scopeID, raw := range identities {
		ref, err := schedule.ParseLegacyScopeRef(scopeID)
		if err != nil || ref.Kind != "group" {
			continue
		}
		var values map[string]string
		if err := json.Unmarshal(raw, &values); err != nil {
			return fmt.Errorf("迁移 identity %s 失败: %w", scopeID, err)
		}
		for memberID, unionOpenID := range values {
			if err := upsertMigratedMember(tx, ref.ID, memberID, "", unionOpenID); err != nil {
				return err
			}
		}
	}
	for scopeID, raw := range rosters {
		ref, err := schedule.ParseLegacyScopeRef(scopeID)
		if err != nil || ref.Kind != "group" {
			continue
		}
		var roster schedule.GroupRoster
		if err := json.Unmarshal(raw, &roster); err != nil {
			return fmt.Errorf("迁移 roster %s 失败: %w", scopeID, err)
		}
		for _, member := range roster.Members {
			if err := upsertMigratedMember(tx, ref.ID, member.UserID, member.Name, member.UnionOpenID); err != nil {
				return err
			}
			if _, err := tx.Exec(
				`UPDATE group_members SET is_in_roster = 1, last_roster_at = ?
				 WHERE group_openid = ? AND member_openid = ?`,
				firstNonEmpty(roster.UpdatedAt, time.Now().UTC().Format(time.RFC3339Nano)),
				ref.ID, member.UserID,
			); err != nil {
				return fmt.Errorf("更新 roster 成员 %s/%s 失败: %w", ref.ID, member.UserID, err)
			}
		}
	}
	return nil
}

func loadLegacyKVMap(tx *sql.Tx, namespace string) (map[string][]byte, error) {
	rows, err := tx.Query(
		`SELECT key, value FROM kv_data WHERE scope = ? AND namespace = ?`,
		schedule.KVScopeGlobal, namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("读取旧 KV %s 失败: %w", namespace, err)
	}
	defer rows.Close()
	result := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("读取旧 KV %s 失败: %w", namespace, err)
		}
		result[key] = value
	}
	return result, rows.Err()
}

func upsertMigratedMember(tx *sql.Tx, groupOpenID, memberOpenID, displayName, unionOpenID string) error {
	memberOpenID = strings.TrimSpace(memberOpenID)
	if memberOpenID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(
		`INSERT INTO groups(group_openid, created_at, updated_at)
		 VALUES (?, ?, ?) ON CONFLICT(group_openid) DO NOTHING`,
		groupOpenID, now, now,
	); err != nil {
		return fmt.Errorf("迁移群 %s 失败: %w", groupOpenID, err)
	}
	displayName = strings.TrimSpace(displayName)
	insertName := displayName
	if insertName == "" {
		insertName = memberOpenID
	}
	if _, err := tx.Exec(
		`INSERT INTO group_members(
			group_openid, member_openid, display_name, union_openid,
			first_seen_at, last_seen_at
		 ) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(group_openid, member_openid) DO UPDATE SET
			display_name = CASE
				WHEN ? = 1 THEN excluded.display_name
				ELSE group_members.display_name
			END,
			union_openid = CASE
				WHEN excluded.union_openid IS NOT NULL AND excluded.union_openid <> ''
				THEN excluded.union_openid
				ELSE group_members.union_openid
			END,
			last_seen_at = excluded.last_seen_at`,
		groupOpenID, memberOpenID, insertName,
		nullableString(strings.TrimSpace(unionOpenID)), now, now,
		boolToInt(displayName != ""),
	); err != nil {
		return fmt.Errorf("迁移群成员 %s/%s 失败: %w", groupOpenID, memberOpenID, err)
	}
	return nil
}

func migrateNamedState(tx *sql.Tx) error {
	if err := migrateSettings(tx); err != nil {
		return err
	}
	if err := migrateEditLinks(tx); err != nil {
		return err
	}
	if err := migratePanels(tx); err != nil {
		return err
	}
	return nil
}

func migrateEditLinks(tx *sql.Tx) error {
	rows, err := tx.Query(
		`SELECT key, value FROM kv_data WHERE scope = ? AND namespace = ?`,
		schedule.KVScopeGlobal, schedule.KVNamespaceEditLinks,
	)
	if err != nil {
		return fmt.Errorf("读取旧编辑链接失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tokenHash string
		var raw []byte
		if err := rows.Scan(&tokenHash, &raw); err != nil {
			return fmt.Errorf("读取旧编辑链接失败: %w", err)
		}
		var record struct {
			ScopeID   string `json:"scope_id"`
			UserID    string `json:"user_id"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			return fmt.Errorf("解析旧编辑链接 %s 失败: %w", tokenHash, err)
		}
		ref, err := schedule.ParseLegacyScopeRef(record.ScopeID)
		if err != nil || ref.Kind != "group" {
			continue
		}
		if err := upsertMigratedMember(tx, ref.ID, record.UserID, "", ""); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO schedule_edit_links(
				token_hash, group_openid, member_openid, expires_at, created_at
			 ) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(token_hash) DO NOTHING`,
			tokenHash, ref.ID, record.UserID, record.ExpiresAt,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("迁移编辑链接 %s 失败: %w", tokenHash, err)
		}
	}
	return rows.Err()
}

func migrateSettings(tx *sql.Tx) error {
	var raw []byte
	err := tx.QueryRow(
		`SELECT value FROM kv_data WHERE scope = ? AND namespace = ? AND key = 'bot'`,
		schedule.KVScopeGlobal, schedule.KVNamespaceSettings,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取旧设置失败: %w", err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("解析旧设置失败: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO bot_settings(id, settings_json, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("迁移机器人设置失败: %w", err)
	}
	return nil
}

func migratePanels(tx *sql.Tx) error {
	rows, err := tx.Query(
		`SELECT key, value FROM kv_data WHERE scope = ? AND namespace = ?`,
		schedule.KVScopeGlobal, schedule.KVNamespacePanel,
	)
	if err != nil {
		return fmt.Errorf("读取旧面板状态失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scope string
		var raw []byte
		if err := rows.Scan(&scope, &raw); err != nil {
			return fmt.Errorf("读取旧面板状态失败: %w", err)
		}
		var state schedule.PanelState
		if err := json.Unmarshal(raw, &state); err != nil {
			return fmt.Errorf("解析旧面板状态 %s 失败: %w", scope, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO bot_panels(panel_scope, panel_id, items_hash, version, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(panel_scope) DO NOTHING`,
			scope, state.PanelID, state.ItemsHash, max(state.Version, 1),
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("迁移面板状态 %s 失败: %w", scope, err)
		}
	}
	return rows.Err()
}

func metadataFlag(tx *sql.Tx, key string) (bool, error) {
	var value string
	err := tx.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取迁移状态 %s 失败: %w", key, err)
	}
	return value == "1", nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
