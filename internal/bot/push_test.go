package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
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
	env.PushCron = "not a cron"
	if _, err := StartScheduler(env); err == nil {
		t.Fatal("invalid cron should fail")
	}
	env.PushCron = "0 7 * * *"
	scheduler, err := StartScheduler(env)
	if err != nil {
		t.Fatalf("StartScheduler: %v", err)
	}
	scheduler.Stop()
}

func TestPushDuePerScopeCron(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	ctx := context.Background()
	if err := env.ImportICS(ctx, freshMessage(base), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	env.PushCron = "30 7 * * *"
	baseScope := env.Scope(base)
	if err := env.SetPushSubscription(baseScope, PushSubscription{
		Enabled: true, Origin: "group", OpenID: base.GroupOpenID, Cron: "0 8 * * *",
	}); err != nil {
		t.Fatal(err)
	}
	other := schedule.ScopeGroup("OTHER")
	if err := env.SetPushSubscription(other, PushSubscription{
		Enabled: true, Origin: "group", OpenID: "OTHER-OPENID",
	}); err != nil {
		t.Fatal(err)
	}

	at := func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02 15:04", value, schedule.LocalTZ)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}

	// 07:30 is the default time: the other scope has no schedule (skipped),
	// while the base scope waits for its own 08:00.
	if sent, skipped, failed := env.PushDue(ctx, at("2026-09-17 07:30")); sent != 0 || skipped != 1 || failed != 0 {
		t.Fatalf("07:30 PushDue = %d/%d/%d", sent, skipped, failed)
	}
	if sent, skipped, failed := env.PushDue(ctx, at("2026-09-17 08:00")); sent != 1 || skipped != 0 || failed != 0 {
		t.Fatalf("08:00 PushDue = %d/%d/%d", sent, skipped, failed)
	}
	// The same minute must not double-send (restart/duplicate tick guard).
	if sent, _, _ := env.PushDue(ctx, at("2026-09-17 08:00")); sent != 0 {
		t.Fatalf("duplicate minute sent %d", sent)
	}
	if sent, _, _ := env.PushDue(ctx, at("2026-09-18 08:00")); sent != 1 {
		t.Fatalf("next day sent %d", sent)
	}
}

func TestPushTimeCommand(t *testing.T) {
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

	// Not subscribed yet.
	msg := freshMessage(base)
	msg.MemberRole = "owner"
	msg.Content = "/推送时间"
	handler.Dispatch(ctx, msg)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "请先发送 /启用推送") {
		t.Fatalf("reply = %q", content)
	}

	enable := freshMessage(base)
	enable.MemberRole = "owner"
	enable.Content = "/启用推送"
	handler.Dispatch(ctx, enable)
	_ = waitMessage(t, fake)

	// Normal members cannot change the group time.
	member := freshMessage(base)
	member.Content = "/推送时间 08:15"
	handler.Dispatch(ctx, member)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "只有群管理员") {
		t.Fatalf("member reply = %q", content)
	}

	set := freshMessage(base)
	set.MemberRole = "owner"
	set.Content = "/推送时间 08:15"
	handler.Dispatch(ctx, set)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "每天 08:15") {
		t.Fatalf("set reply = %q", content)
	}
	subscriptions, err := env.PushSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	if got := subscriptions[env.Scope(base)].Cron; got != "15 8 * * *" {
		t.Fatalf("stored cron = %q", got)
	}

	show := freshMessage(base)
	show.MemberRole = "owner"
	show.Content = "/推送时间"
	handler.Dispatch(ctx, show)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "每天 08:15") || !strings.Contains(content, "自定义") {
		t.Fatalf("show reply = %q", content)
	}

	bad := freshMessage(base)
	bad.MemberRole = "owner"
	bad.Content = "/推送时间 25:99"
	handler.Dispatch(ctx, bad)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "HH:MM") {
		t.Fatalf("bad reply = %q", content)
	}

	reset := freshMessage(base)
	reset.MemberRole = "owner"
	reset.Content = "/推送时间 默认"
	handler.Dispatch(ctx, reset)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "每天 07:30") {
		t.Fatalf("reset reply = %q", content)
	}
}

var _ = qqapi.Keyboard{}

func TestPushTestCommand(t *testing.T) {
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

	// Without a subscription the test command explains what to do.
	msg := freshMessage(base)
	msg.MemberRole = "owner"
	msg.Content = "/推送测试"
	handler.Dispatch(ctx, msg)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "请先发送 /启用推送") {
		t.Fatalf("reply = %q", content)
	}

	// Enable, then the test command pushes a card and confirms.
	enable := freshMessage(base)
	enable.MemberRole = "owner"
	enable.Content = "/启用推送"
	handler.Dispatch(ctx, enable)
	_ = waitMessage(t, fake)

	test := freshMessage(base)
	test.MemberRole = "owner"
	test.Content = "/推送测试"
	handler.Dispatch(ctx, test)
	card := waitMessage(t, fake)
	if msgType, _ := card["msg_type"].(float64); msgType != 7 {
		t.Fatalf("push test card = %+v", card)
	}
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已推送一次当日课表") {
		t.Fatalf("reply = %q", content)
	}
}
