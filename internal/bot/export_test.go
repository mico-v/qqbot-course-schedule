package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
	freshImport := freshMessage(base)
	if err := env.ImportICS(ctx, freshImport, NewReplier(freshImport, env.Client), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	// Export own schedule.
	msg := freshMessage(base)
	msg.Content = "/导出课表"
	dispatch(ctx, env, handler, msg)
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
	if err := env.Service.EnsureMember(env.Scope(base), "U2", "小红"); err != nil {
		t.Fatal(err)
	}
	empty := freshMessage(base)
	empty.UserOpenID = "U2"
	empty.Content = "/导出课表"
	dispatch(ctx, env, handler, empty)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "还没有课程") {
		t.Fatalf("empty export reply = %q", content)
	}

	// A member cannot export someone else's schedule.
	other := freshMessage(base)
	other.Content = "/导出课表 小红"
	dispatch(ctx, env, handler, other)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "普通成员只能操作自己") {
		t.Fatalf("permission reply = %q", content)
	}
}
