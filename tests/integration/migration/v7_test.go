package migration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func TestV6ToV7PreservesGroupIdentityAndEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	seedV6Database(t, path)

	handle, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	db := openSQLite(t, path)
	defer db.Close()

	assertCount(t, db, `SELECT COUNT(*) FROM group_members`, 3)
	assertCount(t, db, `SELECT COUNT(*) FROM group_member_schedules`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM group_member_course_events`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM group_member_schedules
		WHERE member_openid = 'SAME-MEMBER'`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM schedule_members
		WHERE scope_id = 'private:PRIVATE-1'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM schedule_edit_links`, 1)

	var displayName, unionOpenID string
	if err := db.QueryRow(
		`SELECT display_name, union_openid FROM group_members
		 WHERE group_openid = 'GROUP-1' AND member_openid = 'SAME-MEMBER'`,
	).Scan(&displayName, &unionOpenID); err != nil {
		t.Fatal(err)
	}
	if displayName != "成员一" || unionOpenID != "UNION-1" {
		t.Fatalf("member profile = %q/%q", displayName, unionOpenID)
	}

	var settingsJSON string
	if err := db.QueryRow(
		`SELECT settings_json FROM bot_settings WHERE id = 1`,
	).Scan(&settingsJSON); err != nil {
		t.Fatal(err)
	}

	first, found, err := handle.GetGroupMemberSchedule(context.Background(), schedule.GroupMemberKey{
		GroupOpenID: "GROUP-1", MemberOpenID: "SAME-MEMBER",
	})
	if err != nil || !found {
		t.Fatalf("GetGroupMemberSchedule found=%v err=%v", found, err)
	}
	second, found, err := handle.GetGroupMemberSchedule(context.Background(), schedule.GroupMemberKey{
		GroupOpenID: "GROUP-2", MemberOpenID: "SAME-MEMBER",
	})
	if err != nil || !found {
		t.Fatalf("GetGroupMemberSchedule second found=%v err=%v", found, err)
	}
	if first.Name == second.Name || len(first.Events) != 1 || len(second.Events) != 1 {
		t.Fatalf("cross-group rows were merged: first=%+v second=%+v", first, second)
	}

	backups, err := filepath.Glob(path + ".v6.*.bak")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("v6 backups = %v, want one", backups)
	}
}

func TestV7ForeignKeysCascadeAndRevisionCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v7.db")
	handle, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { handle.Close() })

	key := schedule.GroupMemberKey{GroupOpenID: "G1", MemberOpenID: "M1"}
	event, err := schedule.MakeEvent(
		"高数", "2026-09-01 08:00", "2026-09-01 09:30",
		"教一", "", "", "event-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := handle.PutGroupMemberSchedule(context.Background(), key, &schedule.Member{
		Name: "小明", Source: "test", Events: []schedule.Event{event},
	}, 0)
	if err != nil || revision != 1 {
		t.Fatalf("create revision=%d err=%v", revision, err)
	}
	if _, err := handle.PutGroupMemberSchedule(context.Background(), key, &schedule.Member{
		Name: "小明",
	}, 0); !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("create-only overwrite err=%v, want conflict", err)
	}
	if _, err := handle.PutGroupMemberSchedule(context.Background(), key, &schedule.Member{
		Name: "小明",
	}, 2); !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("stale update err=%v, want conflict", err)
	}

	db := openSQLite(t, path)
	defer db.Close()
	if _, err := db.Exec(
		`DELETE FROM group_members WHERE group_openid = 'G1' AND member_openid = 'M1'`,
	); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM group_member_schedules`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM group_member_course_events`, 0)
}

func TestV7MigrationRollsBackOnBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.db")
	seedBaseV6(t, path)
	db := openSQLite(t, path)
	if _, err := db.Exec(
		`INSERT INTO schedule_members(scope_id, user_id, data_json, updated_at, revision)
		 VALUES ('group:G1', 'M1', '{bad json', '2026-01-01T00:00:00Z', 1)`,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Open(path); err == nil {
		t.Fatal("Open should reject malformed v6 member JSON")
	}
	db = openSQLite(t, path)
	defer db.Close()
	var version string
	if err := db.QueryRow(
		`SELECT value FROM metadata WHERE key = 'schema_version'`,
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "6" {
		t.Fatalf("schema version after rollback = %q, want 6", version)
	}
	var table string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'group_members'`,
	).Scan(&table)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("partial v7 table survived rollback: table=%q err=%v", table, err)
	}
}

