package bot

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

const botWakeUpShare = `{"courseLen":50,"id":1,"name":"默认"}
[{"endTime":"08:45","node":1,"startTime":"08:00"}]
{"maxWeek":20,"nodes":1,"startDate":"2026-8-31","tableName":"26秋"}
{"courseName":"高等数学","room":"A101","day":1,"startNode":1,"step":1,"startWeek":1,"endWeek":4}`

type stubWakeUp struct {
	shareData string
	err       error
	gotCode   string
}

func (s *stubWakeUp) FetchShareData(_ context.Context, code string) (string, error) {
	s.gotCode = code
	return s.shareData, s.err
}

func TestDispatchAutoImportsWakeUpShare(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	stub := &stubWakeUp{shareData: botWakeUpShare}
	env.WakeUp = stub
	handler := NewDefaultHandler(env)

	message.Content = "这是来自「WakeUp课程表」的课表分享，30分钟内有效。分享口令为「aBc123」"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "已创建 小明 的课表") {
		t.Fatalf("reply = %v", reply)
	}
	if stub.gotCode != "aBc123" {
		t.Fatalf("fetched code = %q, want aBc123", stub.gotCode)
	}
}

func TestImportCommandAcceptsWakeUpCode(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	stub := &stubWakeUp{shareData: botWakeUpShare}
	env.WakeUp = stub
	handler := NewDefaultHandler(env)

	message.Content = "/导入课表 aBc123"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "已创建 小明 的课表") {
		t.Fatalf("reply = %v", reply)
	}
	if stub.gotCode != "aBc123" {
		t.Fatalf("fetched code = %q, want aBc123", stub.gotCode)
	}
}

func TestImportCommandWithoutCodePrompts(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	env.WakeUp = &stubWakeUp{}
	handler := NewDefaultHandler(env)

	message.Content = "/导入课表"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "未检测到") {
		t.Fatalf("reply = %v", reply)
	}
}

func TestAutoImportIgnoresPlainSharePhrase(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	stub := &stubWakeUp{shareData: botWakeUpShare}
	env.WakeUp = stub
	handler := NewDefaultHandler(env)

	// Mentions the phrase but is not the official WakeUp share message.
	message.Content = "大家互相分享口令的时候要注意隐私"
	dispatch(context.Background(), env, handler, message)

	if stub.gotCode != "" {
		t.Fatalf("fetcher should not run, got code %q", stub.gotCode)
	}
}

func TestImportCommandReportsFetchFailure(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	env.WakeUp = &stubWakeUp{err: errors.New("boom")}
	handler := NewDefaultHandler(env)

	message.Content = "/导入课表 aBc123"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "获取 WakeUp 课表失败") {
		t.Fatalf("reply = %v", reply)
	}
}

func TestImportCommandReportsExpiredCode(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, message := newTestEnv(t, fake, apiServer.URL)
	env.WakeUp = &stubWakeUp{}
	handler := NewDefaultHandler(env)

	message.Content = "/导入课表 aBc123"
	dispatch(context.Background(), env, handler, message)

	reply := waitMessage(t, fake)
	content, _ := reply["content"].(string)
	if !strings.Contains(content, "无效或已过期") {
		t.Fatalf("reply = %v", reply)
	}
}
