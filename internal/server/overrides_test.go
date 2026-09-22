package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func listOverrides(t *testing.T, router *gin.Engine, scope string) []schedule.WebDayOverride {
	t.Helper()
	recorder := adminRequest(router, http.MethodGet, "/api/overrides?scope_id="+scope, nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list overrides = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Overrides []schedule.WebDayOverride `json:"overrides"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Overrides
}

func TestOverrideAPI(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	seedScope(t, service, scope)

	if rows := listOverrides(t, router, scope); len(rows) != 0 {
		t.Fatalf("initial overrides = %+v", rows)
	}

	// Holiday for one member and a shift for everyone.
	if recorder := adminRequest(router, http.MethodPost, "/api/overrides/set", map[string]any{
		"scope_id": scope, "user_id": "U1", "day": "2026-10-01", "kind": "holiday",
	}, true); recorder.Code != http.StatusOK {
		t.Fatalf("set holiday = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if recorder := adminRequest(router, http.MethodPost, "/api/overrides/set", map[string]any{
		"scope_id": scope, "user_id": "*", "day": "2026-10-11", "kind": "shift", "source_day": "2026-10-08",
	}, true); recorder.Code != http.StatusOK {
		t.Fatalf("set shift = %d (%s)", recorder.Code, recorder.Body.String())
	}

	rows := listOverrides(t, router, scope)
	if len(rows) != 2 {
		t.Fatalf("overrides = %+v", rows)
	}
	if rows[0].Day != "2026-10-01" || rows[0].Name != "小明" || rows[0].Kind != schedule.DayOverrideHoliday {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].Day != "2026-10-11" || rows[1].Name != "全体成员" || rows[1].SourceDay != "2026-10-08" {
		t.Fatalf("second row = %+v", rows[1])
	}

	// Validation failures.
	cases := []map[string]any{
		{"scope_id": scope, "user_id": "U1", "day": "2026-10-02", "kind": "party"},
		{"scope_id": scope, "user_id": "U1", "day": "2026-10-02", "kind": "shift"},
		{"scope_id": scope, "user_id": "U1", "day": "2026-10-02", "kind": "shift", "source_day": "2026-10-02"},
		{"scope_id": scope, "user_id": "UNKNOWN", "day": "2026-10-02", "kind": "holiday"},
		{"scope_id": scope, "user_id": "U1", "day": "not-a-day", "kind": "holiday"},
	}
	for index, body := range cases {
		if recorder := adminRequest(router, http.MethodPost, "/api/overrides/set", body, true); recorder.Code != http.StatusBadRequest {
			t.Fatalf("case %d = %d, want 400 (%s)", index, recorder.Code, recorder.Body.String())
		}
	}

	// The bot reply path sees the same markers.
	today, err := time.ParseInLocation("2006-01-02", "2026-09-17", schedule.LocalTZ)
	if err != nil {
		t.Fatal(err)
	}
	text, err := service.DayOverrideListText(scope, map[string]*schedule.Member{"U1": {UserID: "U1", Name: "小明"}}, today)
	if err != nil || !strings.Contains(text, "2026-10-01") || !strings.Contains(text, "2026-10-11") {
		t.Fatalf("list text = %q err=%v", text, err)
	}

	// Delete one marker; deleting again reports deleted=false.
	recorder := adminRequest(router, http.MethodPost, "/api/overrides/delete", map[string]any{
		"scope_id": scope, "user_id": "U1", "day": "2026-10-01",
	}, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &deleted); err != nil || !deleted.Deleted {
		t.Fatalf("delete payload = %s err=%v", recorder.Body.String(), err)
	}
	recorder = adminRequest(router, http.MethodPost, "/api/overrides/delete", map[string]any{
		"scope_id": scope, "user_id": "U1", "day": "2026-10-01",
	}, true)
	if err := json.Unmarshal(recorder.Body.Bytes(), &deleted); err != nil || deleted.Deleted {
		t.Fatalf("second delete payload = %s err=%v", recorder.Body.String(), err)
	}
	if rows := listOverrides(t, router, scope); len(rows) != 1 || rows[0].Day != "2026-10-11" {
		t.Fatalf("overrides after delete = %+v", rows)
	}
}
