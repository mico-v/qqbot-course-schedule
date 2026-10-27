package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
)

func TestNewBuildsGroupOnlyHTTPApplication(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{
		Port:            8080,
		Bind:            "127.0.0.1",
		AppID:           "app-id",
		Secret:          "secret",
		Domain:          config.DefaultDomain,
		TokenEndpoint:   config.DefaultTokenEndpoint,
		Database:        filepath.Join(dataDir, "test.db"),
		DataDir:         dataDir,
		LogLevel:        config.DefaultLogLevel,
		WakeUpAndroidID: config.DefaultWakeUpAndroidID,
	}
	application, err := New(cfg, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(application.close)

	health := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	application.httpServer.Handler.ServeHTTP(health, request)
	if health.Code != http.StatusOK {
		t.Fatalf("health = %d (%s)", health.Code, health.Body.String())
	}

	webhook := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/webhook", nil)
	application.httpServer.Handler.ServeHTTP(webhook, request)
	if webhook.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned webhook = %d, want 401", webhook.Code)
	}
}

func TestRunStopsCleanlyWhenContextIsCanceled(t *testing.T) {
	cfg := config.Config{
		Port:            0,
		Bind:            "127.0.0.1",
		AppID:           "app-id",
		Secret:          "secret",
		Domain:          config.DefaultDomain,
		TokenEndpoint:   config.DefaultTokenEndpoint,
		Database:        filepath.Join(t.TempDir(), "run.db"),
		DataDir:         t.TempDir(),
		LogLevel:        config.DefaultLogLevel,
		WakeUpAndroidID: config.DefaultWakeUpAndroidID,
	}
	application, err := New(cfg, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Keep lifecycle coverage deterministic without making external API calls.
	application.env.Client = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := application.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
