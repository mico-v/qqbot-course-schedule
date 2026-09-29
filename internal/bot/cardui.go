package bot

import (
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// cardKeyboardForDay builds 前一天 / 今天 / 后一天 callback buttons.
func cardKeyboardForDay(selected, today string, userOpenID string) *qqapi.Keyboard {
	return &qqapi.Keyboard{Content: qqapi.KeyboardContent{Rows: []qqapi.KeyboardRow{{
		Buttons: []qqapi.KeyboardButton{
			callbackButton("prev", "前一天", "day:"+shiftDay(selected, -1), userOpenID),
			callbackButton("today", "今天", "day:"+today, userOpenID),
			callbackButton("next", "后一天", "day:"+shiftDay(selected, 1), userOpenID),
		},
	}}}}
}

// cardKeyboardForRank builds 本周 / 上周 / 本月 callback buttons.
func cardKeyboardForRank(userOpenID string) *qqapi.Keyboard {
	return &qqapi.Keyboard{Content: qqapi.KeyboardContent{Rows: []qqapi.KeyboardRow{{
		Buttons: []qqapi.KeyboardButton{
			callbackButton("rank-thisweek", "本周", "rank:thisweek", userOpenID),
			callbackButton("rank-lastweek", "上周", "rank:lastweek", userOpenID),
			callbackButton("rank-thismonth", "本月", "rank:thismonth", userOpenID),
		},
	}}}}
}

func shiftDay(day string, delta int) string {
	parsed, err := time.ParseInLocation("2006-01-02", day, schedule.LocalTZ)
	if err != nil {
		return day
	}
	return parsed.AddDate(0, 0, delta).Format("2006-01-02")
}

func callbackButton(id, label, data, userOpenID string) qqapi.KeyboardButton {
	permission := &qqapi.ButtonPermission{Type: qqapi.ButtonPermissionSomeUser}
	if userOpenID != "" {
		permission.SpecifyUserIDs = []string{userOpenID}
	}
	return qqapi.KeyboardButton{
		ID:         id,
		RenderData: qqapi.ButtonRenderData{Label: label, VisitedLabel: label, Style: qqapi.ButtonStyleBlue},
		Action: qqapi.ButtonAction{
			Type:          qqapi.ButtonActionCallback,
			Data:          data,
			Permission:    permission,
			UnsupportTips: "当前版本不支持按钮，请直接发送指令",
		},
	}
}
