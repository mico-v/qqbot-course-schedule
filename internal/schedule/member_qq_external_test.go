package schedule_test

import (
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestNormalizeQQ(t *testing.T) {
	valid := []string{"12345", "1234567890", "12345678901"}
	for _, value := range valid {
		got, err := schedule.NormalizeQQ(" " + value + " ")
		if err != nil || got != value {
			t.Fatalf("NormalizeQQ(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"1234", "012345", "123456789012", "abcde", "123 456"} {
		if _, err := schedule.NormalizeQQ(value); err == nil {
			t.Fatalf("NormalizeQQ(%q) should fail", value)
		}
	}
	if got, err := schedule.NormalizeQQ("  "); err != nil || got != "" {
		t.Fatalf("empty = %q, %v", got, err)
	}
}

func TestSetMemberQQAndBackup(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("GQQ")
	if _, err := service.SaveICS(scope, "U1", "小明", adminICS, "s.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetMemberQQ(scope, "U1", "123456789", "U1"); err != nil {
		t.Fatalf("SetMemberQQ: %v", err)
	}
	page, found, err := service.PageSchedule(scope, "U1")
	if err != nil || !found || page.QQ != "123456789" {
		t.Fatalf("page = %+v found=%v err=%v", page, found, err)
	}
	bindings, err := service.MemberQQBindings(scope)
	if err != nil || bindings["U1"] != "123456789" {
		t.Fatalf("bindings = %+v err=%v", bindings, err)
	}

	// Saving the page without a QQ keeps the existing binding.
	name := "小明"
	revision := page.Revision
	events := []schedule.WebEventInput{{ID: 1, Course: "高等数学", Start: "2026-09-01T08:00", End: "2026-09-01T09:30"}}
	if _, err := service.SavePageSchedule(schedule.SavePagePayload{
		ScopeID: scope, UserID: "U1", Revision: &revision, Name: &name, Events: events,
	}, "webui"); err != nil {
		t.Fatalf("SavePageSchedule: %v", err)
	}
	if page, _, _ = service.PageSchedule(scope, "U1"); page.QQ != "123456789" {
		t.Fatalf("qq lost after save: %+v", page)
	}

	// Backup carries the binding; import restores it.
	backup, err := service.ExportBackup(scope)
	if err != nil || len(backup.Members) != 1 || backup.Members[0].QQ != "123456789" {
		t.Fatalf("backup = %+v err=%v", backup, err)
	}
	if _, err := service.ImportBackup(schedule.ScopeGroup("GQQ2"), backup, "webui"); err != nil {
		t.Fatal(err)
	}
	if page, _, _ = service.PageSchedule(schedule.ScopeGroup("GQQ2"), "U1"); page.QQ != "123456789" {
		t.Fatalf("imported qq = %q", page.QQ)
	}

	// Invalid numbers and unknown members fail.
	if _, err := service.SetMemberQQ(scope, "U1", "not-a-qq", "U1"); err == nil {
		t.Fatal("invalid qq should fail")
	}
	if _, err := service.SetMemberQQ(scope, "U9", "123456789", "U1"); err == nil {
		t.Fatal("unknown member should fail")
	}
	if _, err := service.SetMemberQQ(scope, "U1", "", "U1"); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if page, _, _ = service.PageSchedule(scope, "U1"); page.QQ != "" {
		t.Fatalf("qq after unbind = %q", page.QQ)
	}
}
