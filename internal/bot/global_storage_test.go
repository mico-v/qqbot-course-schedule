package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func enableGlobalStorage(t *testing.T, env *Env) {
	t.Helper()
	settings, err := env.Service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	settings.StorageMode = schedule.StorageModeGlobal
	if err := env.Service.SaveBotSettings(settings); err != nil {
		t.Fatalf("SaveBotSettings: %v", err)
	}
}

func TestGlobalUserIDFallsBackToMemberAPIAndCaches(t *testing.T) {
	fake := newFakeQQ()
	fake.memberUnionIDs["U1"] = "UNION-U1"
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	enableGlobalStorage(t, env)

	globalID, err := env.GlobalUserID(context.Background(), message)
	if err != nil {
		t.Fatalf("GlobalUserID: %v", err)
	}
	if globalID != "UNION-U1" {
		t.Fatalf("global ID = %q, want UNION-U1", globalID)
	}
	if calls := fake.memberCallCount(); calls != 1 {
		t.Fatalf("member API calls = %d, want 1", calls)
	}

	globalID, err = env.GlobalUserID(context.Background(), message)
	if err != nil || globalID != "UNION-U1" {
		t.Fatalf("cached GlobalUserID = %q, %v", globalID, err)
	}
	if calls := fake.memberCallCount(); calls != 1 {
		t.Fatalf("member API was called again: %d", calls)
	}
	resolved, found, err := env.Service.ResolveIdentity(env.Scope(message), message.UserOpenID)
	if err != nil || !found || resolved != "UNION-U1" {
		t.Fatalf("recorded identity = %q, %v, %v", resolved, found, err)
	}
}

func TestGlobalUserIDUsesMessageValueWithoutRequest(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	enableGlobalStorage(t, env)
	message.GlobalUserOpenID = "UNION-MESSAGE"

	globalID, err := env.GlobalUserID(context.Background(), message)
	if err != nil || globalID != "UNION-MESSAGE" {
		t.Fatalf("GlobalUserID = %q, %v", globalID, err)
	}
	if calls := fake.memberCallCount(); calls != 0 {
		t.Fatalf("member API calls = %d, want 0", calls)
	}
}

func TestGlobalStorageUsesLegacyGroupScheduleWithoutUnionOpenID(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	scopeID := env.Scope(message)
	if _, err := env.Service.SaveICS(scopeID, message.UserOpenID, "小明", testICS, "legacy.ics", message.UserOpenID); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, env)

	members, err := env.ScheduleMembers(context.Background(), message)
	if err != nil {
		t.Fatalf("ScheduleMembers: %v", err)
	}
	if member := members[message.UserOpenID]; member == nil || len(member.Events) == 0 {
		t.Fatalf("legacy member = %+v", member)
	}

	target, err := env.ScheduleTarget(context.Background(), message, message.UserOpenID)
	if err != nil {
		t.Fatalf("ScheduleTarget: %v", err)
	}
	if target.ScopeID != scopeID || target.UserID != message.UserOpenID {
		t.Fatalf("legacy target = %+v", target)
	}
	if calls := fake.memberCallCount(); calls != 1 {
		t.Fatalf("member API calls = %d, want 1", calls)
	}
}

func TestGlobalStorageMatchesLegacyScheduleAcrossGroupsByLocalOpenID(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	if _, err := env.Service.SaveICS(env.Scope(message), message.UserOpenID, "旧群课表", testICS, "legacy.ics", message.UserOpenID); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, env)

	otherGroupMessage := *message
	otherGroupMessage.GroupOpenID = "GROUP-2"
	members, err := env.ScheduleMembers(context.Background(), &otherGroupMessage)
	if err != nil {
		t.Fatalf("ScheduleMembers: %v", err)
	}
	if member := members[message.UserOpenID]; member == nil || member.Name != "旧群课表" {
		t.Fatalf("cross-group local OpenID fallback = %+v", member)
	}
	if calls := fake.memberCallCount(); calls != 0 {
		t.Fatalf("member API calls = %d, want 0", calls)
	}
}

