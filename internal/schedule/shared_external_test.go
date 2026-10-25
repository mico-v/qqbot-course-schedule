package schedule_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func enableGlobalStorage(t *testing.T, service *schedule.Service) {
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

func TestGlobalStorageSharesScheduleByUnionOpenID(t *testing.T) {
	service, _ := newService(t)
	enableGlobalStorage(t, service)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if err := service.RecordIdentity(groupOne, "U-G1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U-G2", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U-NO-SCHEDULE", "UNION-B"); err != nil {
		t.Fatal(err)
	}

	target, err := service.ResolveStorageTarget(groupOne, "U-G1", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.ScopeID != schedule.ScopeGlobalUser("UNION-A") || target.UserID != "UNION-A" {
		t.Fatalf("storage target = %+v", target)
	}
	if _, err := service.SaveICS(target.ScopeID, target.UserID, "小明", adminICS, "schedule.ics", "U-G1"); err != nil {
		t.Fatalf("SaveICS: %v", err)
	}

	visible, err := service.VisibleMembers(groupTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 {
		t.Fatalf("visible members = %+v, want only the shared identity", visible)
	}
	member := visible["U-G2"]
	if member == nil || member.UserID != "U-G2" || len(member.Events) != 1 {
		t.Fatalf("projected member = %+v", member)
	}
	if member.Events[0]["SUMMARY"] != "高等数学" || member.Name != "小明" {
		t.Fatalf("projected schedule = %+v", member)
	}

	if _, err := service.ResolveStorageTarget(groupTwo, "U-UNKNOWN", ""); err == nil ||
		!strings.Contains(err.Error(), "先在本群发送一条消息") {
		t.Fatalf("missing identity error = %v", err)
	}
}

func TestGlobalOverridesAndCheckinsKeepScopeBoundaries(t *testing.T) {
	service, _ := newService(t)
	enableGlobalStorage(t, service)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if err := service.RecordIdentity(groupOne, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}

	target, err := service.ResolveStorageTarget(groupOne, "U1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(target.ScopeID, target.UserID, "小明", adminICS, "schedule.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	one := time.Date(2026, 10, 1, 0, 0, 0, 0, schedule.LocalTZ)
	two := time.Date(2026, 10, 2, 0, 0, 0, 0, schedule.LocalTZ)
	_, err = service.SetDayOverridesForTargets([]schedule.DayOverrideTarget{
		{ScopeID: groupOne, UserID: schedule.DayOverrideAll, DisplayID: schedule.DayOverrideAll},
	}, []time.Time{one}, schedule.DayOverrideHoliday, nil, "U1", nil, two)
	if err != nil {
		t.Fatalf("SetDayOverrides for group-wide: %v", err)
	}
	_, err = service.SetDayOverridesForTargets([]schedule.DayOverrideTarget{
		{ScopeID: target.ScopeID, UserID: target.UserID, DisplayID: "U1"},
	}, []time.Time{two}, schedule.DayOverrideShift, &one, "U1", nil, two)
	if err != nil {
		t.Fatalf("SetDayOverrides for member: %v", err)
	}

	visibleOne, err := service.VisibleMembers(groupOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(visibleOne["U1"].DayOverrides) != 2 {
		t.Fatalf("group one overrides = %+v", visibleOne["U1"].DayOverrides)
	}
	visibleTwo, err := service.VisibleMembers(groupTwo)
	if err != nil {
		t.Fatal(err)
	}
	if len(visibleTwo["U1"].DayOverrides) != 1 || visibleTwo["U1"].DayOverrides["2026-10-02"].Kind != schedule.DayOverrideShift {
		t.Fatalf("group two overrides = %+v", visibleTwo["U1"].DayOverrides)
	}
	text, err := service.ClearDayOverridesForTargets(groupOne, []schedule.DayOverrideTarget{{
		ScopeID: target.ScopeID, UserID: target.UserID, DisplayID: "U1",
	}}, []time.Time{one}, visibleOne)
	if err != nil {
		t.Fatalf("ClearDayOverridesForTargets: %v", err)
	}
	if !strings.Contains(text, "面向全体成员的标记") {
		t.Fatalf("scope-wide clear text = %q", text)
	}

	if _, err := service.Checkin(groupOne, "U1", "小明", two, 5); err != nil {
		t.Fatal(err)
	}
	otherStatus, err := service.CheckinStatus(groupTwo, "U1")
	if err != nil {
		t.Fatal(err)
	}
	if otherStatus.Total != 0 || otherStatus.Days != 0 {
		t.Fatalf("check-in leaked across groups: %+v", otherStatus)
	}
}

func TestStorageModeNormalizationAndIdentityMap(t *testing.T) {
	for input, want := range map[string]string{
		"":       schedule.StorageModeGroup,
		"group":  schedule.StorageModeGroup,
		"global": schedule.StorageModeGlobal,
		"other":  schedule.StorageModeGroup,
	} {
		if got := schedule.NormalizeStorageMode(input); got != want {
			t.Fatalf("NormalizeStorageMode(%q) = %q, want %q", input, got, want)
		}
	}

	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	if err := service.RecordIdentity(scope, "LOCAL", "UNION"); err != nil {
		t.Fatal(err)
	}
	globalID, found, err := service.ResolveIdentity(scope, "LOCAL")
	if err != nil || !found || globalID != "UNION" {
		t.Fatalf("ResolveIdentity = %q, %v, %v", globalID, found, err)
	}
	identities, err := service.ScopeIdentities(scope)
	if err != nil || identities["LOCAL"] != "UNION" {
		t.Fatalf("ScopeIdentities = %+v, %v", identities, err)
	}
}
