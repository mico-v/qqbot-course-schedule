package qqapi

import (
	"context"
	"encoding/json"
	"errors"
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

func TestListGroupMembersPaginatesAndEscapesCursor(t *testing.T) {
	groupOpenID := "group/with?chars"
	wantPath := "/v2/groups/" + url.PathEscape(groupOpenID) + "/members"
	nextCursor := "cursor /+?&="
	calls := 0
	client := memberTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != wantPath {
			t.Errorf("path = %q, want %q", got, wantPath)
		}
		calls++
		switch calls {
		case 1:
			if cursor := r.URL.Query().Get("cursor"); cursor != "" {
				t.Errorf("first cursor = %q", cursor)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"members": []map[string]any{{
					"member_openid": "U1",
					"username":      "小明",
					"union_openid":  "UNION-1",
				}},
				"next_cursor": nextCursor,
			})
		case 2:
			if cursor := r.URL.Query().Get("cursor"); cursor != nextCursor {
				t.Errorf("second cursor = %q, want %q", cursor, nextCursor)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"members": []map[string]any{{
					"member_openid": "U2",
					"username":      "小红",
				}},
				"next_cursor": "",
			})
		default:
			t.Errorf("unexpected request %d", calls)
		}
	})

	members, err := client.ListGroupMembers(context.Background(), groupOpenID)
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(members) != 2 || members[0].MemberOpenID != "U1" ||
		members[0].UnionOpenID != "UNION-1" || members[1].MemberOpenID != "U2" {
		t.Fatalf("members = %+v", members)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestListGroupMembersRejectsRepeatedCursor(t *testing.T) {
	calls := 0
	client := memberTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"members":     []map[string]any{},
			"next_cursor": "same-cursor",
		})
	})

	_, err := client.ListGroupMembers(context.Background(), "GROUP")
	if err == nil || err.Error() != "群成员分页游标重复" {
		t.Fatalf("error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestListGroupMembersReturnsPermissionError(t *testing.T) {
	client := memberTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    11253,
			"message": "应用无接口访问权限",
		})
	})

	_, err := client.ListGroupMembers(context.Background(), "GROUP")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 11253 {
		t.Fatalf("error = %v", err)
	}
}
