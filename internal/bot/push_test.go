package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

func TestEnableDisablePushAndPushDaily(t *testing.T) {
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

	// A normal member cannot enable the group push.
	member := freshMessage(base)
	member.Content = "/启用推送"
	handler.Dispatch(ctx, member)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "只有群管理员") {
		t.Fatalf("member reply = %q", content)
	}

	// The owner enables it.
	owner := freshMessage(base)
	owner.MemberRole = "owner"
	owner.Content = "/启用推送"
	handler.Dispatch(ctx, owner)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已开启每日课表推送（每天 07:30）") {
		t.Fatalf("owner reply = %q", content)
	}

	// The daily push sends a proactive card (no msg_id).
	sent, skipped, failed := env.PushDaily(ctx)
	if sent != 1 || skipped != 0 || failed != 0 {
		t.Fatalf("PushDaily = %d/%d/%d", sent, skipped, failed)
	}
	card := waitMessage(t, fake)
	if card["msg_id"] != nil && card["msg_id"] != "" {
		t.Fatalf("proactive card must not carry msg_id: %+v", card)
	}
	if msgType, _ := card["msg_type"].(float64); msgType != 7 {
		t.Fatalf("push card = %+v", card)
	}

	// Disable: the next run sends nothing.
	disable := freshMessage(base)
	disable.MemberRole = "owner"
	disable.Content = "/关闭推送"
	handler.Dispatch(ctx, disable)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已关闭") {
		t.Fatalf("disable reply = %q", content)
	}
	if sent, _, _ := env.PushDaily(ctx); sent != 0 {
		t.Fatalf("push after disable sent %d", sent)
	}
}

func TestPushTimeText(t *testing.T) {
	if got := pushTimeText("30 7 * * *"); got != "每天 07:30" {
		t.Errorf("pushTimeText = %q", got)
	}
	if got := pushTimeText("bad spec"); !strings.Contains(got, "定时") {
		t.Errorf("pushTimeText fallback = %q", got)
	}
}

func TestCardKeyboards(t *testing.T) {
	dayKeyboard := cardKeyboardForDay("2026-09-17", "2026-09-17", "U1")
	buttons := dayKeyboard.Content.Rows[0].Buttons
	if len(buttons) != 3 {
		t.Fatalf("buttons = %d", len(buttons))
	}
	if buttons[0].Action.Data != "day:2026-09-16" || buttons[2].Action.Data != "day:2026-09-18" {
		t.Fatalf("day data = %q %q", buttons[0].Action.Data, buttons[2].Action.Data)
	}
	if buttons[0].Action.Permission == nil || len(buttons[0].Action.Permission.SpecifyUserIDs) != 1 ||
		buttons[0].Action.Permission.SpecifyUserIDs[0] != "U1" {
		t.Fatalf("permission = %+v", buttons[0].Action.Permission)
	}
	rankKeyboard := cardKeyboardForRank("U1")
	rankData := rankKeyboard.Content.Rows[0].Buttons[0].Action.Data
	if rankData != "rank:thisweek" {
		t.Fatalf("rank data = %q", rankData)
	}
}

func TestHandleCallbackRendersRequestedDay(t *testing.T) {
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

	handler.HandleCallback(ctx, &Callback{
		Data:        "day:2026-09-18",
		EventID:     "INT-9",
		GroupOpenID: "GROUP",
		UserOpenID:  "U1",
	})
	card := waitMessage(t, fake)
	if card["event_id"] != "INT-9" {
		t.Fatalf("callback reply must use event_id: %+v", card)
	}
	if msgType, _ := card["msg_type"].(float64); msgType != 7 {
		t.Fatalf("callback card = %+v", card)
	}
}

func TestStartSchedulerValidatesCron(t *testing.T) {
	env, _ := newTestEnv(t, newFakeQQ(), "http://127.0.0.1:1")
	if _, err := StartScheduler(env, "not a cron"); err == nil {
		t.Fatal("invalid cron should fail")
	}
	scheduler, err := StartScheduler(env, "0 7 * * *")
	if err != nil {
		t.Fatalf("StartScheduler: %v", err)
	}
	scheduler.Stop()
}

var _ = qqapi.Keyboard{}
