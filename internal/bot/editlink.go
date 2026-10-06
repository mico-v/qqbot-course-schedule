package bot

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// handleEditScheduleCommand issues a short-lived public link for the sender's
// own schedule in the current scope.
func (h *Handler) handleEditScheduleCommand(ctx context.Context, in *Inbound, r *Replier) error {
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
	if err := h.env.Service.EnsureMember(scopeID, in.UserOpenID, in.Username); err != nil {
		return r.Reply(ctx, "准备课表失败："+err.Error())
	}

	settings, err := h.env.Service.BotSettings()
	if err != nil {
		return r.Reply(ctx, "读取设置失败："+err.Error())
	}
	baseURL := h.env.PublicBaseURLForLinks()
	if baseURL == "" {
		return r.Reply(ctx, "尚未配置服务回调地址，请联系管理员在设置中填写。")
	}
	ttl := time.Duration(settings.ScheduleLinkTTLMinutes) * time.Minute
	token, expiresAt, err := h.env.Service.CreateScheduleEditLink(scopeID, in.UserOpenID, ttl, h.env.now())
	if err != nil {
		return r.Reply(ctx, "生成修改链接失败："+err.Error())
	}
	link := fmt.Sprintf(
		"%s/schedule/edit?token=%s",
		strings.TrimRight(baseURL, "/"),
		url.QueryEscape(token),
	)
	return r.Reply(ctx, fmt.Sprintf(
		"课程表修改链接：\n%s\n\n链接将在 %s 前有效，过期后请重新发送 /修改课程表。",
		link,
		expiresAt.In(schedule.LocalTZ).Format("2006-01-02 15:04"),
	))
}
