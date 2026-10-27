package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
)

func (a *App) startWorkers(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	a.cancelWorker = cancel
	a.workers.Add(2)

	go func() {
		defer a.workers.Done()
		a.env.RefreshBotAvatar(ctx)
	}()
	go func() {
		defer a.workers.Done()
		panelCtx, panelCancel := context.WithTimeout(ctx, 30*time.Second)
		defer panelCancel()
		created, updated, err := bot.SyncPanels(panelCtx, a.env, a.handler)
		if err != nil {
			slog.Warn("同步指令面板失败", "err", err)
			return
		}
		slog.Info("指令面板同步完成", "created", created, "updated", updated)
	}()
	return nil
}
