package schedule_test

import (
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestBotSettingsDefaultsWhenUnset(t *testing.T) {
	service, _ := newService(t)
	settings, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if settings != schedule.DefaultSettings() {
		t.Fatalf("settings = %+v, want defaults %+v", settings, schedule.DefaultSettings())
	}
	if !settings.Enabled || !settings.ReplyPlain || !settings.ReplySlash || !settings.ReplyMention {
		t.Fatalf("defaults must all be on: %+v", settings)
	}
}

func TestBotSettingsRoundTrip(t *testing.T) {
	service, _ := newService(t)
	want := schedule.Settings{Enabled: false, ReplyPlain: true, ReplySlash: false, ReplyMention: true}
	if err := service.SaveBotSettings(want); err != nil {
		t.Fatalf("SaveBotSettings: %v", err)
	}
	got, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBotSettingsMissingFieldKeepsDefault(t *testing.T) {
	service, storeHandle := newService(t)
	// A stored document that only carries one field must not disable the rest.
	if err := storeHandle.SetKV("global", "settings", "bot", map[string]any{"enabled": false}); err != nil {
		t.Fatalf("SetKV: %v", err)
	}
	got, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if got.Enabled {
		t.Fatal("enabled should be false")
	}
	if !got.ReplyPlain || !got.ReplySlash || !got.ReplyMention {
		t.Fatalf("absent reply flags must stay on: %+v", got)
	}
}
