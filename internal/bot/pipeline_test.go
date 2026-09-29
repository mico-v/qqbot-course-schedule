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
	mu          sync.Mutex
	uploads     []map[string]any
	messages    []map[string]any
	msgCh       chan map[string]any
	failUploads bool
	// failMarkdown rejects msg_type=2 so the plain-text fallback can be tested.
	failMarkdown bool
	avatarURL    string
}

func newFakeQQ() *fakeQQ {
	return &fakeQQ{msgCh: make(chan map[string]any, 32)}
}

func (f *fakeQQ) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/getAppAccessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "test-token", "expires_in": "7200"})
	})
	mux.HandleFunc("/users/@me", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		avatarURL := f.avatarURL
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "课表机器人", "avatar": avatarURL})
	})
	mux.HandleFunc("/v2/groups/GROUP/files", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.uploads = append(f.uploads, body)
		fail := f.failUploads
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 40034105, "message": "主动消息发送失败，无权限"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"file_info": "fi-1"})
	})
	mux.HandleFunc("/v2/groups/GROUP/messages", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.messages = append(f.messages, body)
		failMarkdown := f.failMarkdown
		f.mu.Unlock()
		if failMarkdown {
			if msgType, _ := body["msg_type"].(float64); msgType == 2 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 40034024, "message": "markdown 消息不可用"})
				return
			}
		}
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

func newTestEnv(t *testing.T, fake *fakeQQ, apiURL string) (*Env, *Inbound) {
	t.Helper()
	env, message, _ := newTestEnvWithStore(t, fake, apiURL)
	return env, message
}

func newTestEnvWithStore(t *testing.T, fake *fakeQQ, apiURL string) (*Env, *Inbound, *store.Store) {
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
		PanelStore:    storeHandle,
		Service:       schedule.NewService(storeHandle),
		Renderer:      renderer,
		DataDir:       cfg.DataDir,
		ImagesDir:     filepath.Join(cfg.DataDir, "images"),
		PublicBaseURL: "https://cards.example.com",
		Now:           func() time.Time { return fixedNow },
	}
	message := &Inbound{
		Origin:      OriginGroup,
		GroupOpenID: "GROUP",
		UserOpenID:  "U1",
		MsgID:       "m1",
		Username:    "小明",
		MemberRole:  "member",
	}
	return env, message, storeHandle
}

// dispatch routes one inbound through the handler with a fresh replier, which
// is how the webhook dispatcher calls it in production. env may be nil for the
// routing-only tests, which never reach a command that sends.
func dispatch(ctx context.Context, env *Env, handler *Handler, in *Inbound) {
	var client *qqapi.Client
	if env != nil {
		client = env.Client
	}
	handler.Dispatch(ctx, in, NewReplier(in, client))
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
	if err := env.ImportICS(ctx, message, NewReplier(message, env.Client), Attachment{
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
	dispatch(ctx, env, handler, message)
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
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "还没有可展示的课程表") {
		t.Fatalf("reply = %v", reply)
	}
}

func TestMarkdownCardUsesRenderedDimensions(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	env.Buttons = true
	ctx := context.Background()
	card := RenderedCard{
		URL:    "https://cards.example.com/images/schedule_20260917_test.jpg",
		Width:  1240,
		Height: 928,
	}

	if err := env.SendCard(ctx, message, env.newReplier(ctx, message), card, &qqapi.Keyboard{}); err != nil {
		t.Fatalf("SendCard: %v", err)
	}
	reply := waitMessage(t, fake)
	markdown, _ := reply["markdown"].(map[string]any)
	content, _ := markdown["content"].(string)
	want := "![课程表 #1240px #928px](" + card.URL + ")"
	if content != want {
		t.Fatalf("markdown content = %q, want %q", content, want)
	}
}

func TestScheduleCommandParsesDate(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	message.Content = "/课表 不存在的东西"
	dispatch(context.Background(), env, handler, message)

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
	if err := env.ImportICS(context.Background(), message, NewReplier(message, env.Client), Attachment{
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
func freshMessage(base *Inbound) *Inbound {
	return &Inbound{
		Origin:      base.Origin,
		GroupOpenID: base.GroupOpenID,
		UserOpenID:  base.UserOpenID,
		MsgID:       base.MsgID,
		Username:    base.Username,
		MemberRole:  base.MemberRole,
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

	freshImport := freshMessage(base)
	if err := env.ImportICS(ctx, freshImport, NewReplier(freshImport, env.Client), Attachment{
		URL: icsServer.URL + "/schedule.ics", Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}
	_ = waitMessage(t, fake)

	// Admin marks tomorrow as a holiday for everyone.
	admin := freshMessage(base)
	admin.MemberRole = "owner"
	admin.Content = "/休假 明天"
	dispatch(ctx, env, handler, admin)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已将 2026-09-18 标记为休假（全体成员）") {
		t.Fatalf("day off reply = %q", content)
	}

	// /假期 lists it.
	list := freshMessage(base)
	list.Content = "/假期"
	dispatch(ctx, env, handler, list)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "2026-09-18 休假") {
		t.Fatalf("holiday list = %q", content)
	}

	// A normal member cannot target everyone.
	member := freshMessage(base)
	member.Content = "/休假 明天 全体"
	dispatch(ctx, env, handler, member)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "只有管理员") {
		t.Fatalf("member reply = %q", content)
	}

	// Admin cancels the marker.
	cancel := freshMessage(base)
	cancel.MemberRole = "owner"
	cancel.Content = "/销假 明天"
	dispatch(ctx, env, handler, cancel)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已取消 2026-09-18 的休假标记") {
		t.Fatalf("cancel reply = %q", content)
	}

	// Alias command renders the rank card as an image.
	rank := freshMessage(base)
	rank.Content = "/本周上课排行"
	dispatch(ctx, env, handler, rank)
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

func TestPlainMessageRecordsSeenMember(t *testing.T) {
	env, base, storeHandle := newTestEnvWithStore(t, newFakeQQ(), "http://127.0.0.1:1")
	handler := NewDefaultHandler(env)
	msg := freshMessage(base)
	msg.Content = "大家早上好"
	dispatch(context.Background(), env, handler, msg)

	var seen map[string]string
	found, err := storeHandle.GetKV(schedule.KVScopeGlobal, schedule.KVNamespaceSeen, env.Scope(base), &seen)
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(seen) != 1 || seen["U1"] != "小明" {
		t.Fatalf("seen members = %+v, found=%v", seen, found)
	}
}
