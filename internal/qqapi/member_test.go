package qqapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
)

func memberTestClient(t *testing.T, memberHandler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/getAppAccessToken" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token",
				"expires_in":   "7200",
			})
			return
		}
		memberHandler(w, r)
	}))
	t.Cleanup(server.Close)
	return New(&config.Config{
		AppID:         "10000",
		Secret:        "secret",
		Domain:        server.URL,
		TokenEndpoint: server.URL + "/app/getAppAccessToken",
	})
}

func TestGetGroupMemberEscapesPathAndDecodesUnionOpenID(t *testing.T) {
	groupOpenID := "group/id"
	memberOpenID := "member?id"
	client := memberTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v2/groups/" + url.PathEscape(groupOpenID) + "/members/" + url.PathEscape(memberOpenID)
		if got := r.URL.EscapedPath(); got != wantPath {
			t.Errorf("path = %q, want %q", got, wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "QQBot test-token" {
			t.Errorf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"member_openid": memberOpenID,
			"username":      "小明",
			"member_role":   "admin",
			"bot":           false,
			"union_openid":  "UNION-1",
		})
	})

	member, err := client.GetGroupMember(context.Background(), groupOpenID, memberOpenID)
	if err != nil {
		t.Fatalf("GetGroupMember: %v", err)
	}
	if member.MemberOpenID != memberOpenID || member.Username != "小明" ||
		member.MemberRole != "admin" || member.UnionOpenID != "UNION-1" {
		t.Fatalf("member = %+v", member)
	}
}

func TestGetGroupMemberReturnsPermissionError(t *testing.T) {
	client := memberTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    11253,
			"message": "应用无接口访问权限",
		})
	})

	_, err := client.GetGroupMember(context.Background(), "GROUP", "U1")
	if err == nil {
		t.Fatal("GetGroupMember should fail without member API permission")
	}
	if code := ErrorCode(err); code != 11253 {
		t.Fatalf("error code = %d, want 11253 (%v)", code, err)
	}
}
