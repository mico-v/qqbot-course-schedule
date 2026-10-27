package schedule

import (
	"context"
	"net/url"
	"strings"
	"unicode"
)

// Send formats accepted by Settings.SendFormat.
const (
	SendFormatImage    = "image"
	SendFormatMarkdown = "markdown"
)

// Settings holds the bot-wide runtime switches edited from the WebUI.
type Settings struct {
	// Enabled is the master switch: when false the bot ignores every message.
	Enabled bool `json:"enabled"`
	// ReplyPlain answers commands without a leading slash ("课表").
	ReplyPlain bool `json:"reply_plain"`
	// ReplySlash answers commands with a leading slash ("/课表").
	ReplySlash bool `json:"reply_slash"`
	// ReplyMention answers commands that open with an @ mention.
	ReplyMention bool `json:"reply_mention"`
	// SendFormat selects how schedule cards are delivered: an image (default)
	// or a markdown list of the day's courses.
	SendFormat string `json:"send_format"`
	// Nickname is the bot name drawn on rendered cards; empty hides it.
	Nickname string `json:"nickname"`
	// BaseURL is the public service origin used for links, images and files.
	// Empty falls back to the public_base_url from config.json.
	BaseURL string `json:"base_url"`
	// ScheduleLinkTTLMinutes controls how long /修改课程表 links stay valid.
	ScheduleLinkTTLMinutes int `json:"schedule_link_ttl_minutes"`
	// AdminOpenIDs restricts chat administrator commands to these OpenIDs.
	// Empty keeps the platform group owner/administrator fallback.
	AdminOpenIDs []string `json:"admin_open_ids"`
}

// MaxBotNicknameLength caps the bot nickname drawn on cards.
const MaxBotNicknameLength = 24

// Schedule edit-link lifetime bounds.
const (
	DefaultScheduleLinkTTLMinutes = 60
	MinScheduleLinkTTLMinutes     = 5
	MaxScheduleLinkTTLMinutes     = 30 * 24 * 60
)

// DefaultSettings returns the switches used before any customization. All of
// them are on so an upgrade keeps the previous behaviour.
func DefaultSettings() Settings {
	return Settings{
		Enabled: true, ReplyPlain: true, ReplySlash: true, ReplyMention: true,
		SendFormat:             SendFormatImage,
		ScheduleLinkTTLMinutes: DefaultScheduleLinkTTLMinutes,
		AdminOpenIDs:           []string{},
	}
}

// BotSettings loads the bot switches, falling back to defaults when unset. The
// defaults are pre-filled so a stored document missing a field keeps that
// switch on instead of silently disabling it.
func (s *Service) BotSettings() (Settings, error) {
	return s.BotSettingsContext(context.Background())
}

// BotSettingsContext loads settings with request cancellation.
func (s *Service) BotSettingsContext(ctx context.Context) (Settings, error) {
	settings := DefaultSettings()
	stored, found, err := s.settings.LoadBotSettings(ctx)
	if err != nil {
		return DefaultSettings(), err
	}
	if found {
		settings = stored
	}
	settings.SendFormat = NormalizeSendFormat(settings.SendFormat)
	settings.Nickname = NormalizeNickname(settings.Nickname)
	settings.BaseURL = normalizeStoredBaseURL(settings.BaseURL)
	settings.AdminOpenIDs = NormalizeAdminOpenIDs(settings.AdminOpenIDs)
	if ttl, err := NormalizeScheduleLinkTTLMinutes(settings.ScheduleLinkTTLMinutes); err == nil {
		settings.ScheduleLinkTTLMinutes = ttl
	} else {
		settings.ScheduleLinkTTLMinutes = DefaultScheduleLinkTTLMinutes
	}
	return settings, nil
}

// SaveBotSettings stores the bot switches.
func (s *Service) SaveBotSettings(settings Settings) error {
	return s.SaveBotSettingsContext(context.Background(), settings)
}

// SaveBotSettingsContext validates and saves settings with cancellation.
func (s *Service) SaveBotSettingsContext(ctx context.Context, settings Settings) error {
	settings.SendFormat = NormalizeSendFormat(settings.SendFormat)
	settings.Nickname = NormalizeNickname(settings.Nickname)
	settings.AdminOpenIDs = NormalizeAdminOpenIDs(settings.AdminOpenIDs)
	baseURL, err := NormalizeBaseURL(settings.BaseURL)
	if err != nil {
		return err
	}
	ttl, err := NormalizeScheduleLinkTTLMinutes(settings.ScheduleLinkTTLMinutes)
	if err != nil {
		return err
	}
	settings.BaseURL = baseURL
	settings.ScheduleLinkTTLMinutes = ttl
	return s.settings.SaveBotSettings(ctx, settings)
}

// AllowsAdmin reports whether a sender may execute chat administrator
// commands. A configured list is authoritative; an empty list preserves the
// platform owner/administrator fallback for backward compatibility.
func (s Settings) AllowsAdmin(localUserID, globalUserID string, platformAdmin bool) bool {
	admins := NormalizeAdminOpenIDs(s.AdminOpenIDs)
	if len(admins) == 0 {
		return platformAdmin
	}
	localUserID = strings.TrimSpace(localUserID)
	globalUserID = strings.TrimSpace(globalUserID)
	for _, adminID := range admins {
		if adminID == localUserID || (globalUserID != "" && adminID == globalUserID) {
			return true
		}
	}
	return false
}

// NormalizeAdminOpenIDs trims, splits pasted lists, and removes duplicates
// while preserving the administrator's input order.
func NormalizeAdminOpenIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, raw := range values {
		fields := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || unicode.IsSpace(r)
		})
		for _, value := range fields {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

// NormalizeSendFormat maps unknown or absent values to the image default so an
// older stored document keeps the previous behaviour.
func NormalizeSendFormat(value string) string {
	if value == SendFormatMarkdown {
		return SendFormatMarkdown
	}
	return SendFormatImage
}

// NormalizeNickname trims the bot nickname and caps it so a stored value
// always fits the card header.
func NormalizeNickname(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > MaxBotNicknameLength {
		value = string(runes[:MaxBotNicknameLength])
	}
	return value
}

// NormalizeBaseURL validates the public service address and removes trailing
// slashes. An empty value keeps the config.json fallback.
func NormalizeBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ValidationErrorf("回调地址必须是完整的 HTTP(S) 地址，例如 https://kb.example.com。")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ValidationErrorf("回调地址不能包含查询参数或锚点。")
	}
	return value, nil
}

// NormalizeScheduleLinkTTLMinutes validates the public edit-link lifetime.
// Zero is treated as the default so older API clients keep working.
func NormalizeScheduleLinkTTLMinutes(value int) (int, error) {
	if value == 0 {
		return DefaultScheduleLinkTTLMinutes, nil
	}
	if value < MinScheduleLinkTTLMinutes || value > MaxScheduleLinkTTLMinutes {
		return 0, ValidationErrorf(
			"课表修改链接有效期必须在 %d 到 %d 分钟之间。",
			MinScheduleLinkTTLMinutes,
			MaxScheduleLinkTTLMinutes,
		)
	}
	return value, nil
}

func normalizeStoredBaseURL(value string) string {
	normalized, err := NormalizeBaseURL(value)
	if err != nil {
		return ""
	}
	return normalized
}
