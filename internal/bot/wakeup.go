package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/wakeup"
)

// WakeUpFetcher fetches the raw shareData text for a WakeUp share code. It is
// the network port the bot depends on; the concrete client lives in
// internal/wakeup and is injected in cmd/bot.
type WakeUpFetcher interface {
	FetchShareData(ctx context.Context, code string) (string, error)
}

// wakeUpImportTimeout bounds the two-request fetch so a slow WakeUp server
// cannot stall the handler.
const wakeUpImportTimeout = 25 * time.Second

// ImportWakeUp resolves a share code or share message and stores it as the
// sender's schedule, replacing any previous one.
func (e *Env) ImportWakeUp(ctx context.Context, in *Inbound, r *Replier, text string) error {
	if e.WakeUp == nil {
		return r.Reply(ctx, "WakeUp 导入未配置。")
	}
	code := wakeup.ExtractShareCode(text)
	if code == "" {
		return r.Reply(ctx, "没有识别到 WakeUp 分享口令，请把完整的分享文案发给我。")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, wakeUpImportTimeout)
	defer cancel()
	shareData, err := e.WakeUp.FetchShareData(fetchCtx, code)
	if err != nil {
		slog.Warn("获取 WakeUp 课表失败", "err", err)
		return r.Reply(ctx, "获取 WakeUp 课表失败："+err.Error())
	}
	if strings.TrimSpace(shareData) == "" {
		return r.Reply(ctx, "分享口令无效或已过期，请让分享者重新分享。")
	}
	result, err := e.Service.SaveWakeUpShare(e.Scope(in), in.UserOpenID, in.Username, shareData, "wakeup", in.UserOpenID)
	if err != nil {
		slog.Warn("导入 WakeUp 课表失败", "err", err)
		return r.Reply(ctx, err.Error())
	}
	action := "已更新"
	if result.Created {
		action = "已创建"
	}
	return r.Reply(ctx, fmt.Sprintf("%s %s 的课表：%d 个课程事件。", action, result.Name, result.EventCount))
}

// isWakeUpShareText reports whether a message is the official WakeUp share
// message, which is what triggers an automatic import. Requiring both markers
// keeps ordinary chat that merely mentions "分享口令" from hitting the network.
func isWakeUpShareText(content string) bool {
	return strings.Contains(content, "WakeUp") && strings.Contains(content, "分享口令")
}
