package admin

import (
	"context"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// BotSettings returns the bot-wide settings used by the admin page.
func (s *Service) BotSettings() (schedule.Settings, error) {
	return s.domain.BotSettings()
}

// BotSettingsContext returns settings with request cancellation.
func (s *Service) BotSettingsContext(ctx context.Context) (schedule.Settings, error) {
	return s.domain.BotSettingsContext(ctx)
}

// SaveBotSettings persists settings through the explicit admin use case.
func (s *Service) SaveBotSettings(settings schedule.Settings) error {
	return s.domain.SaveBotSettings(settings)
}

// SaveBotSettingsContext persists settings with request cancellation.
func (s *Service) SaveBotSettingsContext(ctx context.Context, settings schedule.Settings) error {
	return s.domain.SaveBotSettingsContext(ctx, settings)
}
