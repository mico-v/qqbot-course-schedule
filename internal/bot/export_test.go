package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestExportCommand(t *testing.T) {
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
	if err := env.ImportICS(ctx, freshMessage(base), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	// Export own schedule.
	msg := freshMessage(base)
	msg.Content = "/导出课表"
	handler.Dispatch(ctx, msg)
	reply := waitMessage(t, fake)
	if msgType, _ := reply["msg_type"].(float64); msgType != 7 {
		t.Fatalf("export reply = %+v", reply)
	}

	fake.mu.Lock()
	upload := fake.uploads[len(fake.uploads)-1]
	fake.mu.Unlock()
	if fileType, _ := upload["file_type"].(float64); fileType != 4 {
		t.Fatalf("file_type = %v, want 4", upload["file_type"])
	}
	url, _ := upload["url"].(string)
	if !strings.HasPrefix(url, "https://cards.example.com/files/schedule_") || !strings.HasSuffix(url, ".ics") {
		t.Fatalf("export url = %q", url)
	}
	if name, _ := upload["file_name"].(string); !strings.Contains(name, "课表.ics") {
		t.Fatalf("file_name = %q", upload["file_name"])
	}
	// The exported file is written to the public files directory.
	name := strings.TrimPrefix(url, "https://cards.example.com/files/")
	if _, err := filepath.Glob(filepath.Join(env.FilesDir, name)); err != nil {
		t.Fatalf("exported file missing: %v", err)
	}

	// An empty member has nothing to export.
	if _, err := env.Service.CreateMemberSchedules(env.Scope(base), []schedule.NewMember{{UserID: "U2", Name: "小红"}}, "test"); err != nil {
		t.Fatal(err)
	}
	empty := freshMessage(base)
	empty.UserOpenID = "U2"
	empty.Content = "/导出课表"
	handler.Dispatch(ctx, empty)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "还没有课程") {
		t.Fatalf("empty export reply = %q", content)
	}

	// A member cannot export someone else's schedule.
	other := freshMessage(base)
	other.Content = "/导出课表 小红"
	handler.Dispatch(ctx, other)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "普通成员只能操作自己") {
		t.Fatalf("permission reply = %q", content)
	}
}

func TestPushPausesOnPermissionError(t *testing.T) {
	fake := newFakeQQ()
	fake.failUploads = true
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()
	if err := env.ImportICS(ctx, freshMessage(base), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	owner := freshMessage(base)
	owner.MemberRole = "owner"
	owner.Content = "/启用推送"
	handler.Dispatch(ctx, owner)
	_ = waitMessage(t, fake)

	sent, _, failed := env.PushDaily(ctx)
	if sent != 0 || failed != 1 {
		t.Fatalf("PushDaily = %d/%d, want 0 sent / 1 failed", sent, failed)
	}
	subscriptions, err := env.PushSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	sub, ok := subscriptions[env.Scope(base)]
	if !ok || !sub.Paused || !strings.Contains(sub.PauseReason, "消息推送") {
		t.Fatalf("subscription = %+v", sub)
	}

	// A paused scope is skipped on the next run.
	if sent, _, failed := env.PushDaily(ctx); sent != 0 || failed != 0 {
		t.Fatalf("paused PushDaily = %d/%d", sent, failed)
	}
}
