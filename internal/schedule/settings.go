package schedule

// Send formats accepted by Settings.SendFormat.
const (
	SendFormatImage    = "image"
	SendFormatMarkdown = "markdown"
)

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
	// SendFormat selects how schedule cards are delivered: an image (default)
	// or a markdown list of the day's courses.
	SendFormat string `json:"send_format"`
}

const settingsKey = "bot"

// DefaultSettings returns the switches used before any customization. All of
// them are on so an upgrade keeps the previous behaviour.
func DefaultSettings() Settings {
	return Settings{
		Enabled: true, ReplyPlain: true, ReplySlash: true, ReplyMention: true,
		SendFormat: SendFormatImage,
	}
}

// BotSettings loads the bot switches, falling back to defaults when unset. The
// defaults are pre-filled so a stored document missing a field keeps that
// switch on instead of silently disabling it.
func (s *Service) BotSettings() (Settings, error) {
	settings := DefaultSettings()
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceSettings, settingsKey, &settings); err != nil {
		return DefaultSettings(), err
	}
	settings.SendFormat = NormalizeSendFormat(settings.SendFormat)
	return settings, nil
}

// SaveBotSettings stores the bot switches.
func (s *Service) SaveBotSettings(settings Settings) error {
	settings.SendFormat = NormalizeSendFormat(settings.SendFormat)
	return s.store.SetKV(KVScopeGlobal, KVNamespaceSettings, settingsKey, settings)
}

// NormalizeSendFormat maps unknown or absent values to the image default so an
// older stored document keeps the previous behaviour.
func NormalizeSendFormat(value string) string {
	if value == SendFormatMarkdown {
		return SendFormatMarkdown
	}
	return SendFormatImage
}
