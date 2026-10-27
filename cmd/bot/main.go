// Command bot runs the QQ official-bot course schedule service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "time/tzdata"

	"github.com/mico-v/qqbot-course-schedule/internal/app"
	"github.com/mico-v/qqbot-course-schedule/internal/config"
)

// version is injected at build time.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("BOT_CONFIG")
	if configPath == "" {
		configPath = "config.json"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	setupLogger(cfg.LogLevel)
	slog.Info("启动 qqbot-course-schedule", "version", version, "port", cfg.Port, "domain", cfg.Domain)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, *cfg, version)
}

func setupLogger(level string) {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})))
}
