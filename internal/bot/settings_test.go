package bot

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func newSettingsEnv(t *testing.T) (*Env, *Handler) {
	t.Helper()
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })
	env := &Env{Store: storeHandle, Service: schedule.NewService(storeHandle)}
	handler := NewHandler()
	handler.env = env
	return env, handler
}

func TestClassifyReplyMode(t *testing.T) {
	cases := []struct {
		mentioned bool
		slash     bool
		want      replyMode
	}{
		{false, false, replyModePlain},
		{false, true, replyModeSlash},
		{true, true, replyModeMention},
		{true, false, replyModeMention},
	}
	for _, tc := range cases {
		if got := classifyReplyMode(tc.mentioned, tc.slash); got != tc.want {
			t.Errorf("classify(%v,%v) = %q, want %q", tc.mentioned, tc.slash, got, tc.want)
		}
	}
}

func TestAllowsReply(t *testing.T) {
	settings := schedule.Settings{Enabled: true, ReplyPlain: false, ReplySlash: true, ReplyMention: false}
	cases := map[replyMode]bool{
		replyModePlain:   false,
		replyModeSlash:   true,
		replyModeMention: false,
	}
	for mode, want := range cases {
		if got := allowsReply(settings, mode); got != want {
			t.Errorf("allowsReply(%q) = %v, want %v", mode, got, want)
		}
	}
}

func TestParseOnOff(t *testing.T) {
	for _, value := range []string{"开", "on", "TRUE", "1", "yes", "启用"} {
		if got, ok := parseOnOff(value); !ok || !got {
			t.Errorf("parseOnOff(%q) = %v,%v want true,true", value, got, ok)
		}
	}
	for _, value := range []string{"关", "off", "false", "0", "no", "禁用"} {
		if got, ok := parseOnOff(value); !ok || got {
			t.Errorf("parseOnOff(%q) = %v,%v want false,true", value, got, ok)
		}
	}
	if _, ok := parseOnOff("也许"); ok {
		t.Error("parseOnOff accepted an invalid value")
	}
}

func TestApplySettingArgs(t *testing.T) {
	base := schedule.DefaultSettings()

	updated, problem := applySettingArgs(base, []string{"机器人", "关"})
	if problem != "" || updated.Enabled {
		t.Fatalf("机器人 关 => %+v problem=%q", updated, problem)
	}

	updated, problem = applySettingArgs(base, []string{"关"})
	if problem != "" || updated.Enabled {
		t.Fatalf("关 => %+v problem=%q", updated, problem)
	}

	updated, problem = applySettingArgs(base, []string{"回复", "斜杠", "关"})
	if problem != "" || updated.ReplySlash || !updated.ReplyPlain {
		t.Fatalf("回复 斜杠 关 => %+v problem=%q", updated, problem)
	}

	updated, problem = applySettingArgs(base, []string{"回复", "全部", "关"})
	if problem != "" || updated.ReplyPlain || updated.ReplySlash || updated.ReplyMention {
		t.Fatalf("回复 全部 关 => %+v problem=%q", updated, problem)
	}

	if _, problem = applySettingArgs(base, []string{"回复", "斜杠"}); problem == "" {
		t.Error("missing value should return usage")
	}
	if _, problem = applySettingArgs(base, []string{"乱写"}); problem == "" {
		t.Error("unknown token should return usage")
	}
}

func TestDispatchRespectsEnabledSwitch(t *testing.T) {
	env, handler := newSettingsEnv(t)
	calls := 0
	handler.Register(&Command{Prefix: "/课表", Handle: func(context.Context, *Message) error {
		calls++
		return nil
	}})

	off := schedule.DefaultSettings()
	off.Enabled = false
	if err := env.Service.SaveBotSettings(off); err != nil {
		t.Fatal(err)
	}
	handler.Dispatch(context.Background(), &Message{Content: "/课表"})
	if calls != 0 {
		t.Fatalf("calls = %d, want 0 while disabled", calls)
	}

	if err := env.Service.SaveBotSettings(schedule.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	handler.Dispatch(context.Background(), &Message{Content: "/课表"})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 after enabling", calls)
	}
}

func TestDispatchRespectsReplyPolicy(t *testing.T) {
	env, handler := newSettingsEnv(t)
	var got []string
	handler.Register(&Command{Prefix: "/课表", Handle: func(_ context.Context, msg *Message) error {
		got = append(got, msg.Content)
		return nil
	}})

	settings := schedule.DefaultSettings()
	settings.ReplyPlain = false
	settings.ReplySlash = false
	if err := env.Service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}

	handler.Dispatch(context.Background(), &Message{Content: "课表"})
	handler.Dispatch(context.Background(), &Message{Content: "/课表"})
	handler.Dispatch(context.Background(), &Message{Content: "<@BOTOPENID> /课表"})

	if len(got) != 1 || got[0] != "/课表" {
		t.Fatalf("got = %v, want a single mention call with content %q", got, "/课表")
	}
}

func TestDispatchAdminSettingsBypassesDisabled(t *testing.T) {
	env, handler := newSettingsEnv(t)
	called := 0
	handler.Register(&Command{Prefix: settingsCommandPrefix, Handle: func(context.Context, *Message) error {
		called++
		return nil
	}})

	off := schedule.DefaultSettings()
	off.Enabled = false
	if err := env.Service.SaveBotSettings(off); err != nil {
		t.Fatal(err)
	}

	handler.Dispatch(context.Background(), &Message{Content: "/设置", MemberRole: "member"})
	if called != 0 {
		t.Fatal("a plain member reached settings while the bot is off")
	}
	handler.Dispatch(context.Background(), &Message{Content: "/设置", MemberRole: "admin"})
	if called != 1 {
		t.Fatal("an admin must be able to manage settings while the bot is off")
	}
}

func TestSettingsCommandUpdatesSwitches(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	message.MemberRole = "admin"
	message.Args = "机器人 关"
	handler := NewDefaultHandler(env)
	if err := handler.handleSettings(context.Background(), message); err != nil {
		t.Fatalf("handleSettings: %v", err)
	}
	settings, err := env.Service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled {
		t.Fatalf("settings = %+v, want enabled=false", settings)
	}
}
