package bot

import (
	"context"
	"fmt"
	"image"
	"math"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// Env carries everything the command handlers need.
type Env struct {
	Client        *qqapi.Client
	Store         schedule.Storage
	Service       *schedule.Service
	Renderer      *render.Renderer
	DataDir       string
	ImagesDir     string
	PublicBaseURL string
	// Now is overridable in tests.
	Now func() time.Time

	botAvatar atomic.Pointer[image.RGBA]
}

// SetBotAvatar stores the fetched bot avatar for card headers.
func (e *Env) SetBotAvatar(avatar *image.RGBA) {
	if e != nil {
		e.botAvatar.Store(avatar)
	}
}

// BotAvatar returns the cached bot avatar, or nil when unavailable.
func (e *Env) BotAvatar() image.Image {
	if e == nil {
		return nil
	}
	if avatar := e.botAvatar.Load(); avatar != nil {
		return avatar
	}
	return nil
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().In(schedule.LocalTZ)
}

// Scope returns the schedule scope for a message.
func (e *Env) Scope(msg *Message) string {
	if msg.Origin == OriginPrivate {
		return schedule.ScopePrivate(msg.UserOpenID)
	}
	return schedule.ScopeGroup(msg.GroupOpenID)
}

// PublicImageURL builds the public URL of a generated card.
func (e *Env) PublicImageURL(name string) string {
	return strings.TrimRight(e.PublicBaseURL, "/") + "/images/" + name
}

// RenderDayCard builds and saves the card for one day.
// ok is false when the scope has no saved schedules.
func (e *Env) RenderDayCard(ctx context.Context, msg *Message, day time.Time) (string, bool, error) {
	_ = ctx
	card, ok, err := e.Service.BuildDayCard(e.Scope(msg), day, e.now())
	if err != nil || !ok {
		return "", ok, err
	}
	image := e.Renderer.DayCard(render.DayCardData{
		Title:         card.Title,
		Subtitle:      card.Subtitle,
		FoldedTitle:   card.FoldedTitle,
		Footer:        card.Footer,
		Rows:          card.Rows,
		Folded:        card.Folded,
		BotAvatar:     e.BotAvatar(),
		DurationLabel: "本节持续",
	})
	name, err := render.SaveJPEG(image, e.ImagesDir, "schedule_"+card.Selected.Format("20060102"))
	if err != nil {
		return "", false, fmt.Errorf("保存课表图片失败: %w", err)
	}
	return e.PublicImageURL(name), true, nil
}

// RenderRankCard builds and saves the class-hours leaderboard for one period.
// ok is false when the scope has no countable courses.
func (e *Env) RenderRankCard(ctx context.Context, msg *Message, period string) (string, bool, error) {
	_ = ctx
	rows, label, err := e.Service.RankBoardRows(e.Scope(msg), period, e.now())
	if err != nil {
		return "", false, err
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	shown := rows
	if len(shown) > schedule.DefaultRankTopN {
		shown = shown[:schedule.DefaultRankTopN]
	}
	dayRows := make([]schedule.DayRow, 0, len(shown))
	for _, row := range shown {
		statusKey := "rank"
		if row.Rank > 0 && row.Rank <= 3 {
			statusKey = fmt.Sprintf("rank%d", row.Rank)
		}
		status := "无课"
		duration := "统计区间内没有课程"
		countdown := "—"
		if row.Rank > 0 {
			status = fmt.Sprintf("#%d", row.Rank)
			duration = fmt.Sprintf("已上 %s / 共 %s", row.ElapsedText, row.HoursText)
			countdown = fmt.Sprintf("%d%%", int(math.Round(row.Progress*100)))
		}
		dayRows = append(dayRows, schedule.DayRow{
			UserID:         row.UserID,
			Name:           row.Name,
			StatusKey:      statusKey,
			Status:         status,
			Course:         row.HoursText,
			TimeText:       fmt.Sprintf("%d 节 · %d 门课", row.CourseCount, row.CourseNames),
			Duration:       duration,
			Progress:       row.Progress,
			CountdownLabel: "时长占比",
			Countdown:      countdown,
			CourseCount:    row.CourseCount,
		})
	}
	total := 0
	for _, row := range rows {
		total += row.Minutes
	}
	footer := "重复课程按 RRULE 展开 · 时间以本地时区为准"
	if len(rows) > len(shown) {
		footer += fmt.Sprintf(" · 仅展示前 %d 名", len(shown))
	}
	image := e.Renderer.DayCard(render.DayCardData{
		Title:    "群友上课时长榜",
		Subtitle: fmt.Sprintf("%s · 共 %d 位成员 · 合计 %s", label, len(rows), schedule.FormatDurationMinutes(total)),
		Footer:   footer,
		Rows:     dayRows,
		Legend: []render.LegendItem{
			{Key: "none", Label: "同一时段冲突的课程只计一次"},
			{Key: "none", Label: "全天日程不计入时长"},
		},
		BotAvatar: e.BotAvatar(),
	})
	name, err := render.SaveJPEG(image, e.ImagesDir, "rank_"+label)
	if err != nil {
		return "", false, fmt.Errorf("保存榜单图片失败: %w", err)
	}
	return e.PublicImageURL(name), true, nil
}

// ImagesDirectory returns the absolute images directory (for tests/tools).
func (e *Env) ImagesDirectory() string {
	if e.ImagesDir != "" {
		return e.ImagesDir
	}
	return filepath.Join(e.DataDir, "images")
}