func TestGlobalImportWithoutUnionKeepsCurrentGroupSource(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	oldScope := schedule.ScopeGroup("GROUP-OLD")
	if _, err := env.Service.SaveICSWithSource(
		oldScope, message.UserOpenID, "旧群课表", testICS, "old.ics", message.UserOpenID,
		schedule.ImportSource{GroupOpenID: "GROUP-OLD", UserOpenID: message.UserOpenID},
	); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, env)

	if err := env.ImportICS(context.Background(), message, NewReplier(message, env.Client), Attachment{
		URL:      icsServer.URL + "/schedule.ics",
		Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}
	saved, err := env.Service.ScopeMembers(env.Scope(message))
	if err != nil || saved[message.UserOpenID] == nil {
		t.Fatalf("current group import = %+v err=%v", saved, err)
	}
	member := saved[message.UserOpenID]
	if member.ImportGroupOpenID != message.GroupOpenID ||
		member.ImportUserOpenID != message.UserOpenID ||
		member.ImportUnionOpenID != "" {
		t.Fatalf("current group source = %+v", member)
	}
}

func TestGlobalStorageUsesRosterAndLastImportedSchedule(t *testing.T) {
	fake := newFakeQQ()
	fake.rosterMembers = []map[string]any{
		{
			"member_openid": "U1",
			"username":      "小明",
			"member_role":   "member",
			"bot":           false,
			"union_openid":  "UNION-U1",
		},
		{
			"member_openid": "U2",
			"username":      "小红",
			"member_role":   "member",
			"bot":           false,
			"union_openid":  "UNION-U2",
		},
	}
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	enableGlobalStorage(t, env)
	if _, err := env.Service.SaveICSWithSource(
		schedule.ScopeGlobalUser("UNION-U1"), "UNION-U1", "小明", testICS, "global.ics", "U1",
		schedule.ImportSource{GroupOpenID: "GROUP-OLD", UserOpenID: "OLD-U1", UnionOpenID: "UNION-U1"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.SaveICSWithSource(
		schedule.ScopeGlobalUser("UNION-U2"), "UNION-U2", "小红", testICS, "global.ics", "U2",
		schedule.ImportSource{GroupOpenID: "GROUP", UserOpenID: "U2", UnionOpenID: "UNION-U2"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.SaveICS(
		env.Scope(message), "U3", "不在名单", testICS, "stale.ics", "U3",
	); err != nil {
		t.Fatal(err)
	}

	members, err := env.ScheduleMembers(context.Background(), message)
	if err != nil {
		t.Fatalf("ScheduleMembers: %v", err)
	}
	if len(members) != 2 ||
		members["U1"] == nil || members["U1"].Name != "小明" ||
		members["U2"] == nil || members["U2"].Name != "小红" ||
		members["U3"] != nil {
		t.Fatalf("roster projection = %+v", members)
	}
	if _, err := env.ScheduleMembers(context.Background(), message); err != nil {
		t.Fatalf("cached ScheduleMembers: %v", err)
	}
	if calls := fake.rosterCallCount(); calls != 1 {
		t.Fatalf("roster calls = %d, want 1", calls)
	}
}

func TestGlobalStorageKeepsCachedRosterOnPermissionError(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	enableGlobalStorage(t, env)
	scopeID := env.Scope(message)
	if err := env.Service.SaveGroupRoster(scopeID, []schedule.GroupRosterMember{{
		UserID:      message.UserOpenID,
		Name:        "小明",
		UnionOpenID: "UNION-U1",
	}}); err != nil {
		t.Fatalf("SaveGroupRoster: %v", err)
	}
	if _, err := env.Service.SaveICSWithSource(
		schedule.ScopeGlobalUser("UNION-U1"), "UNION-U1", "小明", testICS, "global.ics", "U1",
		schedule.ImportSource{GroupOpenID: "GROUP", UserOpenID: "U1", UnionOpenID: "UNION-U1"},
	); err != nil {
		t.Fatal(err)
	}
	fake.rosterError = 11253

	members, err := env.ScheduleMembers(context.Background(), message)
	if err != nil {
		t.Fatalf("ScheduleMembers: %v", err)
	}
	if member := members[message.UserOpenID]; member == nil || len(member.Events) == 0 {
		t.Fatalf("cached roster member = %+v", member)
	}
	roster, found, err := env.Service.GroupRoster(scopeID)
	if err != nil || !found || len(roster.Members) != 1 {
		t.Fatalf("cached roster = %+v found=%v err=%v", roster, found, err)
	}
}

func TestGlobalStorageTargetAlwaysUsesUnionWhenKnown(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	if _, err := env.Service.SaveICS(env.Scope(message), message.UserOpenID, "旧群课表", testICS, "legacy.ics", message.UserOpenID); err != nil {
		t.Fatal(err)
	}
	enableGlobalStorage(t, env)
	message.GlobalUserOpenID = "UNION-U1"

	target, err := env.ScheduleTarget(context.Background(), message, message.UserOpenID)
	if err != nil {
		t.Fatalf("ScheduleTarget: %v", err)
	}
	if target.ScopeID != schedule.ScopeGlobalUser("UNION-U1") || target.UserID != "UNION-U1" {
		t.Fatalf("global target = %+v", target)
	}
}

func TestIsolatedImportRecordsSourceAndUsesCurrentGroup(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	fake.memberUnionIDs[message.UserOpenID] = "UNION-U1"
	if err := env.ImportICS(context.Background(), message, NewReplier(message, env.Client), Attachment{
		URL:      icsServer.URL + "/schedule.ics",
		Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}

	members, err := env.Service.ScopeMembers(env.Scope(message))
	if err != nil {
		t.Fatalf("ScopeMembers: %v", err)
	}
	saved := members[message.UserOpenID]
	if saved == nil ||
		saved.ImportGroupOpenID != message.GroupOpenID ||
		saved.ImportUserOpenID != message.UserOpenID ||
		saved.ImportUnionOpenID != "UNION-U1" {
		t.Fatalf("saved source = %+v", saved)
	}
}

func TestGlobalImportRecordsSourceAndUsesUnionTarget(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	enableGlobalStorage(t, env)
	message.GlobalUserOpenID = "UNION-U1"
	if err := env.ImportICS(context.Background(), message, NewReplier(message, env.Client), Attachment{
		URL:      icsServer.URL + "/schedule.ics",
		Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}

	saved, err := env.Service.ScopeMembers(schedule.ScopeGlobalUser("UNION-U1"))
	if err != nil {
		t.Fatalf("ScopeMembers: %v", err)
	}
	member := saved["UNION-U1"]
	if member == nil ||
		member.ImportGroupOpenID != message.GroupOpenID ||
		member.ImportUserOpenID != message.UserOpenID ||
		member.ImportUnionOpenID != message.GlobalUserOpenID {
		t.Fatalf("saved source = %+v", member)
	}
}

func TestGlobalUserIDExplainsMissingUnionOpenID(t *testing.T) {
	tests := []struct {
		name      string
		errorCode int
		want      string
	}{
		{name: "empty union openid", want: "平台未返回 union_openid"},
		{name: "member API permission", errorCode: 11253, want: "群成员信息接口权限"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fake := newFakeQQ()
			fake.memberError = testCase.errorCode
			apiServer := httptest.NewServer(fake.handler())
			t.Cleanup(apiServer.Close)

			env, message := newTestEnv(t, fake, apiServer.URL)
			enableGlobalStorage(t, env)
			_, err := env.GlobalUserID(context.Background(), message)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want text %q", err, testCase.want)
			}
		})
	}
}
