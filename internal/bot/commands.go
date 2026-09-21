package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// planned lists commands from the project plan that are not implemented yet.
var planned = []struct {
	Prefix      string
	Description string
	Milestone   string
}{
	{"/上课时长榜", "生成本会话群友上课时长排行榜", "M2"},
	{"/休假", "标记某天为休假", "M2"},
	{"/调休", "把某天的课程换成另一天的课程", "M2"},
	{"/销假", "取消休假/调休标记", "M2"},
	{"/假期", "列出当前会话的休假/调休标记", "M2"},
	{"/导出课表", "导出当前课表为 .ics 文件", "M4"},
	{"/启用推送", "允许机器人向本会话主动推送", "M4"},
	{"/关闭推送", "关闭机器人对本会话的主动推送", "M4"},
}

// NewDefaultHandler registers the M1 commands.
func NewDefaultHandler(env *Env) *Handler {
	h := NewHandler()
	h.env = env

	h.Register(&Command{
		Prefix:      "/ping",
		Description: "连通性测试",
		Handle: func(ctx context.Context, msg *Message) error {
			return msg.Reply(ctx, "pong")
		},
	})
	h.Register(&Command{
		Prefix:      "/help",
		Description: "查看指令列表",
		Handle:      h.handleHelp,
	})
	h.Register(&Command{
		Prefix:      "/帮助",
		Description: "查看指令列表",
		Handle:      h.handleHelp,
	})

	h.Register(&Command{
		Prefix:      "/今日课表",
		Description: "生成当前会话今日课程表图片",
		Handle: func(ctx context.Context, msg *Message) error {
			return handleDayCard(ctx, h.env, msg, nil)
		},
	})
	h.Register(&Command{
		Prefix:      "/明日课表",
		Description: "生成当前会话明日课程表图片",
		Handle: func(ctx context.Context, msg *Message) error {
			tomorrow := h.env.now().AddDate(0, 0, 1)
			return handleDayCard(ctx, h.env, msg, &tomorrow)
		},
	})
	h.Register(&Command{
		Prefix:      "/课表",
		Description: "查询指定日期课程表",
		Handle:      h.handleScheduleCommand,
	})
	h.Register(&Command{
		Prefix:      "/导入课表",
		Description: "导入 .ics 课程表文件",
		Handle:      h.handleImportCommand,
	})

	for _, item := range planned {
		item := item
		h.Register(&Command{
			Prefix:      item.Prefix,
			Description: item.Description,
			Handle: func(ctx context.Context, msg *Message) error {
				return msg.Reply(ctx, fmt.Sprintf("指令 %s 将在 %s 实现。", item.Prefix, item.Milestone))
			},
		})
	}
	return h
}

func (h *Handler) handleHelp(ctx context.Context, msg *Message) error {
	var lines []string
	lines = append(lines, "可用指令：")
	for _, cmd := range h.Commands() {
		if cmd.Description == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s：%s", cmd.Prefix, cmd.Description))
	}
	return msg.Reply(ctx, strings.Join(lines, "\n"))
}

func (h *Handler) handleScheduleCommand(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	args := strings.TrimSpace(strings.TrimPrefix(msg.Content, "/课表"))
	day, message := schedule.SingleDayQuery(args, h.env.now())
	if message != "" {
		return msg.Reply(ctx, message)
	}
	if day == nil {
		return handleDayCard(ctx, h.env, msg, nil)
	}
	return handleDayCard(ctx, h.env, msg, day)
}

func (h *Handler) handleImportCommand(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	for _, attachment := range msg.Attachments {
		if isICSFile(attachment) {
			return h.env.ImportICS(ctx, msg, attachment)
		}
	}
	return msg.Reply(ctx, "未检测到 .ics 文件。请发送 /导入课表 并附加 .ics 文件，或直接发送 .ics 文件。")
}

func handleDayCard(ctx context.Context, env *Env, msg *Message, day *time.Time) error {
	if env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	target := env.now()
	if day != nil {
		target = *day
	}
	url, ok, err := env.RenderDayCard(ctx, msg, target)
	if err != nil {
		return err
	}
	if !ok {
		return msg.Reply(ctx, "当前会话还没有可展示的课程表。请先发送 /导入课表 并附加 .ics 文件。")
	}
	return msg.ReplyImage(ctx, url)
}
