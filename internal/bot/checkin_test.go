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

func TestNicknameCommandSetsOwnName(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()
	scope := env.Scope(base)

	// A plain member sets their own nickname, which is not the bot name.
	member := freshMessage(base)
	member.Content = "/nikname 张三"
	dispatch(ctx, env, handler, member)
	reply := waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已将你的昵称设置为「张三」") {
		t.Fatalf("set reply = %q", content)
	}
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		t.Fatal(err)
	}
	if got := members["U1"].Name; got != "张三" {
		t.Fatalf("member name = %q, want 张三", got)
	}

	// Bare /nickname reports the current name and the usage.
	query := freshMessage(base)
	query.Content = "/nickname"
	dispatch(ctx, env, handler, query)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "你当前的昵称：张三") {
		t.Fatalf("query reply = %q", content)
	}

	tooLong := freshMessage(base)
	tooLong.Content = "/nickname " + strings.Repeat("长", schedule.MaxMemberNicknameLength+1)
	dispatch(ctx, env, handler, tooLong)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "昵称不能超过") {
		t.Fatalf("long reply = %q", content)
	}

	clear := freshMessage(base)
	clear.Content = "/nickname 清空"
	dispatch(ctx, env, handler, clear)
	reply = waitMessage(t, fake)
	if content, _ := reply["content"].(string); !strings.Contains(content, "已清空你的昵称") {
		t.Fatalf("clear reply = %q", content)
	}
	members, err = env.Service.ScopeMembers(scope)
	if err != nil {
		t.Fatal(err)
	}
	if got := members["U1"].Name; got != "" {
		t.Fatalf("member name after clear = %q", got)
	}
}
