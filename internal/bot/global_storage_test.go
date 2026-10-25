package bot

import (
	"context"
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
	if calls := fake.memberCallCount(); calls != 0 {
		t.Fatalf("member API calls = %d, want 0", calls)
	}
}

func TestGlobalStorageFindsLegacyScheduleFromAnotherGroupWithoutUnionOpenID(t *testing.T) {
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
	if member := members[message.UserOpenID]; member == nil || member.Name != "旧群课表" || len(member.Events) == 0 {
		t.Fatalf("fallback member = %+v", member)
	}
	if calls := fake.memberCallCount(); calls != 0 {
		t.Fatalf("member API calls = %d, want 0", calls)
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
