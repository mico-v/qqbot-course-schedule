package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/wakeup"
)

// NewDefaultHandler registers the implemented commands.
func NewDefaultHandler(env *Env) *Handler {
	h := NewHandler()
	h.env = env

	h.Register(&Command{
		Prefix:      "/ping",
		Description: "连通性测试",
		Handle: func(ctx context.Context, in *Inbound, r *Replier) error {
			return r.Reply(ctx, "pong")
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
		Handle: func(ctx context.Context, in *Inbound, r *Replier) error {
			return handleDayCard(ctx, h.env, in, r, nil)
		},
	})
	h.Register(&Command{
		Prefix:      "/明日课表",
		Description: "生成当前会话明日课程表图片",
		Ready:       true,
		Handle: func(ctx context.Context, in *Inbound, r *Replier) error {
			tomorrow := h.env.now().AddDate(0, 0, 1)
			return handleDayCard(ctx, h.env, in, r, &tomorrow)
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
		Description: "导入 .ics 课表文件或 WakeUp 分享口令",
		Ready:       true,
		Handle:      h.handleImportCommand,
	})
	h.Register(&Command{
		Prefix:      "/修改课程表",
		Description: "生成仅可修改自己课表的临时链接",
		Ready:       true,
		Handle:      h.handleEditScheduleCommand,
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
		Handle: func(ctx context.Context, in *Inbound, r *Replier) error {
			return h.handleOverrideSet(ctx, in, r, schedule.DayOverrideHoliday)
		},
	})
	h.Register(&Command{
		Prefix:      "/调休",
		Aliases:     []string{"/补课", "/调课"},
		Description: "把某天的课程换成另一天的课程",
		Ready:       true,
		Handle: func(ctx context.Context, in *Inbound, r *Replier) error {
			return h.handleOverrideSet(ctx, in, r, schedule.DayOverrideShift)
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
		Prefix:      "/导出课表",
		Description: "导出当前课表为 .ics 文件",
		Ready:       true,
		Handle:      h.handleExportCommand,
	})
	h.Register(&Command{
		Prefix:      "/绑定QQ",
		Description: "绑定你的 QQ 号以显示真实头像",
		Aliases:     []string{"/绑定qq"},
		Ready:       true,
		Handle:      h.handleBindQQ,
	})
	h.Register(&Command{
		Prefix:      "/解绑QQ",
		Description: "解除已绑定的 QQ 号",
		Aliases:     []string{"/解绑qq"},
		Ready:       true,
		Handle:      h.handleUnbindQQ,
	})
	h.Register(&Command{
		Prefix:      "/设置",
		Description: "查看或修改机器人设置（管理员）",
		Aliases:     []string{"/配置", "/settings"},
		Ready:       true,
		Handle:      h.handleSettings,
	})
	h.Register(&Command{
		Prefix:      nicknameCommandPrefix,
		Aliases:     []string{"/nickname", "/昵称"},
		Description: "设置机器人昵称（管理员）",
		Ready:       true,
		Handle:      h.handleNickname,
	})
	h.Register(&Command{
		Prefix:      "/签到",
		Aliases:     []string{"/打卡"},
		Description: "每日签到随机获得 1-10 群积分",
		Ready:       true,
		Handle:      h.handleCheckin,
	})
	h.Register(&Command{
		Prefix:      "/积分",
		Aliases:     []string{"/我的积分", "/积分记录"},
		Description: "查看群积分与签到记录",
		Ready:       true,
		Handle:      h.handlePoints,
	})
	h.Register(&Command{
		Prefix:      "/同步面板",
		Description: "同步机器人指令面板（管理员）",
		Handle:      h.handleSyncPanelCommand,
	})

	return h
}

func (h *Handler) handleHelp(ctx context.Context, in *Inbound, r *Replier) error {
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
	return r.Reply(ctx, strings.Join(lines, "\n"))
}

func (h *Handler) handleScheduleCommand(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	day, message := schedule.SingleDayQuery(in.Args, h.env.now())
	if message != "" {
		return r.Reply(ctx, message)
	}
	if day == nil {
		return handleDayCard(ctx, h.env, in, r, nil)
	}
	return handleDayCard(ctx, h.env, in, r, day)
}

func (h *Handler) handleImportCommand(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	for _, attachment := range in.Attachments {
		if isICSFile(attachment) {
			return h.env.ImportICS(ctx, in, r, attachment)
		}
	}
	if code := wakeup.ExtractShareCode(in.Args); code != "" {
		return h.env.ImportWakeUp(ctx, in, r, code)
	}
	return r.Reply(ctx, "未检测到 .ics 文件或 WakeUp 分享口令。可以附加 .ics 文件，或发送 /导入课表 <WakeUp分享口令>。")
}

func (h *Handler) handleRankCommand(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	card, ok, err := h.env.RenderRankCard(ctx, in, r, in.Args)
	if err != nil {
		return r.Reply(ctx, err.Error())
	}
	if !ok {
		return r.Reply(ctx, "当前会话还没有可统计的课程。")
	}
	return h.env.SendCard(ctx, in, r, card, cardKeyboardForRank(in.UserOpenID))
}

func (h *Handler) handleOverrideSet(ctx context.Context, in *Inbound, r *Replier, kind string) error {
	env := h.env
	if env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	today := env.now()
	days, rest, err := schedule.SplitDayOverrideArgs(in.Args, today, schedule.RollForward)
	if err != nil {
		return r.Reply(ctx, err.Error())
	}
	person := strings.Join(rest, " ")
	if len(days) == 0 {
		if kind == schedule.DayOverrideShift {
			return r.Reply(ctx, "请提供两个日期：/调休 <被覆盖的日期> <来源日期> [成员]，例如 /调休 2026-10-11 2026-10-08 表示 10 月 11 日按 10 月 8 日的课程上课。")
		}
		return r.Reply(ctx, "请提供日期：/休假 <日期> [成员]，例如 /休假 2026-10-01，也可以使用 今天、明天 或 10月1日至10月8日。")
	}
	var sourceDay *time.Time
	if kind == schedule.DayOverrideShift {
		if len(days) < 2 {
			return r.Reply(ctx, "调休需要来源日期：/调休 <被覆盖的日期> <来源日期> [成员]。")
		}
		if len(days) > 2 {
			return r.Reply(ctx, "调休一次只能指定一个日期和一个来源日期，例如 /调休 2026-10-11 2026-10-08。")
		}
		sourceDay = &days[1]
		days = days[:1]
	}
	scope := env.Scope(in)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	targets, errMsg := schedule.ResolveOverrideTargets(members, in.UserOpenID, in.Origin == OriginGroup, in.IsAdmin(), person, toScheduleMentions(in.Mentions))
	if errMsg != "" {
		return r.Reply(ctx, errMsg)
	}
	text, err := env.Service.SetDayOverrides(scope, targets, days, kind, sourceDay, in.UserOpenID, members, today)
	if err != nil {
		return r.Reply(ctx, err.Error())
	}
	return r.Reply(ctx, text)
}

func (h *Handler) handleCancelDayOffCommand(ctx context.Context, in *Inbound, r *Replier) error {
	env := h.env
	if env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	days, rest, err := schedule.SplitDayOverrideArgs(in.Args, env.now(), schedule.RollForward)
	if err != nil {
		return r.Reply(ctx, err.Error())
	}
	if len(days) == 0 {
		return r.Reply(ctx, "请提供日期：/销假 <日期> [成员]，例如 /销假 2026-10-01。")
	}
	person := strings.Join(rest, " ")
	scope := env.Scope(in)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	targets, errMsg := schedule.ResolveOverrideTargets(members, in.UserOpenID, in.Origin == OriginGroup, in.IsAdmin(), person, toScheduleMentions(in.Mentions))
	if errMsg != "" {
		return r.Reply(ctx, errMsg)
	}
	text, err := env.Service.ClearDayOverrides(scope, targets, days, members)
	if err != nil {
		return r.Reply(ctx, err.Error())
	}
	return r.Reply(ctx, text)
}

func (h *Handler) handleDayOffListCommand(ctx context.Context, in *Inbound, r *Replier) error {
	env := h.env
	if env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	scope := env.Scope(in)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	text, err := env.Service.DayOverrideListText(scope, members, env.now())
	if err != nil {
		return err
	}
	return r.Reply(ctx, text)
}

func (h *Handler) handleExportCommand(ctx context.Context, in *Inbound, r *Replier) error {
	env := h.env
	if env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	scope := env.Scope(in)
	members, err := env.Service.ScopeMembers(scope)
	if err != nil {
		return err
	}
	targetID, errMsg := schedule.ResolveMemberTarget(members, in.UserOpenID, in.Origin == OriginGroup, in.IsAdmin(), in.Args, toScheduleMentions(in.Mentions))
	if errMsg != "" {
		return r.Reply(ctx, errMsg)
	}
	member, ok := members[targetID]
	if !ok || member == nil {
		return r.Reply(ctx, "没有找到该成员的课程表。")
	}
	if len(member.Events) == 0 {
		return r.Reply(ctx, "该成员还没有课程，暂无可导出的课表。")
	}
	content := strings.TrimSpace(member.ICS)
	if content == "" {
		content = schedule.SerializeScheduleICS(member.Events, "", member.Name)
	}
	url, err := env.SavePublicFile(content, ".ics")
	if err != nil {
		return r.Reply(ctx, "导出失败："+err.Error())
	}
	return r.ReplyFile(ctx, url, exportFileName(member.Name))
}

func (h *Handler) handleBindQQ(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	scope := h.env.Scope(in)
	members, err := h.env.Service.ScopeMembers(scope)
	if err != nil {
		return r.Reply(ctx, "读取成员失败："+err.Error())
	}
	member, ok := members[in.UserOpenID]
	if !ok || member == nil {
		return r.Reply(ctx, "你还没有课表，请先发送 /导入课表 导入，或让管理员在管理台创建。")
	}
	argument := strings.TrimSpace(in.Args)
	if argument == "" {
		current := "未绑定"
		if member.QQ != "" {
			current = member.QQ
		}
		return r.Reply(ctx, "你当前绑定的 QQ 号："+current+"。\n绑定：/绑定QQ 123456789（绑定后卡片会显示该 QQ 的头像）。")
	}
	qq, err := h.env.Service.SetMemberQQ(scope, in.UserOpenID, argument, in.UserOpenID)
	if err != nil {
		return r.Reply(ctx, "绑定失败："+err.Error())
	}
	return r.Reply(ctx, "已绑定 QQ "+qq+"，之后生成的课表卡片会使用该 QQ 的头像。")
}

func (h *Handler) handleUnbindQQ(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	scope := h.env.Scope(in)
	members, err := h.env.Service.ScopeMembers(scope)
	if err != nil {
		return r.Reply(ctx, "读取成员失败："+err.Error())
	}
	if member, ok := members[in.UserOpenID]; !ok || member == nil {
		return r.Reply(ctx, "你还没有课表。")
	} else if member.QQ == "" {
		return r.Reply(ctx, "你还没有绑定 QQ 号。")
	}
	if _, err := h.env.Service.SetMemberQQ(scope, in.UserOpenID, "", in.UserOpenID); err != nil {
		return r.Reply(ctx, "解绑失败："+err.Error())
	}
	return r.Reply(ctx, "已解除 QQ 绑定，卡片将恢复为昵称首字头像。")
}

func (h *Handler) handleSyncPanelCommand(ctx context.Context, in *Inbound, r *Replier) error {
	if h.env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	if !in.IsAdmin() {
		return r.Reply(ctx, "只有群管理员可以同步指令面板。")
	}
	created, updated, err := SyncPanels(ctx, h.env, h)
	if err != nil {
		return r.Reply(ctx, "同步指令面板失败："+err.Error())
	}
	return r.Reply(ctx, fmt.Sprintf("指令面板已同步：新建 %d 个，更新 %d 个。", created, updated))
}

func handleDayCard(ctx context.Context, env *Env, in *Inbound, r *Replier, day *time.Time) error {
	if env == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	target := env.now()
	if day != nil {
		target = *day
	}
	if env.currentSettings().SendFormat == schedule.SendFormatMarkdown {
		text, ok, err := env.RenderDayMarkdown(in, target)
		if err != nil {
			return err
		}
		if !ok {
			return r.Reply(ctx, "当前会话还没有可展示的课程表。请先发送 /导入课表 并附加 .ics 文件。")
		}
		return env.SendDayMarkdown(ctx, in, r, text)
	}
	card, ok, err := env.RenderDayCard(ctx, in, r, target)
	if err != nil {
		return err
	}
	if !ok {
		return r.Reply(ctx, "当前会话还没有可展示的课程表。请先发送 /导入课表 并附加 .ics 文件。")
	}
	keyboard := cardKeyboardForDay(target.Format("2006-01-02"), env.now().Format("2006-01-02"), in.UserOpenID)
	return env.SendCard(ctx, in, r, card, keyboard)
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
