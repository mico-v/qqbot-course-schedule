package bot

import (
	"context"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

var checkinAwardPattern = regexp.MustCompile(`签到成功，获得 (\d+) 群积分`)

func TestCheckinCommandAwardsOncePerDay(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()

	first := freshMessage(base)
	first.Content = "/签到"
	dispatch(ctx, env, handler, first)
	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	match := checkinAwardPattern.FindStringSubmatch(content)
	if match == nil {
		t.Fatalf("check-in reply = %q", content)
	}
	awarded := match[1]
	if !strings.Contains(content, "已签到 1 天") {
		t.Fatalf("check-in reply = %q", content)
	}

	second := freshMessage(base)
	second.Content = "/打卡"
	dispatch(ctx, env, handler, second)
	reply = waitMessage(t, fake)
	content, _ = reply["content"].(string)
	if !strings.Contains(content, "你今天已经签到过了，本次获得 "+awarded+" 群积分") {
		t.Fatalf("repeat check-in reply = %q", content)
	}

	points := freshMessage(base)
	points.Content = "/积分"
	dispatch(ctx, env, handler, points)
	reply = waitMessage(t, fake)
	content, _ = reply["content"].(string)
	if !strings.Contains(content, "你当前共有 "+awarded+" 群积分，已签到 1 天。") {
		t.Fatalf("points reply = %q", content)
	}
	if !strings.Contains(content, "2026-09-17：+"+awarded) {
		t.Fatalf("points reply missing the record: %q", content)
	}
}

func TestPointsWithoutCheckin(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	message := freshMessage(base)
	message.Content = "/我的积分"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "你还没有签到记录") {
		t.Fatalf("reply = %q", content)
	}
}

func TestNicknameCommand(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()

	// A plain member cannot rename the bot.
	member := freshMessage(base)
	member.Content = "/nikname 张三"
	dispatch(ctx, env, handler, member)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "只有群管理员") {
		t.Fatalf("member reply = %q", content)
	}

	admin := freshMessage(base)
	admin.MemberRole = "admin"
	admin.Content = "/nickname 课表小助手"
	dispatch(ctx, env, handler, admin)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "课表小助手") {
		t.Fatalf("set reply = %q", content)
	}
	settings, err := env.Service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Nickname != "课表小助手" {
		t.Fatalf("nickname = %q", settings.Nickname)
	}

	// Bare /nikname reports the current name and the usage.
	query := freshMessage(base)
	query.Content = "/nikname"
	dispatch(ctx, env, handler, query)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "当前机器人昵称：课表小助手") {
		t.Fatalf("query reply = %q", content)
	}

	tooLong := freshMessage(base)
	tooLong.MemberRole = "admin"
	tooLong.Content = "/nikname " + strings.Repeat("长", schedule.MaxBotNicknameLength+1)
	dispatch(ctx, env, handler, tooLong)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "昵称不能超过") {
		t.Fatalf("long reply = %q", content)
	}

	clear := freshMessage(base)
	clear.MemberRole = "admin"
	clear.Content = "/nikname 清空"
	dispatch(ctx, env, handler, clear)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已清空机器人昵称") {
		t.Fatalf("clear reply = %q", content)
	}
	settings, err = env.Service.BotSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Nickname != "" {
		t.Fatalf("nickname after clear = %q", settings.Nickname)
	}
}

func TestNicknameCommandWorksWhileBotOff(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	off := schedule.DefaultSettings()
	off.Enabled = false
	if err := env.Service.SaveBotSettings(off); err != nil {
		t.Fatal(err)
	}

	admin := freshMessage(base)
	admin.MemberRole = "owner"
	admin.Content = "/nikname 值班机器人"
	dispatch(context.Background(), env, handler, admin)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "值班机器人") {
		t.Fatalf("reply = %q", content)
	}
}
