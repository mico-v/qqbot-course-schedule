package qqapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

func TestC2CMessageVariantsAndMissingMedia(t *testing.T) {
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/app/getAppAccessToken":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "token", "expires_in": "7200",
			})
		case r.URL.Path == "/v2/users/U1/files":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["file_type"] == float64(4) && body["file_name"] == nil {
				_ = json.NewEncoder(w).Encode(map[string]string{})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"file_info": "fi"})
		case r.URL.Path == "/v2/users/U1/messages":
			sends.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "sent"})
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
	if err := client.SendC2CTextEvent(ctx, "U1", "pong", "event"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendC2CText(ctx, "U1", "pong", "msg", 1); err != nil {
		t.Fatal(err)
	}
	if err := client.SendC2CMarkdown(ctx, "U1", "markdown", nil, "msg", 2); err != nil {
		t.Fatal(err)
	}
	if err := client.SendC2CMarkdownEvent(ctx, "U1", "markdown", nil, "event"); err != nil {
		t.Fatal(err)
	}
	if err := client.SendC2CImage(ctx, "U1", "https://example/image.jpg", "msg", 3); err != nil {
		t.Fatal(err)
	}
	if err := client.SendC2CImageEvent(ctx, "U1", "https://example/image.jpg", "event"); err != nil {
		t.Fatal(err)
	}
	if err := client.SendC2CFile(ctx, "U1", "https://example/file.txt", "", "msg", 4); err == nil {
		t.Fatal("SendC2CFile should reject a missing file_info")
	}
	if sends.Load() != 6 {
		t.Fatalf("sends = %d, want 6", sends.Load())
	}
}
