package bot

import (
	"context"
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
	env := &Env{Service: schedule.NewService(storeHandle)}
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

func TestHandlerAdminAllowlist(t *testing.T) {
	env, handler := newSettingsEnv(t)
	settings := schedule.DefaultSettings()
	settings.AdminOpenIDs = []string{"LOCAL-ADMIN", "GLOBAL-ADMIN"}
	if err := env.Service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		in   *Inbound
		want bool
	}{
		{"local openid", &Inbound{UserOpenID: "LOCAL-ADMIN"}, true},
		{"union openid", &Inbound{UserOpenID: "LOCAL-MEMBER", GlobalUserOpenID: "GLOBAL-ADMIN"}, true},
		{"platform owner absent from list", &Inbound{UserOpenID: "OWNER", MemberRole: "owner"}, false},
		{"normal member", &Inbound{UserOpenID: "MEMBER", MemberRole: "member"}, false},
	}
	for _, testCase := range cases {
		if got := handler.isAdmin(testCase.in); got != testCase.want {
			t.Errorf("%s: isAdmin = %v, want %v", testCase.name, got, testCase.want)
		}
	}
}

func TestDispatchRespectsEnabledSwitch(t *testing.T) {
	env, handler := newSettingsEnv(t)
	calls := 0
	handler.Register(&Command{Prefix: "/课表", Handle: func(context.Context, *Inbound, *Replier) error {
		calls++
		return nil
	}})

	off := schedule.DefaultSettings()
	off.Enabled = false
	if err := env.Service.SaveBotSettings(off); err != nil {
		t.Fatal(err)
	}
	dispatch(context.Background(), nil, handler, &Inbound{Content: "/课表"})
	if calls != 0 {
		t.Fatalf("calls = %d, want 0 while disabled", calls)
	}

	if err := env.Service.SaveBotSettings(schedule.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	dispatch(context.Background(), nil, handler, &Inbound{Content: "/课表"})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 after enabling", calls)
	}
}

func TestDispatchRespectsReplyPolicy(t *testing.T) {
	env, handler := newSettingsEnv(t)
	var got []string
	handler.Register(&Command{Prefix: "/课表", Handle: func(_ context.Context, msg *Inbound, _ *Replier) error {
		got = append(got, msg.Content)
		return nil
	}})

	settings := schedule.DefaultSettings()
	settings.ReplyPlain = false
	settings.ReplySlash = false
	if err := env.Service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}

	dispatch(context.Background(), nil, handler, &Inbound{Content: "课表"})
	dispatch(context.Background(), nil, handler, &Inbound{Content: "/课表"})
	dispatch(context.Background(), nil, handler, &Inbound{Content: "<@BOTOPENID> /课表"})

	if len(got) != 1 || got[0] != "/课表" {
		t.Fatalf("got = %v, want a single mention call with content %q", got, "/课表")
	}
}
