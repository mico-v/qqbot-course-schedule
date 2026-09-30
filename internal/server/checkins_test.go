package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func listCheckins(t *testing.T, router *gin.Engine, scope string) admin.WebCheckinBoard {
	t.Helper()
	recorder := adminRequest(router, http.MethodGet, "/api/checkins?scope_id="+scope, nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list checkins = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var board admin.WebCheckinBoard
	if err := json.Unmarshal(recorder.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	return board
}

func TestCheckinAPI(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	seedScope(t, service, scope)

	day := time.Date(2026, 9, 17, 9, 0, 0, 0, schedule.LocalTZ)
	for _, checkin := range []struct {
		user   string
		name   string
		day    time.Time
		points int
	}{
		{"U1", "小明", day, 6},
		{"U1", "小明", day.AddDate(0, 0, 1), 10},
		{"U2", "小红", day, 3},
	} {
		if _, err := service.Checkin(scope, checkin.user, checkin.name, checkin.day, checkin.points); err != nil {
			t.Fatalf("Checkin %s: %v", checkin.user, err)
		}
	}

	board := listCheckins(t, router, scope)
	if len(board.Totals) != 2 {
		t.Fatalf("totals = %+v", board.Totals)
	}
	if board.Totals[0].UserID != "U1" || board.Totals[0].Points != 16 || board.Totals[0].Days != 2 {
		t.Fatalf("first total = %+v", board.Totals[0])
	}
	if board.Totals[0].Name != "小明" || board.Totals[1].Name != "小红" {
		t.Fatalf("display names = %+v", board.Totals)
	}
	if len(board.Records) != 3 || board.Records[0].Day != "2026-09-18" || board.Records[0].Points != 10 {
		t.Fatalf("records = %+v", board.Records)
	}

	// Missing scope and malformed payloads are rejected.
	if recorder := adminRequest(router, http.MethodGet, "/api/checkins", nil, true); recorder.Code != http.StatusBadRequest {
		t.Fatalf("no-scope checkins = %d", recorder.Code)
	}
	if recorder := adminRequest(router, http.MethodPost, "/api/checkins/delete", map[string]any{
		"scope_id": scope, "user_id": "U1", "day": "17-09-2026",
	}, true); recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad day delete = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// Deleting one day drops its points from the totals.
	recorder := adminRequest(router, http.MethodPost, "/api/checkins/delete", map[string]any{
		"scope_id": scope, "user_id": "U1", "day": "2026-09-18",
	}, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete = %d (%s)", recorder.Code, recorder.Body.String())
	}
	board = listCheckins(t, router, scope)
	if board.Totals[0].Points != 6 || board.Totals[0].Days != 1 {
		t.Fatalf("totals after delete = %+v", board.Totals[0])
	}

	// Deleting again reports deleted=false instead of failing.
	recorder = adminRequest(router, http.MethodPost, "/api/checkins/delete", map[string]any{
		"scope_id": scope, "user_id": "U1", "day": "2026-09-18",
	}, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second delete = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Deleted bool `json:"deleted"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Deleted {
		t.Fatal("second delete reported deleted=true")
	}

	if recorder := adminRequest(router, http.MethodGet, "/api/checkins?scope_id="+scope, nil, false); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d, want 401", recorder.Code)
	}
}
