package bot

import (
	"context"
	"fmt"
	"strings"
)

// nicknameCommandPrefix is the dedicated command for a member's own nickname.
const nicknameCommandPrefix = "/nickname"

// handleNickname shows or updates the sender's own display name. The name is
// stored on the member row and drawn whenever that member appears on a card.
func (h *Handler) handleNickname(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil || h.env.Service == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	if in == nil || strings.TrimSpace(in.UserOpenID) == "" {
		return r.Reply(ctx, "无法识别发送人，请稍后重试。")
	}
	scopeID := h.env.Scope(in)
	if scopeID == "" {
		return r.Reply(ctx, "无法识别当前会话，请稍后重试。")
	}
	name := strings.TrimSpace(in.Args)
	if name == "" {
		members, err := h.env.Service.ScopeMembers(scopeID)
		if err != nil {
			return r.Reply(ctx, "读取昵称失败："+err.Error())
		}
		current := "未设置"
		if member := members[in.UserOpenID]; member != nil && strings.TrimSpace(member.Name) != "" {
			current = strings.TrimSpace(member.Name)
		}
		return r.Reply(ctx, fmt.Sprintf(
			"你当前的昵称：%s。\n设置：%s <昵称>，例如 %s 小明；用 %s 清空 可恢复默认昵称。",
			current, nicknameCommandPrefix, nicknameCommandPrefix, nicknameCommandPrefix,
		))
	}
	if isClearNickname(name) {
		name = ""
	}
	saved, err := h.env.Service.SetMemberName(scopeID, in.UserOpenID, name, in.UserOpenID)
	if err != nil {
		return r.Reply(ctx, "保存昵称失败："+err.Error())
	}
	if strings.TrimSpace(saved) == "" {
		return r.Reply(ctx, "已清空你的昵称，之后将显示为默认名称。")
	}
	return r.Reply(ctx, "已将你的昵称设置为「"+saved+"」，之后渲染你的课表时会显示该昵称。")
}
