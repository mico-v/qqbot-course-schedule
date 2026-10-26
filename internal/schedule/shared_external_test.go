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

func TestGlobalStorageUsesLatestLegacyImportWithoutIdentity(t *testing.T) {
	service, storeHandle := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	for _, member := range []struct {
		scopeID  string
		name     string
		imported string
	}{
		{scopeID: groupOne, name: "较早导入", imported: "2026-09-22T13:38:51Z"},
		{scopeID: groupTwo, name: "最后导入", imported: "2026-09-22T13:48:42Z"},
	} {
		if err := storeHandle.PutMember(member.scopeID, "U1", &schedule.Member{
			UserID:            "U1",
			Name:              member.name,
			Events:            []schedule.Event{{"SUMMARY": member.name}},
			ScheduleUpdatedAt: member.imported,
			ImportGroupOpenID: strings.TrimPrefix(member.scopeID, "group:"),
			ImportUserOpenID:  "U1",
		}, nil); err != nil {
			t.Fatalf("PutMember(%s): %v", member.scopeID, err)
		}
	}
	enableGlobalStorage(t, service)

	visibleOne, err := service.VisibleMembers(groupOne)
	if err != nil {
		t.Fatal(err)
	}
	if visibleOne["U1"] == nil || visibleOne["U1"].Name != "最后导入" {
		t.Fatalf("group one visible = %+v", visibleOne)
	}
	visibleTwo, err := service.VisibleMembers(groupTwo)
	if err != nil {
		t.Fatal(err)
	}
	if visibleTwo["U1"] == nil || visibleTwo["U1"].Name != "最后导入" {
		t.Fatalf("group two visible = %+v", visibleTwo)
	}

	target, err := service.ResolveStorageTarget(groupOne, "U1", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.ScopeID != groupTwo || target.UserID != "U1" {
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

	if _, err := service.SaveICSWithSource(groupTwo, "U2", "另一位成员", adminICS, "other.ics", "U2", schedule.ImportSource{
		GroupOpenID: "G2",
		UserOpenID:  "U2",
		UnionOpenID: "UNION-B",
	}); err != nil {
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

func TestGlobalStorageUsesLatestExactUnionImport(t *testing.T) {
	service, storeHandle := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	groupThree := schedule.ScopeGroup("G3")
	for _, member := range []struct {
		scopeID  string
		userID   string
		name     string
		imported string
	}{
		{scopeID: groupOne, userID: "U1", name: "较早导入", imported: "2026-10-01T08:00:00Z"},
		{scopeID: groupTwo, userID: "U2", name: "最后导入", imported: "2026-10-02T08:00:00Z"},
	} {
		if err := storeHandle.PutMember(member.scopeID, member.userID, &schedule.Member{
			UserID:            member.userID,
			Name:              member.name,
			Events:            []schedule.Event{{"SUMMARY": member.name}},
			ScheduleUpdatedAt: member.imported,
			ImportGroupOpenID: strings.TrimPrefix(member.scopeID, "group:"),
			ImportUserOpenID:  member.userID,
			ImportUnionOpenID: "UNION-A",
		}, nil); err != nil {
			t.Fatalf("PutMember: %v", err)
		}
	}
	if err := service.RecordIdentity(groupThree, "U3", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	member, found, err := service.VisibleMember(groupThree, "U3", "")
	if err != nil || !found || member.Name != "最后导入" {
		t.Fatalf("latest exact union member = %+v found=%v err=%v", member, found, err)
	}
}

func TestGlobalStorageUsesLatestLocalOpenIDImportWithoutUnion(t *testing.T) {
	service, storeHandle := newService(t)
	groupOld := schedule.ScopeGroup("G1")
	groupLatest := schedule.ScopeGroup("G2")
	groupCurrent := schedule.ScopeGroup("G3")
	const localUserID = "337202E7E24BFB043BB7B7349222AE6B"
	for _, member := range []struct {
		scopeID  string
		name     string
		imported string
	}{
		{scopeID: groupOld, name: "较早导入", imported: "2026-09-22T13:38:51Z"},
		{scopeID: groupLatest, name: "最后导入", imported: "2026-09-22T13:48:42Z"},
	} {
		if err := storeHandle.PutMember(member.scopeID, localUserID, &schedule.Member{
			UserID:            localUserID,
			Name:              member.name,
			Events:            []schedule.Event{{"SUMMARY": member.name}},
			ScheduleUpdatedAt: member.imported,
			ImportGroupOpenID: strings.TrimPrefix(member.scopeID, "group:"),
			ImportUserOpenID:  localUserID,
		}, nil); err != nil {
			t.Fatalf("PutMember: %v", err)
		}
	}
	if err := service.RecordSeenMember(groupCurrent, localUserID, "謎矢"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	target, err := service.ResolveStorageTarget(groupCurrent, localUserID, "")
	if err != nil {
		t.Fatalf("ResolveStorageTarget: %v", err)
	}
	if target.ScopeID != groupLatest || target.UserID != localUserID {
		t.Fatalf("local fallback target = %+v", target)
	}
	member, found, err := service.VisibleMember(groupCurrent, localUserID, "")
	if err != nil || !found || member.Name != "最后导入" {
		t.Fatalf("local fallback member = %+v found=%v err=%v", member, found, err)
	}

	settings, err := service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.StorageMode = schedule.StorageModeGroup
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	member, found, err = service.VisibleMember(groupCurrent, localUserID, "")
	if err != nil || found || member != nil {
		t.Fatalf("isolated member leaked from another group: member=%+v found=%v err=%v", member, found, err)
	}
}

func TestGlobalStoragePrefersExactUnionOverSameLocalOpenID(t *testing.T) {
	service, storeHandle := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	groupCurrent := schedule.ScopeGroup("G3")
	const localUserID = "SAME-LOCAL"
	for _, member := range []struct {
		scopeID  string
		unionID  string
		name     string
		imported string
	}{
		{scopeID: groupOne, unionID: "UNION-A", name: "正确的全局课表", imported: "2026-10-01T08:00:00Z"},
		{scopeID: groupTwo, unionID: "UNION-B", name: "同本地 ID 的另一人", imported: "2026-10-02T08:00:00Z"},
	} {
		if err := storeHandle.PutMember(member.scopeID, localUserID, &schedule.Member{
			UserID:            localUserID,
			Name:              member.name,
			Events:            []schedule.Event{{"SUMMARY": member.name}},
			ScheduleUpdatedAt: member.imported,
			ImportGroupOpenID: strings.TrimPrefix(member.scopeID, "group:"),
			ImportUserOpenID:  localUserID,
			ImportUnionOpenID: member.unionID,
		}, nil); err != nil {
			t.Fatalf("PutMember: %v", err)
		}
	}
	if err := service.RecordIdentity(groupCurrent, localUserID, "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	member, found, err := service.VisibleMember(groupCurrent, localUserID, "")
	if err != nil || !found || member.Name != "正确的全局课表" {
		t.Fatalf("union priority member = %+v found=%v err=%v", member, found, err)
	}
}

func TestGlobalStorageExactUnionOutranksLaterLocalFallback(t *testing.T) {
	service, storeHandle := newService(t)
	groupExact := schedule.ScopeGroup("G1")
	groupCurrent := schedule.ScopeGroup("G2")
	const localUserID = "SAME-LOCAL"
	if err := storeHandle.PutMember(groupExact, localUserID, &schedule.Member{
		UserID:            localUserID,
		Name:              "正确的全局课表",
		Events:            []schedule.Event{{"SUMMARY": "正确的全局课表"}},
		ScheduleUpdatedAt: "2026-10-01T08:00:00Z",
		ImportGroupOpenID: "G1",
		ImportUserOpenID:  localUserID,
		ImportUnionOpenID: "UNION-A",
	}, nil); err != nil {
		t.Fatalf("PutMember exact union: %v", err)
	}
	if err := storeHandle.PutMember(groupCurrent, localUserID, &schedule.Member{
		UserID:            localUserID,
		Name:              "较新的本群旧课表",
		Events:            []schedule.Event{{"SUMMARY": "较新的本群旧课表"}},
		ScheduleUpdatedAt: "2026-10-02T08:00:00Z",
		ImportGroupOpenID: "G2",
		ImportUserOpenID:  localUserID,
	}, nil); err != nil {
		t.Fatalf("PutMember local fallback: %v", err)
	}
	if err := service.RecordIdentity(groupCurrent, localUserID, "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	member, found, err := service.VisibleMember(groupCurrent, localUserID, "")
	if err != nil || !found || member.Name != "正确的全局课表" {
		t.Fatalf("union priority member = %+v found=%v err=%v", member, found, err)
	}
}

func TestGlobalStorageTieBreakIsDeterministic(t *testing.T) {
	service, storeHandle := newService(t)
	groupThree := schedule.ScopeGroup("G3")
	const sameTime = "2026-10-01T08:00:00Z"
	for _, member := range []struct {
		scopeID string
		userID  string
		name    string
	}{
		{scopeID: schedule.ScopeGroup("G2"), userID: "U2", name: "后排序"},
		{scopeID: schedule.ScopeGroup("G1"), userID: "U1", name: "先排序"},
	} {
		if err := storeHandle.PutMember(member.scopeID, member.userID, &schedule.Member{
			UserID:            member.userID,
			Name:              member.name,
			Events:            []schedule.Event{{"SUMMARY": member.name}},
			ScheduleUpdatedAt: sameTime,
			ImportGroupOpenID: strings.TrimPrefix(member.scopeID, "group:"),
			ImportUserOpenID:  member.userID,
			ImportUnionOpenID: "UNION-A",
		}, nil); err != nil {
			t.Fatalf("PutMember: %v", err)
		}
	}
	if err := service.RecordIdentity(groupThree, "U3", "UNION-A"); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, service)

	for range 5 {
		member, found, err := service.VisibleMember(groupThree, "U3", "")
		if err != nil || !found || member.Name != "先排序" {
			t.Fatalf("tie-break member = %+v found=%v err=%v", member, found, err)
		}
	}
}

func TestGlobalStorageFallbackOverridesStayInCurrentScope(t *testing.T) {
	service, _ := newService(t)
	groupOne := schedule.ScopeGroup("G1")
	groupTwo := schedule.ScopeGroup("G2")
	if _, err := service.SaveICSWithSource(groupOne, "U1", "旧群课表", adminICS, "legacy.ics", "U1", schedule.ImportSource{
		GroupOpenID: "G1",
		UserOpenID:  "U1",
		UnionOpenID: "UNION-A",
	}); err != nil {
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
