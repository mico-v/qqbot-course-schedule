// Package app owns process composition, HTTP routing, workers and lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

const shutdownTimeout = 10 * time.Second

// App owns runtime dependencies and background work for one process.
type App struct {
	version    string
	config     config.Config
	store      *store.Store
	renderer   *render.Renderer
	env        *bot.Env
	handler    *bot.Handler
	httpServer *http.Server

	workers      sync.WaitGroup
	cancelWorker context.CancelFunc
}

// Run constructs the application, serves until ctx is canceled or the HTTP
// server fails, then shuts down in dependency order.
func Run(ctx context.Context, cfg config.Config, version string) error {
	application, err := New(cfg, version)
	if err != nil {
		return err
	}
	return application.Run(ctx)
}

// New builds all dependencies without starting network listeners or workers.
func New(cfg config.Config, version string) (*App, error) {
	storeHandle, err := store.Open(cfg.Database)
	if err != nil {
		return nil, err
	}
	renderer, err := render.New()
	if err != nil {
		_ = storeHandle.Close()
		return nil, fmt.Errorf("加载字体失败: %w", err)
	}
	application := &App{
		version:  version,
		config:   cfg,
		store:    storeHandle,
		renderer: renderer,
	}
	if err := application.build(); err != nil {
		_ = storeHandle.Close()
		return nil, err
	}
	return application, nil
}

// Run starts the HTTP server and workers, then waits for shutdown.
func (a *App) Run(ctx context.Context) error {
	defer a.close()
	if err := a.startWorkers(ctx); err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("webhook 监听中", "addr", a.httpServer.Addr, "path", "/webhook")
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	var runErr error
	select {
	case err := <-serveErr:
		runErr = fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		slog.Info("收到退出信号，正在关闭")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = fmt.Errorf("关闭 HTTP 服务失败: %w", err)
	}
	if a.cancelWorker != nil {
		a.cancelWorker()
	}
	return runErr
}

func (a *App) close() {
	a.workers.Wait()
	if err := a.store.Close(); err != nil {
		slog.Warn("关闭数据库失败", "err", err)
	}
}
