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
	{"/导出课表", "导出当前课表为 .ics 文件", "M5"},
}

// NewDefaultHandler registers the implemented commands.
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
		Aliases:     []string{"/帮助"},
		Description: "查看指令列表",
		Handle:      h.handleHelp,
		Ready:       true,
	})

	h.Register(&Command{
		Prefix:      "/今日课表",
		Description: "生成当前会话今日课程表图片",
		Ready:       true,
		Handle: func(ctx context.Context, msg *Message) error {
			return handleDayCard(ctx, h.env, msg, nil)
		},
	})
	h.Register(&Command{
		Prefix:      "/明日课表",
		Description: "生成当前会话明日课程表图片",
		Ready:       true,
		Handle: func(ctx context.Context, msg *Message) error {
			tomorrow := h.env.now().AddDate(0, 0, 1)
			return handleDayCard(ctx, h.env, msg, &tomorrow)
		},
	})
	h.Register(&Command{
		Prefix:      "/课表",
		Description: "查询指定日期课程表",
		Ready:       true,
		Handle:      h.handleScheduleCommand,
	})
	h.Register(&Command{
		Prefix:      "/导入课表",
		Description: "导入 .ics 课程表文件",
		Ready:       true,
		Handle:      h.handleImportCommand,
	})
	h.Register(&Command{
		Prefix:      "/上课时长榜",
		Aliases:     []string{"/上课排行", "/本周上课排行", "/学习时长榜"},
		Description: "生成本会话群友上课时长排行榜",
		Ready:       true,
		Handle:      h.handleRankCommand,
	})
	h.Register(&Command{
		Prefix:      "/休假",
		Aliases:     []string{"/放假"},
		Description: "标记某天为休假",
		Ready:       true,
		Handle: func(ctx context.Context, msg *Message) error {
			return h.handleOverrideSet(ctx, msg, schedule.DayOverrideHoliday)
		},
	})
	h.Register(&Command{
		Prefix:      "/调休",
		Aliases:     []string{"/补课", "/调课"},
		Description: "把某天的课程换成另一天的课程",
		Ready:       true,
		Handle: func(ctx context.Context, msg *Message) error {
			return h.handleOverrideSet(ctx, msg, schedule.DayOverrideShift)
		},
	})
	h.Register(&Command{
		Prefix:      "/销假",
		Aliases:     []string{"/取消休假", "/取消调休"},
		Description: "取消休假/调休标记",
		Ready:       true,
		Handle:      h.handleCancelDayOffCommand,
	})
	h.Register(&Command{
		Prefix:      "/假期",
		Aliases:     []string{"/假期列表", "/调休列表", "/休假列表"},
		Description: "列出当前会话的休假/调休标记",
		Ready:       true,
		Handle:      h.handleDayOffListCommand,
	})
	h.Register(&Command{
		Prefix:      "/启用推送",
		Description: "允许机器人向本会话主动推送",
		Ready:       true,
		Handle:      h.handleEnablePush,
	})
	h.Register(&Command{
		Prefix:      "/关闭推送",
		Description: "关闭机器人对本会话的主动推送",
		Ready:       true,
		Handle:      h.handleDisablePush,
	})
	h.Register(&Command{
		Prefix:      "/同步面板",
		Description: "同步机器人指令面板（管理员）",
		Handle:      h.handleSyncPanelCommand,
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
		line := fmt.Sprintf("- %s：%s", cmd.Prefix, cmd.Description)
		if len(cmd.Aliases) > 0 {
			line += "（别名：" + strings.Join(cmd.Aliases, " ") + "）"
		}
		lines = append(lines, line)
	}
	return msg.Reply(ctx, strings.Join(lines, "\n"))
}

