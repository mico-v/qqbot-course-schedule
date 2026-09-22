package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler drives the daily push. It ticks every minute; each subscription's
// effective cron (per-scope override or the global default) decides whether the
// scope is pushed. This keeps runtime changes to /推送时间 effective without
// rebuilding cron entries.
type Scheduler struct {
	cron *cron.Cron
}

// StartScheduler validates the default push_cron and starts the minute ticker.
func StartScheduler(env *Env) (*Scheduler, error) {
	if _, err := pushCronParser.Parse(strings.TrimSpace(env.PushCron)); err != nil {
		return nil, fmt.Errorf("push_cron 无效: %w", err)
	}
	scheduler := cron.New(cron.WithParser(pushCronParser))
	_, err := scheduler.AddFunc("* * * * *", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		sent, skipped, failed := env.PushDue(ctx, env.now())
		if sent+skipped+failed > 0 {
			slog.Info("定时课表推送完成", "sent", sent, "skipped", skipped, "failed", failed)
		}
	})
	if err != nil {
		return nil, err
	}
	scheduler.Start()
	return &Scheduler{cron: scheduler}, nil
}

// Stop stops the scheduler; safe on a nil receiver.
func (s *Scheduler) Stop() {
	if s != nil && s.cron != nil {
		s.cron.Stop()
	}
}
