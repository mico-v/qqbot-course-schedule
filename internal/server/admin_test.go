package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

const adminTestICS = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:math-1
SUMMARY:高等数学
DTSTART;TZID=Asia/Shanghai:20260901T080000
DTEND;TZID=Asia/Shanghai:20260901T093000
END:VEVENT
END:VCALENDAR
`

const adminPassword = "test-password"

func newAdminRouter(t *testing.T, password string) (*gin.Engine, *schedule.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })
	service := schedule.NewService(storeHandle)
	router := gin.New()
	RegisterAdmin(router, service, password)
	return router, service
}

func adminRequest(router *gin.Engine, method, path string, body any, withAuth bool) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	if withAuth {
		req.SetBasicAuth("admin", adminPassword)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestAdminRequiresPassword(t *testing.T) {
	router, _ := newAdminRouter(t, adminPassword)

	if recorder := adminRequest(router, http.MethodGet, "/admin", nil, false); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /admin = %d, want 401", recorder.Code)
	}
	recorder := adminRequest(router, http.MethodGet, "/admin", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated /admin = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("课表管理")) {
		t.Fatal("admin page body missing title")
	}
	if recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, false); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/scopes = %d, want 401", recorder.Code)
	}
}

func TestAdminLoopbackOnlyWithoutPassword(t *testing.T) {
	router, _ := newAdminRouter(t, "")

	remote := httptest.NewRequest(http.MethodGet, "/api/scopes", nil)
	remote.RemoteAddr = "192.0.2.10:12345"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, remote)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("remote request = %d, want 403", recorder.Code)
	}

	if recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, false); recorder.Code != http.StatusOK {
		t.Fatalf("loopback request = %d, want 200", recorder.Code)
	}
}

func TestAdminAPIFlow(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminTestICS, "schedule.ics", "U1"); err != nil {
		t.Fatalf("SaveICS: %v", err)
	}

	// scopes
	recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("scopes = %d", recorder.Code)
	}
	var scopesResponse struct {
		Scopes []struct {
			ScopeID     string `json:"scope_id"`
			Kind        string `json:"kind"`
			MemberCount int    `json:"member_count"`
			EventCount  int    `json:"event_count"`
			Members     []struct {
				UserID     string `json:"user_id"`
				EventCount int    `json:"event_count"`
				Revision   int64  `json:"revision"`
			} `json:"members"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &scopesResponse); err != nil {
		t.Fatalf("decode scopes: %v", err)
	}
	if len(scopesResponse.Scopes) != 1 || scopesResponse.Scopes[0].Kind != "group" ||
		scopesResponse.Scopes[0].MemberCount != 1 || scopesResponse.Scopes[0].EventCount != 1 {
		t.Fatalf("scopes = %+v", scopesResponse)
	}

	// schedule
	recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U1", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("schedule = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var page schedule.PageSchedule
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if page.Revision != 1 || len(page.Events) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U9", nil, true); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing member = %d, want 404", recorder.Code)
	}

	// save
	saveBody := map[string]any{
		"scope_id": scope, "user_id": "U1", "revision": page.Revision, "name": "小明",
		"events": []map[string]any{{
			"id": 1, "uid": "math-1", "course": "高等数学A",
			"start": "2026-09-01T08:00", "end": "2026-09-01T09:30",
		}},
	}
	recorder = adminRequest(router, http.MethodPost, "/api/schedule/save", saveBody, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save = %d (%s)", recorder.Code, recorder.Body.String())
	}
	// stale revision -> 409
	recorder = adminRequest(router, http.MethodPost, "/api/schedule/save", saveBody, true)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale save = %d, want 409 (%s)", recorder.Code, recorder.Body.String())
	}

	// members: only observed members without a schedule
	if recorder = adminRequest(router, http.MethodGet, "/api/members?scope_id="+scope, nil, true); recorder.Code != http.StatusOK {
		t.Fatalf("members = %d", recorder.Code)
	}
	var membersResponse struct {
		Members     []schedule.NewMember `json:"members"`
		MemberCount int                  `json:"member_count"`
		Note        string               `json:"note"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &membersResponse); err != nil {
		t.Fatal(err)
	}
	if membersResponse.MemberCount != 0 || membersResponse.Note == "" {
		t.Fatalf("members = %+v", membersResponse)
	}

	if err := service.RecordSeenMember(scope, "U2", "小红"); err != nil {
		t.Fatal(err)
	}
	recorder = adminRequest(router, http.MethodGet, "/api/members?scope_id="+scope, nil, true)
	if err := json.Unmarshal(recorder.Body.Bytes(), &membersResponse); err != nil {
		t.Fatal(err)
	}
	if membersResponse.MemberCount != 1 || membersResponse.Members[0].UserID != "U2" {
		t.Fatalf("members after seen = %+v", membersResponse)
	}

	// create an empty schedule for the observed member
	createBody := map[string]any{
		"scope_id": scope,
		"members":  []map[string]string{{"user_id": "U2", "name": "小红"}},
	}
	recorder = adminRequest(router, http.MethodPost, "/api/schedule/create", createBody, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var createResponse struct {
		CreatedCount int `json:"created_count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &createResponse); err != nil {
		t.Fatal(err)
	}
	if createResponse.CreatedCount != 1 {
		t.Fatalf("create response = %+v", createResponse)
	}
	recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U2", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("created member schedule = %d", recorder.Code)
	}
}
