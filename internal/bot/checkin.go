package bot

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// handleCheckin awards a random 1-10 points once per member per day.
func (h *Handler) handleCheckin(ctx context.Context, in *Inbound, r *Replier) error {
	env := h.env
	if env == nil || env.Service == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	points := rand.IntN(schedule.CheckinMaxPoints-schedule.CheckinMinPoints+1) + schedule.CheckinMinPoints
	result, err := env.Service.Checkin(env.Scope(in), in.UserOpenID, in.Username, env.now(), points)
	if err != nil {
		return r.Reply(ctx, "签到失败："+err.Error())
	}
	if result.Already {
		return r.Reply(ctx, fmt.Sprintf(
			"你今天已经签到过了，本次获得 %d 群积分。\n当前共 %d 群积分，已签到 %d 天。",
			result.Record.Points, result.Total, result.Days,
		))
	}
	return r.Reply(ctx, fmt.Sprintf(
		"签到成功，获得 %d 群积分！\n当前共 %d 群积分，已签到 %d 天。",
		result.Record.Points, result.Total, result.Days,
	))
}

// handlePoints lists the sender's total points and recent check-ins.
func (h *Handler) handlePoints(ctx context.Context, in *Inbound, r *Replier) error {
	env := h.env
	if env == nil || env.Service == nil {
		return r.Reply(ctx, "课表功能未初始化。")
	}
	status, err := env.Service.CheckinStatus(env.Scope(in), in.UserOpenID)
	if err != nil {
		return r.Reply(ctx, "读取积分失败："+err.Error())
	}
	if status.Days == 0 {
		return r.Reply(ctx, "你还没有签到记录，发送 /签到 每天可随机获得 1-10 群积分。")
	}
	lines := []string{
		fmt.Sprintf("你当前共有 %d 群积分，已签到 %d 天。", status.Total, status.Days),
		"最近签到：",
	}
	for _, record := range status.Records {
		lines = append(lines, fmt.Sprintf("- %s：+%d", record.Day, record.Points))
	}
	return r.Reply(ctx, strings.Join(lines, "\n"))
}
