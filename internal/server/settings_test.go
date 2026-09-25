package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestSettingsAPI(t *testing.T) {
	router, _ := newAdminRouter(t, adminPassword)

	// Defaults are returned before any customization.
	recorder := adminRequest(router, http.MethodGet, "/api/settings", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get settings = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var got struct {
		Settings schedule.Settings `json:"settings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Settings != schedule.DefaultSettings() {
		t.Fatalf("defaults = %+v", got.Settings)
	}

	// Save a mixed configuration and read it back.
	want := schedule.Settings{Enabled: false, ReplyPlain: true, ReplySlash: false, ReplyMention: true}
	if recorder := adminRequest(router, http.MethodPost, "/api/settings", want, true); recorder.Code != http.StatusOK {
		t.Fatalf("post settings = %d (%s)", recorder.Code, recorder.Body.String())
	}
	recorder = adminRequest(router, http.MethodGet, "/api/settings", nil, true)
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Settings != want {
		t.Fatalf("round-trip = %+v, want %+v", got.Settings, want)
	}

	// Authentication is required.
	if recorder := adminRequest(router, http.MethodGet, "/api/settings", nil, false); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d, want 401", recorder.Code)
	}
}
