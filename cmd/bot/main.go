// Command bot runs the QQ official-bot course schedule service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	_ "time/tzdata"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/server"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
	"github.com/mico-v/qqbot-course-schedule/internal/webhook"
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

	storeHandle, err := store.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		if err := storeHandle.Close(); err != nil {
			slog.Warn("关闭数据库失败", "err", err)
		}
	}()

	renderer, err := render.New()
	if err != nil {
		return fmt.Errorf("加载字体失败: %w", err)
	}

	client := qqapi.New(cfg)
	env := &bot.Env{
		Client:        client,
		Store:         storeHandle,
		Service:       schedule.NewService(storeHandle),
		Renderer:      renderer,
		DataDir:       cfg.DataDir,
		ImagesDir:     filepath.Join(cfg.DataDir, "images"),
		PublicBaseURL: cfg.PublicImageBase(),
		Buttons:       cfg.Buttons,
		PushCron:      cfg.PushCron,
	}
	handler := bot.NewDefaultHandler(env)
	dispatcher := webhook.NewDispatcher(client, cfg.Secret, handler)
	verify, err := webhook.Verify(cfg.Secret)
	if err != nil {
		return fmt.Errorf("初始化 webhook 签名校验失败: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "version": version})
	})
	router.POST("/webhook", verify, dispatcher.Handle)
	server.RegisterImages(router, env.ImagesDir)
	server.RegisterAdmin(router, env.Service, cfg.AdminPassword)

	server := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go refreshBotAvatar(client, env)
	go syncCommandPanels(env, handler)
	go syncMenu(env)

	scheduler, err := bot.StartScheduler(env, cfg.PushCron)
	if err != nil {
		slog.Warn("每日推送未启用", "err", err)
	} else {
		defer scheduler.Stop()
		slog.Info("每日推送已启用", "cron", cfg.PushCron)
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("webhook 监听中", "addr", server.Addr, "path", "/webhook")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		slog.Info("收到退出信号，正在关闭")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("关闭 HTTP 服务失败: %w", err)
	}
	return nil
}

func syncMenu(env *bot.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	changed, err := bot.SyncMenu(ctx, env)
	if err != nil {
		slog.Warn("同步自定义菜单失败", "err", err)
		return
	}
	slog.Info("自定义菜单同步完成", "changed", changed)
}

func syncCommandPanels(env *bot.Env, handler *bot.Handler) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	created, updated, err := bot.SyncPanels(ctx, env, handler)
	if err != nil {
		slog.Warn("同步指令面板失败", "err", err)
		return
	}
	slog.Info("指令面板同步完成", "created", created, "updated", updated)
}

func refreshBotAvatar(client *qqapi.Client, env *bot.Env) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := client.GetBotInfo(ctx)
	if err != nil {
		slog.Warn("获取机器人信息失败", "err", err)
		return
	}
	avatar := render.FetchBotAvatar(ctx, info.Avatar, env.DataDir)
	if avatar != nil {
		env.SetBotAvatar(avatar)
		slog.Info("机器人头像已缓存", "name", info.Username)
	}
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
