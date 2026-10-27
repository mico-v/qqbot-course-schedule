package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestPublicScheduleEditorIsTokenScoped(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminTestICS, "u1.ics", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveICS(scope, "U2", "小红", adminTestICS, "u2.ics", "test"); err != nil {
		t.Fatal(err)
	}
	token, _, err := service.CreateScheduleEditLink(scope, "U1", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	pageRecorder := adminRequest(router, http.MethodGet, "/schedule/edit", nil, false)
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("public page = %d (%s)", pageRecorder.Code, pageRecorder.Body.String())
	}

	recorder := adminRequest(router, http.MethodGet, "/api/public/schedule?token="+token, nil, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("public get = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var page map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page["name"] != "小明" || page["scope_id"] != nil || page["user_id"] != nil {
		t.Fatalf("public page leaked identity: %+v", page)
	}

	revision := int64(1)
	payload := map[string]any{
		"scope_id": "group:OTHER",
		"user_id":  "U2",
		"revision": revision,
		"name":     "数学课代表",
		"events": []admin.WebEventInput{{
			ID: 1, UID: "math-1", Course: "高等数学A",
			Start: "2026-09-01T08:00", End: "2026-09-01T09:30",
		}},
	}
	recorder = adminRequest(router, http.MethodPost, "/api/public/schedule/save?token="+token, payload, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("public save = %d (%s)", recorder.Code, recorder.Body.String())
	}

	updated, found, err := service.PageSchedule(scope, "U1")
	if err != nil || !found {
		t.Fatalf("load U1 found=%v err=%v", found, err)
	}
	if updated.Name != "数学课代表" || len(updated.Events) != 1 || updated.Events[0].Course != "高等数学A" {
		t.Fatalf("updated U1 = %+v", updated)
	}
	other, found, err := service.PageSchedule(scope, "U2")
	if err != nil || !found {
		t.Fatalf("load U2 found=%v err=%v", found, err)
	}
	if other.Name != "小红" || other.Revision != 1 {
		t.Fatalf("token save escaped its member: %+v", other)
	}

	// Admin APIs still require their session.
	if recorder := adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U1", nil, false); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin API = %d, want 401", recorder.Code)
	}
}

func TestPublicScheduleEditorRejectsInvalidAndExpiredTokens(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminTestICS, "u1.ics", "test"); err != nil {
		t.Fatal(err)
	}
	token, _, err := service.CreateScheduleEditLink(scope, "U1", time.Minute, time.Now().Add(-2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if recorder := adminRequest(router, http.MethodGet, "/api/public/schedule?token="+token, nil, false); recorder.Code != http.StatusGone {
		t.Fatalf("expired token = %d (%s), want 410", recorder.Code, recorder.Body.String())
	}
	if recorder := adminRequest(router, http.MethodGet, "/api/public/schedule?token=invalid", nil, false); recorder.Code != http.StatusNotFound {
		t.Fatalf("invalid token = %d (%s), want 404", recorder.Code, recorder.Body.String())
	}
}