func seedV6Database(t *testing.T, path string) {
	t.Helper()
	seedBaseV6(t, path)
	db := openSQLite(t, path)
	defer db.Close()
	rows := []struct {
		group, member, name, union string
	}{
		{"GROUP-1", "SAME-MEMBER", "成员一", "UNION-1"},
		{"GROUP-2", "SAME-MEMBER", "成员二", "UNION-1"},
	}
	for index, row := range rows {
		data := `{"name":"` + row.name + `","qq":"","schedule":"","source":"ics",` +
			`"import_group_openid":"` + row.group + `","import_user_openid":"` + row.member + `",` +
			`"import_union_openid":"` + row.union + `","event_count":1,` +
			`"updated_at":"2026-09-01T00:00:00Z","schedule_updated_at":"2026-09-01T00:00:00Z",` +
			`"ics":"BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"}`
		if _, err := db.Exec(
			`INSERT INTO schedule_members(scope_id, user_id, data_json, updated_at, revision)
			 VALUES (?, ?, ?, '2026-09-01T00:00:00Z', 3)`,
			schedule.ScopeGroup(row.group), row.member, data,
		); err != nil {
			t.Fatal(err)
		}
		event := schedule.Event{"UID": row.member, "SUMMARY": row.name}
		encoded := mustJSON(t, event)
		if _, err := db.Exec(
			`INSERT INTO course_events(
				scope_id, user_id, event_index, uid, summary, event_json
			 ) VALUES (?, ?, 1, ?, ?, ?)`,
			schedule.ScopeGroup(row.group), row.member, row.member, row.name, encoded,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			`INSERT INTO schedule_day_overrides(
				scope_id, user_id, day, kind, source_day, created_by, created_at
			 ) VALUES (?, ?, '2026-10-01', 'holiday', '', 'admin', ?)`,
			schedule.ScopeGroup(row.group), row.member, "2026-09-01T00:00:00Z",
		); err != nil {
			t.Fatal(err)
		}
		_ = index
	}
	if _, err := db.Exec(
		`INSERT INTO schedule_members(scope_id, user_id, data_json, updated_at, revision)
		 VALUES ('private:PRIVATE-1', 'PRIVATE-1', '{}', '2026-09-01T00:00:00Z', 1)`,
	); err != nil {
		t.Fatal(err)
	}
	kv := []struct {
		namespace, key, value string
	}{
		{schedule.KVNamespaceSeen, "group:GROUP-1", `{"SEEN-ONLY":"只发言"}`},
		{schedule.KVNamespaceRoster, "group:GROUP-1", `{"members":[{"user_id":"SAME-MEMBER","name":"成员一","union_openid":"UNION-1"}],"updated_at":"2026-09-02T00:00:00Z"}`},
		{schedule.KVNamespaceSettings, "bot", `{"enabled":false,"reply_plain":true}`},
		{schedule.KVNamespacePanel, "group", `{"panel_id":"P1","items_hash":"HASH","version":2}`},
		{schedule.KVNamespaceEditLinks, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", `{"scope_id":"group:GROUP-1","user_id":"SAME-MEMBER","expires_at":"2026-12-01T00:00:00Z"}`},
	}
	for _, entry := range kv {
		if _, err := db.Exec(
			`INSERT INTO kv_data(scope, namespace, key, value) VALUES ('global', ?, ?, ?)`,
			entry.namespace, entry.key, entry.value,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func seedBaseV6(t *testing.T, path string) {
	t.Helper()
	db := openSQLite(t, path)
	defer db.Close()
	statements := []string{
		`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO metadata(key, value) VALUES ('schema_version', '6')`,
		`CREATE TABLE schedule_members (
			scope_id TEXT NOT NULL, user_id TEXT NOT NULL, data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY(scope_id, user_id)
		)`,
		`CREATE TABLE course_events (
			scope_id TEXT NOT NULL, user_id TEXT NOT NULL, event_index INTEGER NOT NULL,
			uid TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
			location TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
			dtstart TEXT NOT NULL DEFAULT '', dtend TEXT NOT NULL DEFAULT '',
			dtstart_tzid TEXT NOT NULL DEFAULT '', dtend_tzid TEXT NOT NULL DEFAULT '',
			rrule TEXT NOT NULL DEFAULT '', dtstamp TEXT NOT NULL DEFAULT '',
			event_json TEXT NOT NULL,
			PRIMARY KEY(scope_id, user_id, event_index)
		)`,
		`CREATE TABLE schedule_day_overrides (
			scope_id TEXT NOT NULL, user_id TEXT NOT NULL, day TEXT NOT NULL,
			kind TEXT NOT NULL, source_day TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY(scope_id, user_id, day)
		)`,
		`CREATE TABLE kv_data (
			scope TEXT NOT NULL, namespace TEXT NOT NULL, key TEXT NOT NULL,
			value BLOB NOT NULL, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(scope, namespace, key)
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed v6: %v", err)
		}
	}
}

func openSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s = %d, want %d", query, got, want)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
