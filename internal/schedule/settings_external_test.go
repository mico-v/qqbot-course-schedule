package schedule_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestBotSettingsDefaultsWhenUnset(t *testing.T) {
	service, _ := newService(t)
	settings, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if !slices.Equal(settings.AdminOpenIDs, schedule.DefaultSettings().AdminOpenIDs) ||
		settings.Enabled != schedule.DefaultSettings().Enabled ||
		settings.ReplyPlain != schedule.DefaultSettings().ReplyPlain ||
		settings.ReplySlash != schedule.DefaultSettings().ReplySlash ||
		settings.ReplyMention != schedule.DefaultSettings().ReplyMention ||
		settings.SendFormat != schedule.DefaultSettings().SendFormat ||
		settings.Nickname != schedule.DefaultSettings().Nickname ||
		settings.BaseURL != schedule.DefaultSettings().BaseURL ||
		settings.ScheduleLinkTTLMinutes != schedule.DefaultSettings().ScheduleLinkTTLMinutes ||
		settings.StorageMode != schedule.DefaultSettings().StorageMode {
		t.Fatalf("settings = %+v, want defaults %+v", settings, schedule.DefaultSettings())
	}
	if !settings.Enabled || !settings.ReplyPlain || !settings.ReplySlash || !settings.ReplyMention {
		t.Fatalf("defaults must all be on: %+v", settings)
	}
}

func TestBotSettingsRoundTrip(t *testing.T) {
	service, _ := newService(t)
	want := schedule.Settings{
		Enabled: false, ReplyPlain: true, ReplySlash: false, ReplyMention: true,
		SendFormat: schedule.SendFormatMarkdown,
		BaseURL:    "https://kb.example.com", ScheduleLinkTTLMinutes: 90,
		StorageMode:  schedule.StorageModeGlobal,
		AdminOpenIDs: []string{"ADMIN-LOCAL", "ADMIN-UNION"},
	}
	if err := service.SaveBotSettings(want); err != nil {
		t.Fatalf("SaveBotSettings: %v", err)
	}
	got, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if !slices.Equal(got.AdminOpenIDs, want.AdminOpenIDs) ||
		got.Enabled != want.Enabled || got.ReplyPlain != want.ReplyPlain ||
		got.ReplySlash != want.ReplySlash || got.ReplyMention != want.ReplyMention ||
		got.SendFormat != want.SendFormat || got.Nickname != want.Nickname ||
		got.BaseURL != want.BaseURL || got.ScheduleLinkTTLMinutes != want.ScheduleLinkTTLMinutes ||
		got.StorageMode != want.StorageMode {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBotSettingsNormalizesAdminOpenIDs(t *testing.T) {
	service, _ := newService(t)
	settings := schedule.DefaultSettings()
	settings.AdminOpenIDs = []string{" ADMIN-A ", "ADMIN-B, ADMIN-C", "ADMIN-A"}
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatalf("SaveBotSettings: %v", err)
	}
	got, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	want := []string{"ADMIN-A", "ADMIN-B", "ADMIN-C"}
	if !slices.Equal(got.AdminOpenIDs, want) {
		t.Fatalf("AdminOpenIDs = %v, want %v", got.AdminOpenIDs, want)
	}
}

func TestSettingsAllowsAdmin(t *testing.T) {
	settings := schedule.DefaultSettings()
	if !settings.AllowsAdmin("MEMBER", "", true) {
		t.Fatal("empty allowlist should preserve platform admin fallback")
	}
	if settings.AllowsAdmin("MEMBER", "", false) {
		t.Fatal("empty allowlist should reject a normal member")
	}

	settings.AdminOpenIDs = []string{"LOCAL-ADMIN", "UNION-ADMIN"}
	if !settings.AllowsAdmin("LOCAL-ADMIN", "", false) {
		t.Fatal("local OpenID in allowlist should be accepted")
	}
	if !settings.AllowsAdmin("OTHER", "UNION-ADMIN", false) {
		t.Fatal("global union OpenID in allowlist should be accepted")
	}
	if settings.AllowsAdmin("OWNER", "", true) {
		t.Fatal("configured allowlist must override the platform role fallback")
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
	if got.SendFormat != schedule.SendFormatImage {
		t.Fatalf("absent send format must default to image: %+v", got)
	}
}

func TestNicknameNormalization(t *testing.T) {
	if got := schedule.NormalizeNickname("  课表小助手  "); got != "课表小助手" {
		t.Fatalf("NormalizeNickname trim = %q", got)
	}
	if got := schedule.NormalizeNickname("   "); got != "" {
		t.Fatalf("blank nickname = %q, want empty", got)
	}
	long := strings.Repeat("字", schedule.MaxBotNicknameLength+5)
	if got := schedule.NormalizeNickname(long); len([]rune(got)) != schedule.MaxBotNicknameLength {
		t.Fatalf("NormalizeNickname length = %d, want %d", len([]rune(got)), schedule.MaxBotNicknameLength)
	}
}

func TestBotSettingsNicknameRoundTrip(t *testing.T) {
	service, storeHandle := newService(t)
	settings, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	settings.Nickname = "  课表小助手  "
	if err := service.SaveBotSettings(settings); err != nil {
		t.Fatalf("SaveBotSettings: %v", err)
	}
	got, err := service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if got.Nickname != "课表小助手" {
		t.Fatalf("nickname = %q, want 课表小助手", got.Nickname)
	}

	// A stored value longer than the cap is trimmed on read.
	if err := storeHandle.SetKV("global", "settings", "bot", map[string]any{
		"nickname": strings.Repeat("长", schedule.MaxBotNicknameLength+3),
	}); err != nil {
		t.Fatalf("SetKV: %v", err)
	}
	got, err = service.BotSettings()
	if err != nil {
		t.Fatalf("BotSettings: %v", err)
	}
	if len([]rune(got.Nickname)) != schedule.MaxBotNicknameLength {
		t.Fatalf("stored nickname length = %d", len([]rune(got.Nickname)))
	}
}
