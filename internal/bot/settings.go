package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// settingsCommandPrefix is the command that may bypass the master switch so an
// administrator can turn the bot back on from chat.
const settingsCommandPrefix = "/设置"

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

// canManageSettings reports whether the sender may change the switches.
func canManageSettings(in *Inbound) bool {
	if in == nil {
		return false
	}
	return in.Origin == OriginPrivate || in.IsAdmin()
}

func (h *Handler) handleSettings(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil || h.env.Service == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	if !canManageSettings(in) {
		return r.Reply(ctx, "只有群管理员或私聊可以修改机器人设置。")
	}
	settings, err := h.env.Service.BotSettings()
	if err != nil {
		return r.Reply(ctx, "读取设置失败："+err.Error())
	}
	args := strings.Fields(in.Args)
	if len(args) == 0 {
		return r.Reply(ctx, settingsText(settings))
	}
	updated, problem := applySettingArgs(settings, args)
	if problem != "" {
		return r.Reply(ctx, problem)
	}
	if err := h.env.Service.SaveBotSettings(updated); err != nil {
		return r.Reply(ctx, "保存设置失败："+err.Error())
	}
	return r.Reply(ctx, "已更新。\n"+settingsText(updated))
}

// applySettingArgs parses one /设置 invocation. It returns a usage/error text
// (empty on success) alongside the updated settings.
func applySettingArgs(settings schedule.Settings, args []string) (schedule.Settings, string) {
	usage := fmt.Sprintf("用法：\n%s\n%s 机器人 开|关\n%s 回复 无斜杠|斜杠|@|全部 开|关\n%s 格式 图片|markdown\n%s 昵称 <名字|清空>",
		settingsCommandPrefix, settingsCommandPrefix, settingsCommandPrefix, settingsCommandPrefix, settingsCommandPrefix)

	switch args[0] {
	case "机器人", "bot":
		if len(args) != 2 {
			return settings, usage
		}
		value, ok := parseOnOff(args[1])
		if !ok {
			return settings, usage
		}
		settings.Enabled = value
		return settings, ""
	case "回复", "reply":
		if len(args) != 3 {
			return settings, usage
		}
		value, ok := parseOnOff(args[2])
		if !ok {
			return settings, usage
		}
		switch args[1] {
		case "无斜杠", "plain":
			settings.ReplyPlain = value
		case "斜杠", "slash":
			settings.ReplySlash = value
		case "@", "at", "mention":
			settings.ReplyMention = value
		case "全部", "all":
			settings.ReplyPlain, settings.ReplySlash, settings.ReplyMention = value, value, value
		default:
			return settings, "未知的回复类型：" + args[1] + "（可选 无斜杠 / 斜杠 / @ / 全部）"
		}
		return settings, ""
	case "格式", "format":
		if len(args) != 2 {
			return settings, usage
		}
		switch strings.ToLower(args[1]) {
		case "图片", "image", "img":
			settings.SendFormat = schedule.SendFormatImage
		case "markdown", "md", "列表":
			settings.SendFormat = schedule.SendFormatMarkdown
		default:
			return settings, "未知的发送格式：" + args[1] + "（可选 图片 / markdown）"
		}
		return settings, ""
	case "昵称", "nickname", "nikname":
		if len(args) < 2 {
			return settings, usage
		}
		name := strings.Join(args[1:], " ")
		if isClearNickname(name) {
			settings.Nickname = ""
			return settings, ""
		}
		if len([]rune(name)) > schedule.MaxBotNicknameLength {
			return settings, fmt.Sprintf("昵称不能超过 %d 个字符。", schedule.MaxBotNicknameLength)
		}
		settings.Nickname = name
		return settings, ""
	}
	if len(args) == 1 {
		if value, ok := parseOnOff(args[0]); ok {
			settings.Enabled = value
			return settings, ""
		}
	}
	return settings, usage
}

func parseOnOff(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "开", "on", "true", "1", "yes", "启用":
		return true, true
	case "关", "off", "false", "0", "no", "禁用":
		return false, true
	}
	return false, false
}

// isClearNickname reports whether a nickname argument asks to clear the name.
func isClearNickname(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "清空", "无", "清除", "clear", "none", "-":
		return true
	}
	return false
}

func settingsText(settings schedule.Settings) string {
	onOff := func(value bool) string {
		if value {
			return "开"
		}
		return "关"
	}
	sendFormat := "图片"
	if settings.SendFormat == schedule.SendFormatMarkdown {
		sendFormat = "markdown"
	}
	nickname := settings.Nickname
	if nickname == "" {
		nickname = "未设置"
	}
	return fmt.Sprintf(
		"机器人：%s\n回复策略：无斜杠 %s · 斜杠 %s · @机器人 %s\n发送格式：%s\n机器人昵称：%s\n\n修改：\n%s 机器人 开|关\n%s 回复 无斜杠|斜杠|@|全部 开|关\n%s 格式 图片|markdown\n%s 昵称 <名字|清空>",
		onOff(settings.Enabled),
		onOff(settings.ReplyPlain), onOff(settings.ReplySlash), onOff(settings.ReplyMention),
		sendFormat,
		nickname,
		settingsCommandPrefix, settingsCommandPrefix, settingsCommandPrefix, settingsCommandPrefix,
	)
}
