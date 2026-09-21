package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

const testICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//CN
BEGIN:VEVENT
UID:t1
SUMMARY:高等数学
DTSTART;TZID=Asia/Shanghai:20260917T090000
DTEND;TZID=Asia/Shanghai:20260917T103000
LOCATION:教一101
END:VEVENT
END:VCALENDAR
`

type fakeQQ struct {
	mu       sync.Mutex
	uploads  []map[string]any
	messages []map[string]any
	msgCh    chan map[string]any
}

func newFakeQQ() *fakeQQ {
	return &fakeQQ{msgCh: make(chan map[string]any, 32)}
}

func (f *fakeQQ) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/getAppAccessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "test-token", "expires_in": "7200"})
	})
	mux.HandleFunc("/v2/groups/GROUP/files", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.uploads = append(f.uploads, body)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"file_info": "fi-1"})
	})
	mux.HandleFunc("/v2/groups/GROUP/messages", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.messages = append(f.messages, body)
		f.mu.Unlock()
		f.msgCh <- body
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "sent-1"})
	})
	return mux
}

func waitMessage(t *testing.T, fake *fakeQQ) map[string]any {
	t.Helper()
	select {
	case message := <-fake.msgCh:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
		return nil
	}
}

func newTestEnv(t *testing.T, fake *fakeQQ, apiURL string) (*Env, *Message) {
	t.Helper()
	cfg := &config.Config{
		Port:          8080,
		AppID:         "10000",
		Secret:        "test-secret",
		Domain:        apiURL,
		TokenEndpoint: apiURL + "/app/getAppAccessToken",
		DataDir:       t.TempDir(),
		LogLevel:      "error",
	}
	client := qqapi.New(cfg)
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })
	renderer, err := render.New()
	if err != nil {
		t.Fatalf("render.New: %v", err)
	}
	fixedNow := time.Date(2026, 9, 17, 9, 30, 0, 0, schedule.LocalTZ)
	env := &Env{
		Client:        client,
		Store:         storeHandle,
		Service:       schedule.NewService(storeHandle),
		Renderer:      renderer,
		DataDir:       cfg.DataDir,
		ImagesDir:     filepath.Join(cfg.DataDir, "images"),
		PublicBaseURL: "https://cards.example.com",
		PushCron:      "30 7 * * *",
		Now:           func() time.Time { return fixedNow },
	}
	message := &Message{
		Origin:      OriginGroup,
		GroupOpenID: "GROUP",
		UserOpenID:  "U1",
		MsgID:       "m1",
		Username:    "小明",
		MemberRole:  "member",
		Client:      client,
	}
	return env, message
}

func TestImportThenDayCardPipeline(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()

	// 1. Import the ICS attachment.
	if err := env.ImportICS(ctx, message, Attachment{
		URL:      icsServer.URL + "/schedule.ics",
		Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}
	importReply := waitMessage(t, fake)
	if content, _ := importReply["content"].(string); !strings.Contains(content, "已创建 小明 的课表") {
		t.Fatalf("import reply = %v", importReply)
	}

	// 2. /今日课表 renders a card and sends it as a media message.
	message.Content = "/今日课表"
	handler.Dispatch(ctx, message)
	cardReply := waitMessage(t, fake)

	msgType, _ := cardReply["msg_type"].(float64)
	if msgType != 7 {
		t.Fatalf("card msg_type = %v, want 7 (%v)", cardReply["msg_type"], cardReply)
	}
	if cardReply["msg_id"] != "m1" {
		t.Errorf("card msg_id = %v", cardReply["msg_id"])
	}
	media, _ := cardReply["media"].(map[string]any)
	if media == nil || media["file_info"] != "fi-1" {
		t.Fatalf("card media = %v", cardReply["media"])
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(fake.uploads))
	}
	uploadURL, _ := fake.uploads[0]["url"].(string)
	if !strings.HasPrefix(uploadURL, "https://cards.example.com/images/schedule_20260917_") {
		t.Errorf("upload url = %q", uploadURL)
	}
	if fileType, _ := fake.uploads[0]["file_type"].(float64); fileType != 1 {
		t.Errorf("file_type = %v, want 1", fake.uploads[0]["file_type"])
	}

	// 3. The generated image is served from the images directory.
	name := strings.TrimPrefix(uploadURL, "https://cards.example.com/images/")
	if _, err := filepath.Glob(filepath.Join(env.ImagesDir, name)); err != nil {
		t.Fatalf("image file missing: %v", err)
	}
}

func TestDayCardWithoutScheduleRepliesText(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	message.Content = "/课表"
	handler.Dispatch(context.Background(), message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "还没有可展示的课程表") {
		t.Fatalf("reply = %v", reply)
	}
}

func TestScheduleCommandParsesDate(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	message.Content = "/课表 不存在的东西"
	handler.Dispatch(context.Background(), message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "无法识别日期") {
		t.Fatalf("reply = %v", reply)
	}
}

func TestImportFailureSavesFileAndReplies(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	garbageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PK\x03\x04not-an-ics"))
	}))
	t.Cleanup(garbageServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	if err := env.ImportICS(context.Background(), message, Attachment{
		URL:      garbageServer.URL + "/schedule.ics",
		Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}
	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "不是有效的 iCalendar") || !strings.Contains(content, "压缩包") {
		t.Fatalf("reply = %q", content)
	}
	files, err := filepath.Glob(filepath.Join(env.DataDir, "failed_ics", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("failed_ics files = %v, err = %v", files, err)
	}
}

// freshMessage clones the envelope so every command gets its own reply budget.
func freshMessage(base *Message) *Message {
	return &Message{
		Origin:      base.Origin,
		GroupOpenID: base.GroupOpenID,
		UserOpenID:  base.UserOpenID,
		MsgID:       base.MsgID,
		Username:    base.Username,
		MemberRole:  base.MemberRole,
		Client:      base.Client,
	}
}

func TestDayOffAndRankPipeline(t *testing.T) {
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

	if err := env.ImportICS(ctx, freshMessage(base), Attachment{
		URL: icsServer.URL + "/schedule.ics", Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}
	_ = waitMessage(t, fake)

	// Admin marks tomorrow as a holiday for everyone.
	admin := freshMessage(base)
	admin.MemberRole = "owner"
	admin.Content = "/休假 明天"
	handler.Dispatch(ctx, admin)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已将 2026-09-18 标记为休假（全体成员）") {
		t.Fatalf("day off reply = %q", content)
	}

	// /假期 lists it.
	list := freshMessage(base)
	list.Content = "/假期"
	handler.Dispatch(ctx, list)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "2026-09-18 休假") {
		t.Fatalf("holiday list = %q", content)
	}

	// A normal member cannot target everyone.
	member := freshMessage(base)
	member.Content = "/休假 明天 全体"
	handler.Dispatch(ctx, member)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "只有管理员") {
		t.Fatalf("member reply = %q", content)
	}

	// Admin cancels the marker.
	cancel := freshMessage(base)
	cancel.MemberRole = "owner"
	cancel.Content = "/销假 明天"
	handler.Dispatch(ctx, cancel)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已取消 2026-09-18 的休假标记") {
		t.Fatalf("cancel reply = %q", content)
	}

	// Alias command renders the rank card as an image.
	rank := freshMessage(base)
	rank.Content = "/本周上课排行"
	handler.Dispatch(ctx, rank)
	reply = waitMessage(t, fake)
	if msgType, _ := reply["msg_type"].(float64); msgType != 7 {
		t.Fatalf("rank reply = %+v", reply)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.uploads) == 0 {
		t.Fatal("rank card was not uploaded")
	}
	last := fake.uploads[len(fake.uploads)-1]
	if url, _ := last["url"].(string); !strings.Contains(url, "/images/rank_") {
		t.Fatalf("rank upload url = %q", url)
	}
}
