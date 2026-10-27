package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	handle, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { handle.Close() })
	return handle
}

func TestGroupMemberAndScheduleContract(t *testing.T) {
	ctx := context.Background()
	handle := openStore(t)
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for _, group := range []string{"G1", "G2"} {
		if err := handle.RecordGroupMemberSeen(ctx, schedule.GroupMemberObservation{
			Key:         schedule.GroupMemberKey{GroupOpenID: group, MemberOpenID: "M1"},
			DisplayName: "成员-" + group,
			UnionOpenID: "UNION-1",
			ObservedAt:  now,
		}); err != nil {
			t.Fatalf("RecordGroupMemberSeen %s: %v", group, err)
		}
	}
	if err := handle.RecordGroupMemberSeen(ctx, schedule.GroupMemberObservation{
		Key:         schedule.GroupMemberKey{GroupOpenID: "G3", MemberOpenID: "NO-UNION"},
		DisplayName: "无 union 成员", ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	noUnion, found, err := handle.GetGroupMember(ctx, schedule.GroupMemberKey{
		GroupOpenID: "G3", MemberOpenID: "NO-UNION",
	})
	if err != nil || !found || noUnion.UnionOpenID != "" {
		t.Fatalf("member without union = %+v found=%v err=%v", noUnion, found, err)
	}
	first, found, err := handle.GetGroupMember(ctx, schedule.GroupMemberKey{
		GroupOpenID: "G1", MemberOpenID: "M1",
	})
	if err != nil || !found || first.DisplayName != "成员-G1" {
		t.Fatalf("first member = %+v found=%v err=%v", first, found, err)
	}
	second, found, err := handle.GetGroupMember(ctx, schedule.GroupMemberKey{
		GroupOpenID: "G2", MemberOpenID: "M1",
	})
	if err != nil || !found || second.DisplayName != "成员-G2" {
		t.Fatalf("second member = %+v found=%v err=%v", second, found, err)
	}
	if first.Key == second.Key {
		t.Fatal("same member_openid in two groups must not collapse")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := handle.GetGroupMember(canceled, first.Key); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GetGroupMember err = %v", err)
	}

	event, err := schedule.MakeEvent(
		"高数", "2026-09-02 08:00", "2026-09-02 09:30",
		"教一", "", "FREQ=WEEKLY;BYDAY=WE", "e1",
	)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := handle.PutGroupMemberSchedule(ctx, first.Key, &schedule.Member{
		Name: first.DisplayName, QQ: "123456", Events: []schedule.Event{event},
		Source: "ics",
	}, 0)
	if err != nil || revision != 1 {
		t.Fatalf("create revision=%d err=%v", revision, err)
	}
	if _, err := handle.PutGroupMemberSchedule(ctx, first.Key, &schedule.Member{
		Name: first.DisplayName,
	}, 0); !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("create-only overwrite = %v, want conflict", err)
	}
	members, err := handle.ListGroupSchedules(ctx, schedule.GroupKey{GroupOpenID: "G1"})
	if err != nil || len(members) != 1 || len(members["M1"].Events) != 1 {
		t.Fatalf("ListGroupSchedules = %+v err=%v", members, err)
	}
}

func TestOverridesSettingsLinksPanelsAndStatsContract(t *testing.T) {
	ctx := context.Background()
	handle := openStore(t)
	group := schedule.GroupKey{GroupOpenID: "G1"}
	key := schedule.GroupMemberKey{GroupOpenID: "G1", MemberOpenID: "M1"}
	if err := handle.RecordGroupMemberSeen(ctx, schedule.GroupMemberObservation{
		Key: key, DisplayName: "小明", ObservedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := handle.SetGroupDayOverride(ctx, schedule.DayOverrideRow{
		ScopeID: group.ScopeID(), UserID: schedule.DayOverrideAll,
		Day: "2026-10-01", Kind: schedule.DayOverrideHoliday,
		CreatedBy: "admin", CreatedAt: schedule.NowISO(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := handle.SetGroupDayOverride(ctx, schedule.DayOverrideRow{
		ScopeID: group.ScopeID(), UserID: "M1",
		Day: "2026-10-01", Kind: schedule.DayOverrideShift,
		SourceDay: "2026-10-08", CreatedBy: "admin", CreatedAt: schedule.NowISO(),
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := handle.ListGroupDayOverrides(ctx, group)
	if err != nil || len(rows) != 2 {
		t.Fatalf("overrides = %+v err=%v", rows, err)
	}
	removed, err := handle.DeleteGroupDayOverride(ctx, group, "M1", "2026-10-01")
	if err != nil || !removed {
		t.Fatalf("delete override = %v/%v", removed, err)
	}

	settings := schedule.DefaultSettings()
	settings.Enabled = false
	if err := handle.SaveBotSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := handle.LoadBotSettings(ctx)
	if err != nil || !found || loaded.Enabled {
		t.Fatalf("settings = %+v found=%v err=%v", loaded, found, err)
	}

	link := schedule.EditLinkRecord{
		TokenHash:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GroupOpenID: "G1", MemberOpenID: "M1",
		ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
	}
	if err := handle.SaveEditLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	gotLink, found, err := handle.GetEditLink(ctx, link.TokenHash)
	if err != nil || !found || gotLink.MemberOpenID != "M1" {
		t.Fatalf("edit link = %+v found=%v err=%v", gotLink, found, err)
	}
	if err := handle.DeleteEditLink(ctx, link.TokenHash); err != nil {
		t.Fatal(err)
	}

	panel := schedule.PanelState{PanelID: "P1", ItemsHash: "HASH", Version: 2}
	if err := handle.SetPanelState(ctx, "group", panel); err != nil {
		t.Fatal(err)
	}
	gotPanel, found, err := handle.GetPanelState(ctx, "group")
	if err != nil || !found || gotPanel.PanelID != "P1" {
		t.Fatalf("panel = %+v found=%v err=%v", gotPanel, found, err)
	}

	record := schedule.MessageStats{
		ScopeID: "group:G1", UserID: "M1", Origin: "group",
		Command: "/课表", Stage: schedule.StatsStageReply,
		ReceivedAt: time.Now().UTC(), SendMS: 5, OK: true,
	}
	if err := handle.InsertMessageStats(ctx, record); err != nil {
		t.Fatal(err)
	}
	records, err := handle.ListMessageStats(ctx, time.Now().Add(-time.Hour), &group)
	if err != nil || len(records) != 1 || records[0].Command != "/课表" {
		t.Fatalf("stats = %+v err=%v", records, err)
	}
	removedStats, err := handle.PruneMessageStats(ctx, time.Now().Add(time.Hour))
	if err != nil || removedStats != 1 {
		t.Fatalf("prune stats = %d/%v", removedStats, err)
	}
}
