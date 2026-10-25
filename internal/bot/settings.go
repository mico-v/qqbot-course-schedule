package bot

import (
	"log/slog"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// replyMode classifies how an inbound command was addressed to the bot.
type replyMode string

const (
	replyModePlain   replyMode = "无斜杠"
	replyModeSlash   replyMode = "斜杠"
	replyModeMention replyMode = "@机器人"
)

// classifyReplyMode decides which switch governs a message. A leading mention
// wins over the slash so "@bot /课表" follows the mention switch.
func classifyReplyMode(mentioned, slash bool) replyMode {
	switch {
	case mentioned:
		return replyModeMention
	case slash:
		return replyModeSlash
	default:
		return replyModePlain
	}
}

// allowsReply reports whether the settings permit answering the given mode.
func allowsReply(settings schedule.Settings, mode replyMode) bool {
	switch mode {
	case replyModeMention:
		return settings.ReplyMention
	case replyModeSlash:
		return settings.ReplySlash
	default:
		return settings.ReplyPlain
	}
}

// currentSettings loads the switches, falling back to defaults on any error so
// a storage hiccup never silences the bot unexpectedly.
func (e *Env) currentSettings() schedule.Settings {
	if e == nil || e.Service == nil {
		return schedule.DefaultSettings()
	}
	settings, err := e.Service.BotSettings()
	if err != nil {
		slog.Warn("读取机器人设置失败，使用默认值", "err", err)
		return schedule.DefaultSettings()
	}
	return settings
}

func (h *Handler) currentSettings() schedule.Settings {
	if h == nil {
		return schedule.DefaultSettings()
	}
	return h.env.currentSettings()
}

// isAdmin applies the WebUI administrator allowlist to chat commands.
func (h *Handler) isAdmin(in *Inbound) bool {
	if in == nil {
		return false
	}
	return h.currentSettings().AllowsAdmin(in.UserOpenID, in.GlobalUserOpenID, in.IsAdmin())
}

// isClearNickname reports whether a nickname argument asks to clear the name.
func isClearNickname(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "清空", "无", "清除", "clear", "none", "-":
		return true
	}
	return false
}
