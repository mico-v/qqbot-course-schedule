package admin_test

import (
	"context"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestGlobalModePageAndDeleteResolvedSchedule(t *testing.T) {
	service, handle := newService(t)
	settings, err := service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.StorageMode = schedule.StorageModeGlobal
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	source := schedule.ScopeGroup("SOURCE")
	target := schedule.ScopeGroup("TARGET")
	if _, err := service.SaveICS(source, "M1", "全局课表", adminICS, "global.ics", "M1"); err != nil {
		t.Fatalf("SaveICS source: %v", err)
	}
	zero := int64(0)
	if err := handle.PutMember(target, "M1", &schedule.Member{
		UserID: "M1", Name: "当前群成员", Source: "manual",
	}, &zero); err != nil {
		t.Fatalf("create empty target: %v", err)
	}

	page, found, err := service.PageSchedule(target, "M1")
	if err != nil || !found {
		t.Fatalf("PageSchedule found=%v err=%v", found, err)
	}
	if len(page.Events) != 1 || page.Events[0].Course != "高等数学" {
		t.Fatalf("global page = %+v", page)
	}
	pending, err := service.PendingMembers(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("globally resolved member should not be pending: %+v", pending)
	}

	deleted, err := service.DeleteMemberScheduleContext(
		context.Background(), target, "M1", page.Revision,
	)
	if err != nil || !deleted {
		t.Fatalf("delete = %v/%v", deleted, err)
	}
	if _, found, err := handle.GetGroupMemberSchedule(context.Background(), schedule.GroupMemberKey{
		GroupOpenID: "SOURCE", MemberOpenID: "M1",
	}); err != nil || found {
		t.Fatalf("source schedule still exists found=%v err=%v", found, err)
	}
	if _, found, err := handle.GetGroupMemberSchedule(context.Background(), schedule.GroupMemberKey{
		GroupOpenID: "TARGET", MemberOpenID: "M1",
	}); err != nil || found {
		t.Fatalf("target empty schedule still exists found=%v err=%v", found, err)
	}
	pending, err = service.PendingMembers(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].UserID != "M1" {
		t.Fatalf("pending after delete = %+v", pending)
	}
}

func TestGlobalModeCreateDoesNotAddEmptyShadowSchedule(t *testing.T) {
	service, handle := newService(t)
	settings, err := service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.StorageMode = schedule.StorageModeGlobal
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(
		schedule.ScopeGroup("SOURCE2"), "M1", "全局课表", adminICS, "global.ics", "M1",
	); err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateMemberSchedules(
		schedule.ScopeGroup("TARGET2"),
		[]admin.NewMember{{UserID: "M1", Name: "当前群成员"}},
		"webui",
	)
	if err == nil {
		t.Fatal("CreateMemberSchedules should skip a globally resolved member")
	}
	if _, found, err := handle.GetGroupMemberSchedule(context.Background(), schedule.GroupMemberKey{
		GroupOpenID: "TARGET2", MemberOpenID: "M1",
	}); err != nil || found {
		t.Fatalf("empty shadow schedule created found=%v err=%v", found, err)
	}
}
