package admin_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func enableAdminGlobalStorage(t *testing.T, service *admin.Service) {
	t.Helper()
	settings, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	settings.StorageMode = schedule.StorageModeGlobal
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatalf("SaveBotSettings: %v", err)
	}
}

func TestGlobalStorageAdminEditsSharedScheduleAcrossGroups(t *testing.T) {
	service, _ := newService(t)
	enableAdminGlobalStorage(t, service)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if err := service.RecordIdentity(groupOne, "U-G1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U-G2", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	target, err := service.ResolveStorageTarget(groupOne, "U-G1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(target.ScopeID, target.UserID, "小明", adminICS, "schedule.ics", "U-G1"); err != nil {
		t.Fatal(err)
	}

	page, found, err := service.PageSchedule(groupTwo, "U-G2")
	if err != nil || !found {
		t.Fatalf("PageSchedule through second group: found=%v err=%v", found, err)
	}
	if page.ScopeID != groupTwo || page.UserID != "U-G2" ||
		len(page.Events) != 1 || page.Events[0].Course != "高等数学" {
		t.Fatalf("projected page = %+v", page)
	}

	name := "小明（全局）"
	revision := page.Revision
	_, err = service.SavePageSchedule(admin.SavePagePayload{
		ScopeID:  groupTwo,
		UserID:   "U-G2",
		Revision: &revision,
		Name:     &name,
		Events: []admin.WebEventInput{{
			ID: page.Events[0].ID, UID: page.Events[0].UID, Course: "高等数学 A",
			Start: page.Events[0].Start, End: page.Events[0].End, RRule: page.Events[0].RRule,
		}},
	}, "webui")
	if err != nil {
		t.Fatalf("SavePageSchedule through second group: %v", err)
	}

	firstPage, found, err := service.PageSchedule(groupOne, "U-G1")
	if err != nil || !found || firstPage.Name != name || firstPage.Events[0].Course != "高等数学 A" {
		t.Fatalf("first group page = %+v found=%v err=%v", firstPage, found, err)
	}
}

func TestGlobalStorageAdminPageUsesLegacyCurrentGroupThenFallback(t *testing.T) {
	service, _ := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if _, err := service.SaveICS(groupOne, "U1", "旧群课表", adminICS, "legacy.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableAdminGlobalStorage(t, service)

	pageOne, found, err := service.PageSchedule(groupOne, "U1")
	if err != nil || !found || pageOne.Name != "旧群课表" {
		t.Fatalf("legacy page = %+v found=%v err=%v", pageOne, found, err)
	}
	pageTwo, found, err := service.PageSchedule(groupTwo, "U1")
	if err != nil || !found || pageTwo.Name != "旧群课表" {
		t.Fatalf("fallback page = %+v found=%v err=%v", pageTwo, found, err)
	}

	name := "全局副本"
	revision := pageTwo.Revision
	if _, err := service.SavePageSchedule(admin.SavePagePayload{
		ScopeID:  groupTwo,
		UserID:   "U1",
		Revision: &revision,
		Name:     &name,
		Events: []admin.WebEventInput{{
			ID: pageTwo.Events[0].ID, UID: pageTwo.Events[0].UID, Course: pageTwo.Events[0].Course,
			Start: pageTwo.Events[0].Start, End: pageTwo.Events[0].End, RRule: pageTwo.Events[0].RRule,
		}},
	}, "webui"); err != nil {
		t.Fatalf("copy-on-write save: %v", err)
	}
	pageOne, found, err = service.PageSchedule(groupOne, "U1")
	if err != nil || !found || pageOne.Name != "旧群课表" {
		t.Fatalf("current-group page after global save = %+v found=%v err=%v", pageOne, found, err)
	}
	pageTwo, found, err = service.PageSchedule(groupTwo, "U1")
	if err != nil || !found || pageTwo.Name != name {
		t.Fatalf("global page after global save = %+v found=%v err=%v", pageTwo, found, err)
	}
}

func TestGlobalEditLinkKeepsExactTargetAfterModeSwitch(t *testing.T) {
	service, _ := newService(t)
	enableAdminGlobalStorage(t, service)
	scope := schedule.ScopeGroup("G1")
	if err := service.RecordIdentity(scope, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	target, err := service.ResolveStorageTarget(scope, "U1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(target.ScopeID, target.UserID, "小明", adminICS, "schedule.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, schedule.LocalTZ)
	token, _, err := service.CreateScheduleEditLink(target.ScopeID, target.UserID, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}

	settings, err := service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.StorageMode = schedule.StorageModeGroup
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}

	page, found, err := service.PublicPageSchedule(token, now)
	if err != nil || !found || len(page.Events) != 1 {
		t.Fatalf("PublicPageSchedule = %+v found=%v err=%v", page, found, err)
	}
	name := "切换模式后仍可保存"
	revision := page.Revision
	result, err := service.SavePublicSchedule(token, admin.PublicSavePayload{
		Revision: &revision,
		Name:     &name,
		Events: []admin.WebEventInput{{
			ID: page.Events[0].ID, UID: page.Events[0].UID, Course: "高等数学",
			Start: page.Events[0].Start, End: page.Events[0].End, RRule: page.Events[0].RRule,
		}},
	}, now)
	if err != nil {
		t.Fatalf("SavePublicSchedule: %v", err)
	}
	if result.Name != name {
		t.Fatalf("result = %+v", result)
	}
	saved, found, err := service.PageScheduleAt(target.ScopeID, target.UserID)
	if err != nil || !found || saved.Name != name {
		t.Fatalf("exact global target = %+v found=%v err=%v", saved, found, err)
	}
}

func TestGlobalBackupExportsProjectedMember(t *testing.T) {
	service, _ := newService(t)
	enableAdminGlobalStorage(t, service)
	scope := schedule.ScopeGroup("G1")
	if err := service.RecordIdentity(scope, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	target, err := service.ResolveStorageTarget(scope, "U1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(target.ScopeID, target.UserID, "小明", adminICS, "schedule.ics", "U1"); err != nil {
		t.Fatal(err)
	}

	backup, err := service.ExportBackup(scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.Members) != 1 || backup.Members[0].UserID != "U1" ||
		backup.Members[0].Name != "小明" || len(backup.Members[0].Events) != 1 {
		t.Fatalf("backup = %+v", backup)
	}
	if strings.Contains(backup.Members[0].ICS, "UNION-A") {
		t.Fatalf("backup leaked global storage coordinate: %q", backup.Members[0].ICS)
	}
}