func (h *Handler) handleScheduleCommand(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	day, message := schedule.SingleDayQuery(msg.Args, h.env.now())
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

func (h *Handler) handleRankCommand(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	url, ok, err := h.env.RenderRankCard(ctx, msg, msg.Args)
	if err != nil {
		return msg.Reply(ctx, err.Error())
	}
	if !ok {
		return msg.Reply(ctx, "当前会话还没有可统计的课程。")
	}
	return h.env.SendCard(ctx, msg, url, cardKeyboardForRank(msg.UserOpenID))
}

func (h *Handler) handleOverrideSet(ctx context.Context, msg *Message, kind string) error {
	env := h.env
	if env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	today := env.now()
	days, rest, err := schedule.SplitDayOverrideArgs(msg.Args, today, schedule.RollForward)
	if err != nil {
		return msg.Reply(ctx, err.Error())
	}
	person := strings.Join(rest, " ")
	if len(days) == 0 {
		if kind == schedule.DayOverrideShift {
			return msg.Reply(ctx, "请提供两个日期：/调休 <被覆盖的日期> <来源日期> [成员]，例如 /调休 2026-10-11 2026-10-08 表示 10 月 11 日按 10 月 8 日的课程上课。")
		}
		return msg.Reply(ctx, "请提供日期：/休假 <日期> [成员]，例如 /休假 2026-10-01，也可以使用 今天、明天 或 10月1日至10月8日。")
	}
	var sourceDay *time.Time
	if kind == schedule.DayOverrideShift {
		if len(days) < 2 {
			return msg.Reply(ctx, "调休需要来源日期：/调休 <被覆盖的日期> <来源日期> [成员]。")
		}
		if len(days) > 2 {
			return msg.Reply(ctx, "调休一次只能指定一个日期和一个来源日期，例如 /调休 2026-10-11 2026-10-08。")
		}
		sourceDay = &days[1]
		days = days[:1]
	}
	scope := env.Scope(msg)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	targets, errMsg := schedule.ResolveOverrideTargets(members, msg.UserOpenID, msg.Origin == OriginGroup, msg.IsAdmin(), person, toScheduleMentions(msg.Mentions))
	if errMsg != "" {
		return msg.Reply(ctx, errMsg)
	}
	text, err := env.Service.SetDayOverrides(scope, targets, days, kind, sourceDay, msg.UserOpenID, members, today)
	if err != nil {
		return msg.Reply(ctx, err.Error())
	}
	return msg.Reply(ctx, text)
}

func (h *Handler) handleCancelDayOffCommand(ctx context.Context, msg *Message) error {
	env := h.env
	if env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	days, rest, err := schedule.SplitDayOverrideArgs(msg.Args, env.now(), schedule.RollForward)
	if err != nil {
		return msg.Reply(ctx, err.Error())
	}
	if len(days) == 0 {
		return msg.Reply(ctx, "请提供日期：/销假 <日期> [成员]，例如 /销假 2026-10-01。")
	}
	person := strings.Join(rest, " ")
	scope := env.Scope(msg)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	targets, errMsg := schedule.ResolveOverrideTargets(members, msg.UserOpenID, msg.Origin == OriginGroup, msg.IsAdmin(), person, toScheduleMentions(msg.Mentions))
	if errMsg != "" {
		return msg.Reply(ctx, errMsg)
	}
	text, err := env.Service.ClearDayOverrides(scope, targets, days, members)
	if err != nil {
		return msg.Reply(ctx, err.Error())
	}
	return msg.Reply(ctx, text)
}

func (h *Handler) handleDayOffListCommand(ctx context.Context, msg *Message) error {
	env := h.env
	if env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	scope := env.Scope(msg)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	text, err := env.Service.DayOverrideListText(scope, members, env.now())
	if err != nil {
		return err
	}
	return msg.Reply(ctx, text)
}

func (h *Handler) handleEnablePush(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	if msg.Origin == OriginGroup && !msg.IsAdmin() {
		return msg.Reply(ctx, "只有群管理员可以开启本群的主动推送。")
	}
	sub := PushSubscription{
		Enabled:   true,
		Origin:    "private",
		OpenID:    msg.UserOpenID,
		EnabledBy: msg.UserOpenID,
		EnabledAt: schedule.NowISO(),
	}
	if msg.Origin == OriginGroup {
		sub.Origin = "group"
		sub.OpenID = msg.GroupOpenID
	}
	if err := h.env.SetPushSubscription(h.env.Scope(msg), sub); err != nil {
		return msg.Reply(ctx, "开启失败："+err.Error())
	}
	extra := ""
	if msg.Origin == OriginGroup {
		extra = "；群聊还需群管理员在机器人资料页打开「消息推送」，否则平台会拒绝"
	}
	return msg.Reply(ctx, "已开启每日课表推送（"+pushTimeText(h.env.PushCron)+"）"+extra+"。")
}

func (h *Handler) handleDisablePush(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	if msg.Origin == OriginGroup && !msg.IsAdmin() {
		return msg.Reply(ctx, "只有群管理员可以关闭本群的主动推送。")
	}
	if err := h.env.RemovePushSubscription(h.env.Scope(msg)); err != nil {
		return msg.Reply(ctx, "关闭失败："+err.Error())
	}
	return msg.Reply(ctx, "已关闭本会话的每日课表推送。")
}

func (h *Handler) handleSyncPanelCommand(ctx context.Context, msg *Message) error {
	if h.env == nil {
		return msg.Reply(ctx, "课表功能未初始化。")
	}
	if !msg.IsAdmin() {
		return msg.Reply(ctx, "只有群管理员可以同步指令面板。")
	}
	created, updated, err := SyncPanels(ctx, h.env, h)
	if err != nil {
		return msg.Reply(ctx, "同步指令面板失败："+err.Error())
	}
	return msg.Reply(ctx, fmt.Sprintf("指令面板已同步：新建 %d 个，更新 %d 个。", created, updated))
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
	keyboard := cardKeyboardForDay(target.Format("2006-01-02"), env.now().Format("2006-01-02"), msg.UserOpenID)
	return env.SendCard(ctx, msg, url, keyboard)
}

func toScheduleMentions(mentions []Mention) []schedule.Mention {
	if len(mentions) == 0 {
		return nil
	}
	result := make([]schedule.Mention, 0, len(mentions))
	for _, mention := range mentions {
		result = append(result, schedule.Mention{ID: mention.ID, Name: mention.Name})
	}
	return result
}
