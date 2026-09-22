package schedule_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func backupDay(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02", value, schedule.LocalTZ)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestBackupRoundTrip(t *testing.T) {
	service, _ := newService(t)
	source := schedule.ScopeGroup("G1")
	target := schedule.ScopeGroup("G2")
	if _, err := service.SaveICS(source, "U1", "小明", adminICS, "schedule.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	members, _ := service.ScopeMembers(source)
	if _, err := service.SetDayOverrides(source, []string{schedule.DayOverrideAll},
		[]time.Time{backupDay(t, "2026-10-01")}, schedule.DayOverrideHoliday, nil, "U1", members, backupDay(t, "2026-09-17")); err != nil {
		t.Fatalf("SetDayOverrides: %v", err)
	}

	backup, err := service.ExportBackup(source)
	if err != nil {
		t.Fatalf("ExportBackup: %v", err)
	}
	if backup.Version != 1 || len(backup.Members) != 1 || len(backup.Overrides) != 1 {
		t.Fatalf("backup = %+v", backup)
	}
	if backup.Members[0].UserID != "U1" || len(backup.Members[0].Events) != 1 {
		t.Fatalf("backup member = %+v", backup.Members[0])
	}

	result, err := service.ImportBackup(target, backup, "webui")
	if err != nil {
		t.Fatalf("ImportBackup: %v", err)
	}
	if result.MemberCount != 1 || result.CreatedCount != 1 || result.EventCount != 1 || result.OverrideCount != 1 {
		t.Fatalf("import result = %+v", result)
	}
	restored, found, err := service.PageSchedule(target, "U1")
	if err != nil || !found || len(restored.Events) != 1 || restored.Name != "小明" {
		t.Fatalf("restored = %+v found=%v err=%v", restored, found, err)
	}
	overrides, err := service.DayOverrideListText(target, map[string]*schedule.Member{"U1": {UserID: "U1", Name: "小明"}}, backupDay(t, "2026-09-17"))
	if err != nil || !strings.Contains(overrides, "2026-10-01 休假") {
		t.Fatalf("overrides = %q err=%v", overrides, err)
	}

	// Re-importing replaces the override set: an extra marker is cleared.
	extraMembers, _ := service.ScopeMembers(target)
	if _, err := service.SetDayOverrides(target, []string{schedule.DayOverrideAll},
		[]time.Time{backupDay(t, "2026-12-25")}, schedule.DayOverrideHoliday, nil, "U1", extraMembers, backupDay(t, "2026-09-17")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ImportBackup(target, backup, "webui"); err != nil {
		t.Fatal(err)
	}
	list, _ := service.DayOverrideListText(target, nil, backupDay(t, "2026-09-17"))
	if strings.Contains(list, "2026-12-25") {
		t.Fatalf("stale override survived restore: %q", list)
	}
}
