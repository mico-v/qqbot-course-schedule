package bot

import (
	"context"
	"errors"
	"testing"
)

func TestDispatchStripsMentionAndRoutes(t *testing.T) {
	handler := NewHandler()
	var got string
	handler.Register(&Command{
		Prefix: "/ping",
		Handle: func(_ context.Context, msg *Message) error {
			got = msg.Content
			return nil
		},
	})

	handler.Dispatch(context.Background(), &Message{Content: "<@!abc123>   /ping 参数一"})
	if got != "/ping 参数一" {
		t.Fatalf("content = %q, want %q", got, "/ping 参数一")
	}
}

func TestDispatchAcceptsCommandWithoutSlash(t *testing.T) {
	handler := NewHandler()
	called := false
	handler.Register(&Command{
		Prefix: "/课表",
		Handle: func(_ context.Context, _ *Message) error {
			called = true
			return nil
		},
	})
	handler.Dispatch(context.Background(), &Message{Content: "课表 明天"})
	if !called {
		t.Fatal("slash-less command did not reach the handler")
	}
}

func TestDispatchIgnoresUnknownCommand(t *testing.T) {
	handler := NewHandler()
	called := false
	handler.Register(&Command{
		Prefix: "/ping",
		Handle: func(_ context.Context, _ *Message) error {
			called = true
			return nil
		},
	})

	handler.Dispatch(context.Background(), &Message{Content: "你好啊"})
	if called {
		t.Fatal("handler must not run for unknown content")
	}
}

func TestRegisterReplacesDuplicatePrefix(t *testing.T) {
	handler := NewHandler()
	var calls []string
	handler.Register(&Command{Prefix: "/x", Handle: func(_ context.Context, _ *Message) error {
		calls = append(calls, "first")
		return nil
	}})
	handler.Register(&Command{Prefix: "/x", Handle: func(_ context.Context, _ *Message) error {
		calls = append(calls, "second")
		return nil
	}})

	handler.Dispatch(context.Background(), &Message{Content: "/x"})
	if len(calls) != 1 || calls[0] != "second" {
		t.Fatalf("calls = %v, want [second]", calls)
	}
}

func TestCommandsAreSorted(t *testing.T) {
	handler := NewHandler()
	handler.Register(&Command{Prefix: "/b", Handle: func(_ context.Context, _ *Message) error { return nil }})
	handler.Register(&Command{Prefix: "/a", Handle: func(_ context.Context, _ *Message) error { return nil }})

	commands := handler.Commands()
	if len(commands) != 2 || commands[0].Prefix != "/a" || commands[1].Prefix != "/b" {
		t.Fatalf("commands = %+v", commands)
	}
}

func TestPassiveReplyLimit(t *testing.T) {
	msg := &Message{}
	for i := 1; i <= maxPassiveReplies; i++ {
		seq, err := msg.nextSeq()
		if err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
		if seq != i {
			t.Fatalf("seq = %d, want %d", seq, i)
		}
	}
	if _, err := msg.nextSeq(); !errors.Is(err, ErrPassiveLimit) {
		t.Fatalf("err = %v, want ErrPassiveLimit", err)
	}
}

func TestAdminRoles(t *testing.T) {
	cases := map[string]bool{
		"member": false,
		"admin":  true,
		"owner":  true,
		"":       false,
	}
	for role, want := range cases {
		msg := &Message{MemberRole: role}
		if got := msg.IsAdmin(); got != want {
			t.Errorf("role %q: IsAdmin() = %v, want %v", role, got, want)
		}
	}
}

func TestDefaultHandlerRegistersCommands(t *testing.T) {
	handler := NewDefaultHandler(nil)
	expected := []string{
		"/ping", "/help", "/今日课表", "/明日课表", "/课表", "/导入课表", "/导出课表",
		"/上课时长榜", "/休假", "/调休", "/销假", "/假期",
		"/启用推送", "/关闭推送", "/推送测试", "/同步面板",
	}
	for _, prefix := range expected {
		if _, ok := handler.Command(prefix); !ok {
			t.Errorf("command %s is not registered", prefix)
		}
	}
	if got := len(handler.Commands()); got != len(expected) {
		t.Errorf("Commands() = %d, want %d", got, len(expected))
	}

	// Aliases resolve to the same command pointer.
	canonical, _ := handler.Command("/上课时长榜")
	alias, ok := handler.Command("/上课排行")
	if !ok || alias != canonical {
		t.Fatalf("alias /上课排行 should map to /上课时长榜")
	}
}
