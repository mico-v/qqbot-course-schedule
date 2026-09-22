package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBindQQCommand(t *testing.T) {
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
	scope := env.Scope(base)

	// Without a schedule the command explains what to do.
	msg := freshMessage(base)
	msg.Content = "/绑定QQ 123456789"
	handler.Dispatch(ctx, msg)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "还没有课表") {
		t.Fatalf("reply = %q", content)
	}

	if err := env.ImportICS(ctx, freshMessage(base), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	// Show, bind, show again.
	show := freshMessage(base)
	show.Content = "/绑定QQ"
	handler.Dispatch(ctx, show)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "未绑定") {
		t.Fatalf("show reply = %q", content)
	}

	bind := freshMessage(base)
	bind.Content = "/绑定QQ 123456789"
	handler.Dispatch(ctx, bind)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已绑定 QQ 123456789") {
		t.Fatalf("bind reply = %q", content)
	}
	bindings, err := env.Service.MemberQQBindings(scope)
	if err != nil || bindings["U1"] != "123456789" {
		t.Fatalf("bindings = %+v err=%v", bindings, err)
	}

	invalid := freshMessage(base)
	invalid.Content = "/绑定QQ 12ab"
	handler.Dispatch(ctx, invalid)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "绑定失败") {
		t.Fatalf("invalid reply = %q", content)
	}

	unbind := freshMessage(base)
	unbind.Content = "/解绑QQ"
	handler.Dispatch(ctx, unbind)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已解除 QQ 绑定") {
		t.Fatalf("unbind reply = %q", content)
	}
}

func TestMemberAvatarsFetchAndFallback(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	var hits atomic.Int32
	avatarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.Contains(r.URL.Path, "fail") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(testPNG(t))
	}))
	t.Cleanup(avatarServer.Close)
	restore := qqAvatarURL
	qqAvatarURL = func(qq string) string { return avatarServer.URL + "/" + qq + ".png" }
	t.Cleanup(func() { qqAvatarURL = restore })

	env, base := newTestEnv(t, fake, apiServer.URL)
	ctx := context.Background()
	scope := env.Scope(base)
	if _, err := env.Service.SaveICS(scope, "U1", "小明", testICS, "s.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.SaveICS(scope, "U2", "小红", testICS, "s.ics", "U1"); err != nil {
		t.Fatal(err)
	}

	// Without bindings nothing is fetched.
	if avatars := env.MemberAvatars(ctx, scope, []string{"U1", "U2"}); avatars != nil {
		t.Fatalf("avatars without bindings = %v", avatars)
	}
	if hits.Load() != 0 {
		t.Fatalf("unexpected fetches: %d", hits.Load())
	}

	if _, err := env.Service.SetMemberQQ(scope, "U1", "123456789", "U1"); err != nil {
		t.Fatal(err)
	}
	avatars := env.MemberAvatars(ctx, scope, []string{"U1", "U2"})
	if avatars["U1"] == nil || avatars["U2"] != nil {
		t.Fatalf("avatars = %v", avatars)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", hits.Load())
	}
	// A second call is served from the in-memory cache.
	env.MemberAvatars(ctx, scope, []string{"U1"})
	if hits.Load() != 1 {
		t.Fatalf("hits after cache = %d, want 1", hits.Load())
	}

	// A failing fetch is cached as a miss and returns no avatar.
	if _, err := env.Service.SetMemberQQ(scope, "U2", "987654321", "U2"); err != nil {
		t.Fatal(err)
	}
	qqAvatarURL = func(qq string) string { return avatarServer.URL + "/fail-" + qq + ".png" }
	if avatars := env.MemberAvatars(ctx, scope, []string{"U1", "U2"}); avatars["U2"] != nil {
		t.Fatalf("failed avatar should be absent: %v", avatars)
	}
	before := hits.Load()
	env.MemberAvatars(ctx, scope, []string{"U2"})
	if hits.Load() != before {
		t.Fatalf("failure was not cached: %d -> %d", before, hits.Load())
	}
}
