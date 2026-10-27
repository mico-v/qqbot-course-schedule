package qqapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

func TestClientMessageMediaMenuAndRetry(t *testing.T) {
	var mu sync.Mutex
	tokenRequests := 0
	menuRequests := 0
	messageBodies := make([]map[string]any, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/app/getAppAccessToken":
			mu.Lock()
			tokenRequests++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "token", "expires_in": "7200",
			})
		case r.URL.Path == "/v2/groups/G1/messages":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			messageBodies = append(messageBodies, body)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "sent"})
		case r.URL.Path == "/v2/groups/G1/files":
			_ = json.NewEncoder(w).Encode(map[string]string{"file_info": "file-info"})
		case r.URL.Path == "/interactions/INT1":
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case r.URL.Path == "/users/@me":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "bot", "username": "课表机器人", "avatar": "https://example/avatar.png",
			})
		case r.URL.Path == "/v2/menu" && r.Method == http.MethodGet:
			mu.Lock()
			menuRequests++
			attempt := menuRequests
			mu.Unlock()
			if attempt == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":500,"message":"temporary"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"menu": map[string]any{"items": []map[string]any{{"name": "课表", "type": "send_message"}}},
			})
		case r.URL.Path == "/v2/menu" && r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := qqapi.New(&config.Config{
		AppID: "app", Secret: "secret", Domain: server.URL,
		TokenEndpoint: server.URL + "/app/getAppAccessToken",
	})
	ctx := context.Background()
	result, err := client.SendGroupText(ctx, "G1", "pong", "msg-1", 1)
	if err != nil || result.ID != "sent" {
		t.Fatalf("SendGroupText = %+v, %v", result, err)
	}
	if err := client.SendGroupMarkdown(ctx, "G1", "# 课表", nil, "msg-2", 2); err != nil {
		t.Fatalf("SendGroupMarkdown: %v", err)
	}
	if err := client.SendGroupImage(ctx, "G1", "https://example/card.jpg", "msg-3", 3); err != nil {
		t.Fatalf("SendGroupImage: %v", err)
	}
	if err := client.AckInteraction(ctx, "INT1"); err != nil {
		t.Fatalf("AckInteraction: %v", err)
	}
	info, err := client.GetBotInfo(ctx)
	if err != nil || info.Username != "课表机器人" {
		t.Fatalf("GetBotInfo = %+v, %v", info, err)
	}
	menu, err := client.GetMenu(ctx)
	if err != nil || menu == nil || len(menu.Items) != 1 {
		t.Fatalf("GetMenu = %+v, %v", menu, err)
	}
	if err := client.SetMenu(ctx, qqapi.Menu{Items: []qqapi.MenuItem{
		{Name: "课表", Type: "send_message", SendMessage: "/课表"},
	}}); err != nil {
		t.Fatalf("SetMenu: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if tokenRequests != 1 {
		t.Fatalf("token requests = %d, want cached single request", tokenRequests)
	}
	if len(messageBodies) != 3 {
		t.Fatalf("message bodies = %d, want 3", len(messageBodies))
	}
	if messageBodies[2]["msg_type"] != float64(7) {
		t.Fatalf("media message = %+v", messageBodies[2])
	}
}
