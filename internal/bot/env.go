package bot

import (
	"context"
	"fmt"
	"image"
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
		Title:       card.Title,
		Subtitle:    card.Subtitle,
		FoldedTitle: card.FoldedTitle,
		Footer:      card.Footer,
		Rows:        card.Rows,
		Folded:      card.Folded,
		BotAvatar:   e.BotAvatar(),
	})
	name, err := render.SaveJPEG(image, e.ImagesDir, "schedule_"+card.Selected.Format("20060102"))
	if err != nil {
		return "", false, fmt.Errorf("保存课表图片失败: %w", err)
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
