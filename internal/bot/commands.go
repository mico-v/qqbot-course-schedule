package bot

import (
	"context"
	"fmt"
	"strings"
)

// planned lists commands from the project plan that are not implemented yet.
// They stay registered so the M0 skeleton can be verified end to end.
var planned = []struct {
	Prefix      string
	Description string
	Milestone   string
}{
	{"/今日课表", "生成当前会话今日课程表图片", "M1"},
	{"/明日课表", "生成当前会话明日课程表图片", "M1"},
	{"/课表", "查询指定日期课程表", "M1"},
	{"/上课时长榜", "生成本会话群友上课时长排行榜", "M3"},
	{"/休假", "标记某天为休假", "M3"},
	{"/调休", "把某天的课程换成另一天的课程", "M3"},
	{"/销假", "取消休假/调休标记", "M3"},
	{"/假期", "列出当前会话的休假/调休标记", "M3"},
	{"/导入课表", "导入 .ics 课程表文件", "M1"},
	{"/导出课表", "导出当前课表为 .ics 文件", "M6"},
	{"/启用推送", "允许机器人向本会话主动推送", "M5"},
	{"/关闭推送", "关闭机器人对本会话的主动推送", "M5"},
}

// NewDefaultHandler registers the M0 commands.
func NewDefaultHandler() *Handler {
	h := NewHandler()

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

	for _, item := range planned {
		item := item
		h.Register(&Command{
			Prefix:      item.Prefix,
			Description: item.Description,
			Handle: func(ctx context.Context, msg *Message) error {
				return msg.Reply(ctx, fmt.Sprintf("指令 %s 将在 %s 实现，当前为 M0 骨架。", item.Prefix, item.Milestone))
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
