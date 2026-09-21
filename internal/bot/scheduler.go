package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler runs the daily push job.
type Scheduler struct {
	cron *cron.Cron
}

// StartScheduler registers the daily push on a 5-field cron expression and
// starts it. The expression is interpreted in the server's local timezone.
func StartScheduler(env *Env, spec string) (*Scheduler, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	scheduler := cron.New(cron.WithParser(parser))
	_, err := scheduler.AddFunc(spec, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		sent, skipped, failed := env.PushDaily(ctx)
		slog.Info("每日课表推送完成", "sent", sent, "skipped", skipped, "failed", failed)
	})
	if err != nil {
		return nil, fmt.Errorf("push_cron 无效: %w", err)
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
