package bot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func TestScheduleMembersUsesGlobalModeFallback(t *testing.T) {
	handle, err := store.Open(filepath.Join(t.TempDir(), "global.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handle.Close() })
	service := schedule.NewService(handle)
	settings := schedule.DefaultSettings()
	settings.StorageMode = schedule.StorageModeGlobal
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	event, _ := schedule.MakeEvent(
		"高数", "2026-10-28 08:00", "2026-10-28 09:30",
		"", "", "", "chat-global",
	)
	if _, err := handle.PutGroupMemberSchedule(context.Background(), schedule.GroupMemberKey{
		GroupOpenID: "OTHER", MemberOpenID: "M1",
	}, &schedule.Member{Name: "全局成员", Events: []schedule.Event{event}}, 0); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordSeenMemberContext(
		context.Background(), schedule.ScopeGroup("TARGET"), "M1", "当前群成员",
	); err != nil {
		t.Fatal(err)
	}
	env := &Env{Service: service}
	members, err := env.ScheduleMembers(context.Background(), &Inbound{
		Origin: OriginGroup, GroupOpenID: "TARGET", UserOpenID: "M1",
	})
	if err != nil {
		t.Fatal(err)
	}
	member := members["M1"]
	if member == nil || len(member.Events) != 1 || member.Events[0]["SUMMARY"] != "高数" {
		t.Fatalf("global chat members = %+v", members)
	}
}
