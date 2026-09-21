package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// PushSubscription is one scope that receives the daily card.
type PushSubscription struct {
	Enabled   bool   `json:"enabled"`
	Origin    string `json:"origin"` // group / private
	OpenID    string `json:"openid"`
	EnabledBy string `json:"enabled_by,omitempty"`
	EnabledAt string `json:"enabled_at,omitempty"`
}

const pushNamespace = "push"

// SetPushSubscription stores or replaces one scope's subscription.
func (e *Env) SetPushSubscription(scope string, sub PushSubscription) error {
	return e.Store.SetKV("global", pushNamespace, scope, sub)
}

// RemovePushSubscription disables the daily card for one scope.
func (e *Env) RemovePushSubscription(scope string) error {
	return e.Store.DeleteKV("global", pushNamespace, scope)
}

// PushSubscriptions lists every subscribed scope.
func (e *Env) PushSubscriptions() (map[string]PushSubscription, error) {
	entries, err := e.Store.ListKV("global", pushNamespace)
	if err != nil {
		return nil, err
	}
	result := make(map[string]PushSubscription, len(entries))
	for _, entry := range entries {
		var sub PushSubscription
		if err := json.Unmarshal(entry.Value, &sub); err != nil {
			slog.Warn("解析推送订阅失败", "scope", entry.Key, "err", err)
			continue
		}
		result[entry.Key] = sub
	}
	return result, nil
}

// PushDaily sends the day card to every subscribed scope.
func (e *Env) PushDaily(ctx context.Context) (sent, skipped, failed int) {
	subscriptions, err := e.PushSubscriptions()
	if err != nil {
		slog.Error("读取推送订阅失败", "err", err)
		return 0, 0, 0
	}
	for scope, sub := range subscriptions {
		if !sub.Enabled || sub.OpenID == "" {
			continue
		}
		switch err := e.PushScope(ctx, scope, sub); {
		case err == nil:
			sent++
		case errors.Is(err, errPushNoSchedule):
			skipped++
		default:
			failed++
			slog.Warn("主动推送失败", "scope", scope, "err", err)
		}
	}
	return sent, skipped, failed
}

// errPushNoSchedule means the scope has no schedules to render.
var errPushNoSchedule = errors.New("没有可推送的课程表")

// PushScope sends the day card to one scope proactively.
func (e *Env) PushScope(ctx context.Context, scope string, sub PushSubscription) error {
	msg := &Message{Client: e.Client}
	if sub.Origin == "group" {
		msg.Origin = OriginGroup
		msg.GroupOpenID = sub.OpenID
	} else {
		msg.Origin = OriginPrivate
		msg.UserOpenID = sub.OpenID
	}
	url, ok, err := e.RenderDayCard(ctx, msg, e.now())
	if err != nil {
		return err
	}
	if !ok {
		return errPushNoSchedule
	}
	return msg.PushImage(ctx, url)
}

// pushTimeText renders the cron time for replies ("每天 07:30").
func pushTimeText(spec string) string {
	fields := strings.Fields(spec)
	if len(fields) >= 2 {
		minute, minuteErr := atoiField(fields[0])
		hour, hourErr := atoiField(fields[1])
		if minuteErr == nil && hourErr == nil {
			return fmt.Sprintf("每天 %02d:%02d", hour, minute)
		}
	}
	return "定时（" + spec + "）"
}

func atoiField(value string) (int, error) {
	result := 0
	if value == "" {
		return 0, fmt.Errorf("empty")
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return 0, fmt.Errorf("not a number")
		}
		result = result*10 + int(value[index]-'0')
	}
	return result, nil
}

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
