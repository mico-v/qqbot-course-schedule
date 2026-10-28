package schedule_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func TestGlobalModeResolvesCurrentGroupThenUnionThenLocal(t *testing.T) {
	ctx := context.Background()
	handle, err := store.Open(filepath.Join(t.TempDir(), "global.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handle.Close() })
	service := schedule.NewService(handle)

	event, err := schedule.MakeEvent(
		"高数", "2026-10-28 08:00", "2026-10-28 09:30",
		"教一", "", "", "global-event",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		group string
		union string
		name  string
	}{
		{"CURRENT", "UNION-M1", "当前群"},
		{"OTHER", "UNION-OTHER", "同本地 ID 的另一人"},
		{"GLOBAL", "UNION-M1", "正确全局课表"},
	} {
		if _, err := handle.PutGroupMemberSchedule(ctx, schedule.GroupMemberKey{
			GroupOpenID: item.group, MemberOpenID: "M1",
		}, &schedule.Member{
			Name: item.name, ImportUnionOpenID: item.union,
			Events: []schedule.Event{event}, Source: "ics",
		}, 0); err != nil {
			t.Fatalf("PutGroupMemberSchedule %s: %v", item.group, err)
		}
	}
	// Replace the current group's schedule with an empty row. Empty rows must
	// not hide a usable global schedule.
	if _, err := handle.PutGroupMemberSchedule(ctx, schedule.GroupMemberKey{
		GroupOpenID: "CURRENT", MemberOpenID: "M1",
	}, &schedule.Member{
		Name: "当前群", ImportUnionOpenID: "UNION-M1", Source: "manual",
	}, 1); err != nil {
		t.Fatal(err)
	}

	settings := schedule.DefaultSettings()
	settings.StorageMode = schedule.StorageModeGroup
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	if member, found, err := service.VisibleMemberContext(
		ctx, "group:CURRENT", "M1", "UNION-M1",
	); err != nil || !found || len(member.Events) != 0 {
		t.Fatalf("isolated member=%+v found=%v err=%v", member, found, err)
	}

	settings.StorageMode = schedule.StorageModeGlobal
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	member, found, err := service.VisibleMemberContext(
		ctx, "group:CURRENT", "M1", "UNION-M1",
	)
	if err != nil || !found {
		t.Fatalf("global member found=%v err=%v", found, err)
	}
	if member.UserID != "M1" || member.Name != "当前群" || len(member.Events) != 1 {
		t.Fatalf("projected global member = %+v", member)
	}
	if member.Events[0]["SUMMARY"] != "高数" {
		t.Fatalf("event = %+v", member.Events[0])
	}

	currentEvent, _ := schedule.MakeEvent(
		"当前课程", "2026-10-28 14:00", "2026-10-28 15:00",
		"", "", "", "current-event",
	)
	if _, err := handle.PutGroupMemberSchedule(ctx, schedule.GroupMemberKey{
		GroupOpenID: "CURRENT", MemberOpenID: "M1",
	}, &schedule.Member{
		Name: "当前群", ImportUnionOpenID: "UNION-M1",
		Events: []schedule.Event{currentEvent},
	}, 2); err != nil {
		t.Fatal(err)
	}
	member, found, err = service.VisibleMemberContext(
		ctx, "group:CURRENT", "M1", "UNION-M1",
	)
	if err != nil || !found || member.Events[0]["SUMMARY"] != "当前课程" {
		t.Fatalf("current group priority = %+v found=%v err=%v", member, found, err)
	}
}

func TestGlobalModeFallsBackToLocalMemberOpenIDWithoutUnion(t *testing.T) {
	ctx := context.Background()
	handle, err := store.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handle.Close() })
	service := schedule.NewService(handle)
	event, _ := schedule.MakeEvent(
		"英语", "2026-10-28 10:00", "2026-10-28 11:00",
		"", "", "", "local-event",
	)
	if _, err := handle.PutGroupMemberSchedule(ctx, schedule.GroupMemberKey{
		GroupOpenID: "SOURCE", MemberOpenID: "NO-UNION",
	}, &schedule.Member{Name: "无 union", Events: []schedule.Event{event}}, 0); err != nil {
		t.Fatal(err)
	}
	settings := schedule.DefaultSettings()
	settings.StorageMode = schedule.StorageModeGlobal
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	member, found, err := service.VisibleMemberContext(ctx, "group:TARGET", "NO-UNION", "")
	if err != nil || !found || len(member.Events) != 1 {
		t.Fatalf("local fallback member=%+v found=%v err=%v", member, found, err)
	}
}
