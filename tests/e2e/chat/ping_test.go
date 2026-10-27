package chat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func TestGroupMessageToPassiveReply(t *testing.T) {
	messages := make(chan map[string]any, 1)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/getAppAccessToken":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token",
				"expires_in":   "7200",
			})
		case "/v2/groups/GROUP/messages":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			messages <- body
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "reply-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)

	handle, err := store.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handle.Close() })
	cfg := &config.Config{
		AppID:         "app",
		Secret:        "secret",
		Domain:        fake.URL,
		TokenEndpoint: fake.URL + "/app/getAppAccessToken",
	}
	env := &bot.Env{
		Client:  qqapi.New(cfg),
		Service: schedule.NewService(handle),
		DataDir: t.TempDir(),
	}
	handler := bot.NewDefaultHandler(env)
	in := bot.NewInbound(
		context.Background(),
		bot.OriginGroup,
		"GROUP",
		"MEMBER",
		"msg-1",
		"",
		"/ping",
		"小明",
		"member",
		nil,
		nil,
	)
	handler.Dispatch(context.Background(), in, bot.NewReplier(in, env.Client))

	select {
	case body := <-messages:
		if body["content"] != "pong" {
			t.Fatalf("content = %v, want pong", body["content"])
		}
		if body["msg_id"] != "msg-1" {
			t.Fatalf("msg_id = %v", body["msg_id"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for passive reply")
	}
}
