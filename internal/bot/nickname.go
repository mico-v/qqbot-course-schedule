package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// nicknameCommandPrefix is the dedicated command for the bot nickname.
const nicknameCommandPrefix = "/nikname"

// handleNickname shows or updates the bot nickname drawn on cards.
func (h *Handler) handleNickname(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil || h.env.Service == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	settings, err := h.env.Service.BotSettings()
	if err != nil {
		return r.Reply(ctx, "读取设置失败："+err.Error())
	}
	name := strings.TrimSpace(in.Args)
	if name == "" {
		// Anyone may read the current name; only managers may change it.
		current := settings.Nickname
		if current == "" {
			current = "未设置"
		}
		return r.Reply(ctx, fmt.Sprintf(
			"当前机器人昵称：%s。\n设置：%s <昵称>，例如 %s 课表小助手；用 %s 清空 可恢复默认。",
			current, nicknameCommandPrefix, nicknameCommandPrefix, nicknameCommandPrefix,
		))
	}
	if !canManageSettings(in) {
		return r.Reply(ctx, "只有群管理员或私聊可以修改机器人昵称。")
	}
	if isClearNickname(name) {
		name = ""
	}
	if name != "" && len([]rune(name)) > schedule.MaxBotNicknameLength {
		return r.Reply(ctx, fmt.Sprintf("昵称不能超过 %d 个字符。", schedule.MaxBotNicknameLength))
	}
	settings.Nickname = name
	if err := h.env.Service.SaveBotSettings(settings); err != nil {
		return r.Reply(ctx, "保存昵称失败："+err.Error())
	}
	if name == "" {
		return r.Reply(ctx, "已清空机器人昵称，卡片将不再显示名称。")
	}
	return r.Reply(ctx, "已将机器人昵称设置为「"+name+"」，之后生成的课表卡片会显示该昵称。")
}
