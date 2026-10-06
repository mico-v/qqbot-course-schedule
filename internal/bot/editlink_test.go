package bot

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestEditScheduleCommandCreatesScopedLink(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	settings := schedule.DefaultSettings()
	settings.BaseURL = "https://kb.example.com/service"
	settings.ScheduleLinkTTLMinutes = 15
	if err := env.Service.SaveBotSettings(settings); err != nil {
		t.Fatal(err)
	}
	handler := NewDefaultHandler(env)
	message.Content = "/修改课程表"
	message.Args = ""
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "课程表修改链接") {
		t.Fatalf("reply = %q", content)
	}
	var rawURL string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "/schedule/edit?token=") {
			rawURL = strings.TrimSpace(line)
			break
		}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "kb.example.com" || parsed.Path != "/service/schedule/edit" {
		t.Fatalf("edit URL = %q err=%v", rawURL, err)
	}
	token := parsed.Query().Get("token")
	link, err := env.Service.ResolveScheduleEditLink(token, env.now())
	if err != nil {
		t.Fatalf("ResolveScheduleEditLink: %v", err)
	}
	if link.ScopeID != env.Scope(message) || link.UserID != message.UserOpenID {
		t.Fatalf("link = %+v", link)
	}
	if !link.ExpiresAt.Equal(env.now().Add(15 * time.Minute)) {
		t.Fatalf("expiresAt = %v", link.ExpiresAt)
	}
	members, err := env.Service.ScopeMembers(env.Scope(message))
	if err != nil || members[message.UserOpenID] == nil {
		t.Fatalf("sender schedule was not created: %+v err=%v", members, err)
	}
}

func TestEditScheduleCommandRequiresBaseURL(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	env.PublicBaseURL = ""
	handler := NewDefaultHandler(env)
	message.Content = "/修改课程表"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "尚未配置服务回调地址") {
		t.Fatalf("reply = %q", content)
	}
}
