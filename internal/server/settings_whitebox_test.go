package server

import (
	"encoding/json"
	"net/http"
	"slices"
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
	if len(got.Settings.AdminOpenIDs) != 0 ||
		got.Settings.Enabled != schedule.DefaultSettings().Enabled ||
		got.Settings.ReplyPlain != schedule.DefaultSettings().ReplyPlain ||
		got.Settings.ReplySlash != schedule.DefaultSettings().ReplySlash ||
		got.Settings.ReplyMention != schedule.DefaultSettings().ReplyMention ||
		got.Settings.SendFormat != schedule.DefaultSettings().SendFormat ||
		got.Settings.Nickname != schedule.DefaultSettings().Nickname ||
		got.Settings.BaseURL != schedule.DefaultSettings().BaseURL ||
		got.Settings.ScheduleLinkTTLMinutes != schedule.DefaultSettings().ScheduleLinkTTLMinutes {
		t.Fatalf("defaults = %+v", got.Settings)
	}

	// Save a mixed configuration and read it back.
	want := schedule.Settings{
		Enabled: false, ReplyPlain: true, ReplySlash: false, ReplyMention: true,
		SendFormat: schedule.SendFormatMarkdown, Nickname: "课表小助手",
		BaseURL: "https://kb.example.com", ScheduleLinkTTLMinutes: 90,
		AdminOpenIDs: []string{"ADMIN-1"},
	}
	if recorder := adminRequest(router, http.MethodPost, "/api/settings", want, true); recorder.Code != http.StatusOK {
		t.Fatalf("post settings = %d (%s)", recorder.Code, recorder.Body.String())
	}
	recorder = adminRequest(router, http.MethodGet, "/api/settings", nil, true)
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Settings.AdminOpenIDs, want.AdminOpenIDs) ||
		got.Settings.Enabled != want.Enabled || got.Settings.ReplyPlain != want.ReplyPlain ||
		got.Settings.ReplySlash != want.ReplySlash || got.Settings.ReplyMention != want.ReplyMention ||
		got.Settings.SendFormat != want.SendFormat || got.Settings.Nickname != want.Nickname ||
		got.Settings.BaseURL != want.BaseURL ||
		got.Settings.ScheduleLinkTTLMinutes != want.ScheduleLinkTTLMinutes {
		t.Fatalf("round-trip = %+v, want %+v", got.Settings, want)
	}

	// Authentication is required.
	if recorder := adminRequest(router, http.MethodGet, "/api/settings", nil, false); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d, want 401", recorder.Code)
	}

	invalid := want
	invalid.BaseURL = "not-a-url"
	recorder = adminRequest(router, http.MethodPost, "/api/settings", invalid, true)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid settings = %d (%s), want 400", recorder.Code, recorder.Body.String())
	}
	var errorBody struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &errorBody); err != nil {
		t.Fatal(err)
	}
	if errorBody.Code != "validation" {
		t.Fatalf("error code = %q, want validation", errorBody.Code)
	}
}
