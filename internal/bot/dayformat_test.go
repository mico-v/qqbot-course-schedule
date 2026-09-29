package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// setupMarkdownEnv imports one schedule and switches the bot to markdown.
func setupMarkdownEnv(t *testing.T) (*fakeQQ, *Env, *Handler, *Inbound) {
	t.Helper()
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()
	freshImport := freshMessage(base)
	if err := env.ImportICS(ctx, freshImport, NewReplier(freshImport, env.Client), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	settings := schedule.DefaultSettings()
	settings.SendFormat = schedule.SendFormatMarkdown
	if err := env.Service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	return fake, env, handler, base
}

func TestDayCardMarkdownFormat(t *testing.T) {
	fake, env, handler, base := setupMarkdownEnv(t)
	msg := freshMessage(base)
	msg.Content = "/今日课表"
	dispatch(context.Background(), env, handler, msg)

	card := waitMessage(t, fake)
	if msgType, _ := card["msg_type"].(float64); msgType != 2 {
		t.Fatalf("message = %+v, want msg_type 2", card)
	}
	markdown, _ := card["markdown"].(map[string]any)
	content, _ := markdown["content"].(string)
	if !strings.Contains(content, "**小明**") || !strings.Contains(content, "高等数学") {
		t.Fatalf("markdown content = %q", content)
	}
}

func TestDayCardMarkdownFallsBackToText(t *testing.T) {
	fake, env, handler, base := setupMarkdownEnv(t)
	fake.failMarkdown = true
	msg := freshMessage(base)
	msg.Content = "/今日课表"
	dispatch(context.Background(), env, handler, msg)

	card := waitMessage(t, fake)
	if msgType, _ := card["msg_type"].(float64); msgType != 0 {
		t.Fatalf("fallback message = %+v, want msg_type 0", card)
	}
	content, _ := card["content"].(string)
	if !strings.Contains(content, "小明") || !strings.Contains(content, "高等数学") || strings.Contains(content, "**") {
		t.Fatalf("fallback content = %q", content)
	}
}
