package schedule

// Settings holds the bot-wide runtime switches edited from the WebUI and the
// /设置 command.
type Settings struct {
	// Enabled is the master switch: when false the bot ignores every message.
	Enabled bool `json:"enabled"`
	// ReplyPlain answers commands without a leading slash ("今日课表").
	ReplyPlain bool `json:"reply_plain"`
	// ReplySlash answers commands with a leading slash ("/今日课表").
	ReplySlash bool `json:"reply_slash"`
	// ReplyMention answers commands that open with an @ mention.
	ReplyMention bool `json:"reply_mention"`
}

const (
	settingsScope     = "global"
	settingsNamespace = "settings"
	settingsKey       = "bot"
)

// DefaultSettings returns the switches used before any customization. All of
// them are on so an upgrade keeps the previous behaviour.
func DefaultSettings() Settings {
	return Settings{Enabled: true, ReplyPlain: true, ReplySlash: true, ReplyMention: true}
}

// BotSettings loads the bot switches, falling back to defaults when unset. The
// defaults are pre-filled so a stored document missing a field keeps that
// switch on instead of silently disabling it.
func (s *Service) BotSettings() (Settings, error) {
	settings := DefaultSettings()
	if _, err := s.store.GetKV(settingsScope, settingsNamespace, settingsKey, &settings); err != nil {
		return DefaultSettings(), err
	}
	return settings, nil
}

// SaveBotSettings stores the bot switches.
func (s *Service) SaveBotSettings(settings Settings) error {
	return s.store.SetKV(settingsScope, settingsNamespace, settingsKey, settings)
}
