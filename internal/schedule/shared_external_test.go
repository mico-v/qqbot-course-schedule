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

func TestGlobalStorageKeepsLegacyGroupSchedulesWithoutIdentity(t *testing.T) {
	service, _ := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if _, err := service.SaveICS(groupOne, "U1", "第一群课表", adminICS, "one.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(groupTwo, "U1", "第二群课表", adminICS, "two.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	visibleOne, err := service.VisibleMembers(groupOne)
	if err != nil {
		t.Fatal(err)
	}
	if visibleOne["U1"] == nil || visibleOne["U1"].Name != "第一群课表" {
		t.Fatalf("group one visible = %+v", visibleOne)
	}
	visibleTwo, err := service.VisibleMembers(groupTwo)
	if err != nil {
		t.Fatal(err)
	}
	if visibleTwo["U1"] == nil || visibleTwo["U1"].Name != "第二群课表" {
		t.Fatalf("group two visible = %+v", visibleTwo)
	}

	target, err := service.ResolveStorageTarget(groupOne, "U1", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.ScopeID != groupOne || target.UserID != "U1" {
		t.Fatalf("legacy write target = %+v", target)
	}
}

func TestGlobalStorageFallsBackToOtherGroupsAndPrefersGlobal(t *testing.T) {
	service, _ := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	groupThree := schedule.ScopeGroup("G3")
	if _, err := service.SaveICS(groupOne, "U1", "旧群课表", adminICS, "legacy.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(schedule.ScopeGlobalUser("UNION-A"), "UNION-A", "全局课表", adminICS, "global.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupThree, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	visibleOne, err := service.VisibleMembers(groupOne)
	if err != nil {
		t.Fatal(err)
	}
	if visibleOne["U1"] == nil || visibleOne["U1"].Name != "旧群课表" {
		t.Fatalf("current group should win: %+v", visibleOne)
	}
	visibleThree, err := service.VisibleMembers(groupThree)
	if err != nil {
		t.Fatal(err)
	}
	if visibleThree["U1"] == nil || visibleThree["U1"].Name != "全局课表" {
		t.Fatalf("global schedule should win before another group: %+v", visibleThree)
	}

	if _, err := service.SaveICS(groupTwo, "U2", "另一位成员", adminICS, "other.ics", "U2"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U2", "UNION-B"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupThree, "U2", "UNION-B"); err != nil {
		t.Fatal(err)
	}
	visibleThree, err = service.VisibleMembers(groupThree)
	if err != nil {
		t.Fatal(err)
	}
	if visibleThree["U2"] == nil || visibleThree["U2"].Name != "另一位成员" {
		t.Fatalf("other-group fallback = %+v", visibleThree["U2"])
	}
}

func TestGlobalStorageFallbackOverridesStayInCurrentScope(t *testing.T) {
	service, _ := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if _, err := service.SaveICS(groupOne, "U1", "旧群课表", adminICS, "legacy.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordIdentity(groupTwo, "U1", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	sourceHoliday := time.Date(2026, 10, 1, 0, 0, 0, 0, schedule.LocalTZ)
	currentHoliday := time.Date(2026, 10, 2, 0, 0, 0, 0, schedule.LocalTZ)
	globalShift := time.Date(2026, 10, 3, 0, 0, 0, 0, schedule.LocalTZ)
	if _, err := service.SetDayOverridesForTargets([]schedule.DayOverrideTarget{{
		ScopeID: groupOne, UserID: schedule.DayOverrideAll, DisplayID: schedule.DayOverrideAll,
	}}, []time.Time{sourceHoliday}, schedule.DayOverrideHoliday, nil, "U1", nil, sourceHoliday); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetDayOverridesForTargets([]schedule.DayOverrideTarget{{
		ScopeID: groupTwo, UserID: schedule.DayOverrideAll, DisplayID: schedule.DayOverrideAll,
	}}, []time.Time{currentHoliday}, schedule.DayOverrideHoliday, nil, "U1", nil, currentHoliday); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetDayOverridesForTargets([]schedule.DayOverrideTarget{{
		ScopeID: schedule.ScopeGlobalUser("UNION-A"), UserID: "UNION-A", DisplayID: "U1",
	}}, []time.Time{globalShift}, schedule.DayOverrideShift, &sourceHoliday, "U1", nil, globalShift); err != nil {
		t.Fatal(err)
	}

	visible, err := service.VisibleMembers(groupTwo)
	if err != nil {
		t.Fatal(err)
	}
	member := visible["U1"]
	if member == nil || member.Name != "旧群课表" {
		t.Fatalf("fallback member = %+v", member)
	}
	if _, exists := member.DayOverrides["2026-10-01"]; exists {
		t.Fatalf("source scope-wide override leaked: %+v", member.DayOverrides)
	}
	if member.DayOverrides["2026-10-02"].Kind != schedule.DayOverrideHoliday ||
		member.DayOverrides["2026-10-03"].Kind != schedule.DayOverrideShift {
		t.Fatalf("effective overrides = %+v", member.DayOverrides)
	}
}

func TestGlobalOverridesKeepScopeBoundaries(t *testing.T) {
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
