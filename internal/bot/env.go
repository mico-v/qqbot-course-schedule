package bot

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// Env carries everything the command handlers need.
type Env struct {
	Client        *qqapi.Client
	PanelStore    schedule.PanelStore
	Service       *schedule.Service
	Renderer      *render.Renderer
	DataDir       string
	ImagesDir     string
	FilesDir      string
	PublicBaseURL string
	// WakeUp fetches and imports WakeUp share codes. It is nil when the
	// feature is not configured, in which case those commands explain so.
	WakeUp WakeUpFetcher
	// Buttons enables markdown+keyboard card messages (platform invite only).
	Buttons bool
	// Now is overridable in tests.
	Now func() time.Time

	botAvatar     atomic.Pointer[image.RGBA]
	lastAvatarTry atomic.Int64
	avatarTrying  atomic.Bool

	lastStatsPrune atomic.Int64
	statsPruning   atomic.Bool

	avatarMu    sync.Mutex
	avatarCache map[string]avatarCacheEntry
}

// RenderedCard is a card image ready to send, including the dimensions the
// platform must use when embedding it in a Markdown message.
type RenderedCard struct {
	URL    string
	Width  int
	Height int
}

const (
	botAvatarFetchTimeout  = 15 * time.Second
	botAvatarRetryInterval = 5 * time.Minute
)

// RefreshBotAvatar fetches the bot's own avatar once and caches it in memory
// (and on disk). Failures are logged and never block card rendering.
func (e *Env) RefreshBotAvatar(ctx context.Context) {
	if e == nil || e.Client == nil {
		return
	}
	info, err := e.Client.GetBotInfo(ctx)
	if err != nil {
		slog.Warn("获取机器人信息失败", "err", err)
		return
	}
	if avatar := render.FetchBotAvatar(ctx, info.Avatar, e.DataDir); avatar != nil {
		e.SetBotAvatar(avatar)
		slog.Info("机器人头像已缓存", "name", info.Username)
	}
}

// rowUserIDs flattens the user ids of several row groups.
func rowUserIDs(groups ...[]schedule.DayRow) []string {
	var ids []string
	for _, rows := range groups {
		for _, row := range rows {
			ids = append(ids, row.UserID)
		}
	}
	return ids
}

// EnsureBotAvatar schedules a background refresh when the avatar is missing,
// so a failed startup fetch self-heals. It never blocks and tries at most once
// per retry interval.
func (e *Env) EnsureBotAvatar() {
	if e == nil || e.Client == nil || e.BotAvatar() != nil {
		return
	}
	now := time.Now()
	if last := e.lastAvatarTry.Load(); last != 0 && now.Sub(time.Unix(0, last)) < botAvatarRetryInterval {
		return
	}
	if !e.avatarTrying.CompareAndSwap(false, true) {
		return
	}
	e.lastAvatarTry.Store(now.UnixNano())
	go func() {
		defer e.avatarTrying.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), botAvatarFetchTimeout)
		defer cancel()
		e.RefreshBotAvatar(ctx)
	}()
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

// newReplier builds a replier for one inbound message with the recorder
// installed, so every send path reports its timing without the replier needing
// a back-reference to Env.
func (e *Env) newReplier(ctx context.Context, in *Inbound) *Replier {
	r := NewReplier(in, e.Client)
	return r.WithRecorder(ctx, e.recordStats)
}

// Scope returns the schedule scope for a message.
func (e *Env) Scope(in *Inbound) string {
	if in == nil {
		return ""
	}
	if in.Origin == OriginPrivate {
		return schedule.ScopePrivate(in.UserOpenID)
	}
	return schedule.ScopeGroup(in.GroupOpenID)
}

// GlobalUserID resolves the union OpenID for the sender. Direct webhook data
// wins, with the stored identity map serving callbacks and older messages.
func (e *Env) GlobalUserID(ctx context.Context, in *Inbound) (string, error) {
	if in == nil {
		return "", fmt.Errorf("无法识别发送人，请稍后重试。")
	}
	return e.globalUserIDForMember(ctx, e.Scope(in), in.GroupOpenID, in.UserOpenID, in.GlobalUserOpenID)
}

func (e *Env) globalUserIDForMember(ctx context.Context, scopeID, groupOpenID, localUserID, knownGlobalUserID string) (string, error) {
	localUserID = strings.TrimSpace(localUserID)
	if localUserID == "" {
		return "", fmt.Errorf("无法识别要操作的成员，请稍后重试。")
	}
	if globalID := strings.TrimSpace(knownGlobalUserID); globalID != "" {
		if err := e.Service.RecordIdentity(scopeID, localUserID, globalID); err != nil {
			return "", fmt.Errorf("保存全局用户身份失败：%w", err)
		}
		return globalID, nil
	}
	globalID, found, err := e.Service.ResolveIdentity(scopeID, localUserID)
	if err != nil {
		return "", fmt.Errorf("读取全局用户身份失败：%w", err)
	}
	if found {
		return globalID, nil
	}
	if groupOpenID == "" || e.Client == nil {
		return "", fmt.Errorf("无法识别该成员的全局用户身份：消息未返回 union_openid，且没有可补查的群成员身份。")
	}
	member, err := e.Client.GetGroupMember(ctx, groupOpenID, localUserID)
	if err != nil {
		if qqapi.ErrorCode(err) == 11253 {
			return "", fmt.Errorf("无法获取 union_openid：机器人尚未获得群成员信息接口权限，请向 QQ 开放平台申请该能力。")
		}
		return "", fmt.Errorf("获取群成员全局身份失败：%w", err)
	}
	globalID = strings.TrimSpace(member.UnionOpenID)
	if globalID == "" {
		return "", fmt.Errorf("平台未返回 union_openid，无法使用全局课表；请确认已申请并配置该能力。")
	}
	if err := e.Service.RecordIdentity(scopeID, localUserID, globalID); err != nil {
		return "", fmt.Errorf("保存全局用户身份失败：%w", err)
	}
	return globalID, nil
}

// ScheduleTarget resolves the write coordinates for one member. In global mode
// a schedule already stored in the current group keeps that target; otherwise
// the local member OpenID is projected onto union_openid.
func (e *Env) ScheduleTarget(ctx context.Context, in *Inbound, localUserID string) (schedule.StorageTarget, error) {
	localUserID = strings.TrimSpace(localUserID)
	globalUserID := ""
	groupOpenID := ""
	if in != nil {
		groupOpenID = in.GroupOpenID
		if localUserID == in.UserOpenID {
			globalUserID = strings.TrimSpace(in.GlobalUserOpenID)
		}
	}
	scopeID := e.Scope(in)
	settings, err := e.Service.BotSettings()
	if err != nil {
		return schedule.StorageTarget{}, fmt.Errorf("读取课表存储设置失败：%w", err)
	}
	if settings.StorageMode != schedule.StorageModeGlobal {
		return schedule.StorageTarget{ScopeID: scopeID, UserID: localUserID, LocalUserID: localUserID}, nil
	}
	if localUserID == "" {
		return schedule.StorageTarget{}, fmt.Errorf("无法识别要操作的成员，请稍后重试。")
	}

	target, targetErr := e.Service.ResolveStorageTarget(scopeID, localUserID, globalUserID)
	if targetErr == nil {
		return target, nil
	}
	if globalUserID != "" {
		return schedule.StorageTarget{}, targetErr
	}

	globalUserID, err = e.globalUserIDForMember(ctx, scopeID, groupOpenID, localUserID, "")
	if err != nil {
		return schedule.StorageTarget{}, err
	}
	return e.Service.ResolveStorageTarget(scopeID, localUserID, globalUserID)
}

// ScheduleMembers returns the member view for the current conversation. In
// global mode old rows in the current group take priority, then schedules from
// other groups are projected back onto local IDs so buttons keep working.
func (e *Env) ScheduleMembers(ctx context.Context, in *Inbound) (map[string]*schedule.Member, error) {
	scopeID := e.Scope(in)
	if e.currentSettings().StorageMode != schedule.StorageModeGlobal {
		return e.Service.ScopeMembers(scopeID)
	}
	identities, err := e.Service.ScopeIdentities(scopeID)
	if err != nil {
		return nil, err
	}
	if in != nil {
		localUserID := strings.TrimSpace(in.UserOpenID)
		if localUserID != "" {
			if globalUserOpenID := strings.TrimSpace(in.GlobalUserOpenID); globalUserOpenID != "" {
				identities[localUserID] = globalUserOpenID
			} else if _, exists := identities[localUserID]; !exists {
				// An empty value still asks the global resolver to look up this
				// local OpenID across legacy group schedules.
				identities[localUserID] = ""
			}
		}
	}
	return e.Service.SharedUserMembers(scopeID, identities)
}

// OverrideTargets resolves local member IDs to the coordinates used for
// personal holiday/shift markers. Whole-conversation markers always stay on
// the current group or private-chat scope.
func (e *Env) OverrideTargets(ctx context.Context, in *Inbound, localUserIDs []string) ([]schedule.DayOverrideTarget, error) {
	scopeID := e.Scope(in)
	targets := make([]schedule.DayOverrideTarget, 0, len(localUserIDs))
	for _, localUserID := range localUserIDs {
		localUserID = strings.TrimSpace(localUserID)
		if localUserID == "" {
			continue
		}
		if localUserID == schedule.DayOverrideAll {
			targets = append(targets, schedule.DayOverrideTarget{
				ScopeID:   scopeID,
				UserID:    schedule.DayOverrideAll,
				DisplayID: schedule.DayOverrideAll,
			})
			continue
		}
		target, err := e.ScheduleTarget(ctx, in, localUserID)
		if err != nil {
			return nil, err
		}
		targets = append(targets, schedule.DayOverrideTarget{
			ScopeID:   target.ScopeID,
			UserID:    target.UserID,
			DisplayID: target.LocalUserID,
		})
	}
	return targets, nil
}

// PublicImageURL builds the public URL of a generated card.
func (e *Env) PublicImageURL(name string) string {
	return e.PublicBaseURLForLinks() + "/images/" + name
}

// PublicBaseURLForLinks returns the service origin used in public links.
// The WebUI setting wins over the config.json fallback.
func (e *Env) PublicBaseURLForLinks() string {
	if e == nil {
		return ""
	}
	if e.Service != nil {
		if settings, err := e.Service.BotSettings(); err == nil && settings.BaseURL != "" {
			return settings.BaseURL
		}
	}
	return strings.TrimRight(strings.TrimSpace(e.PublicBaseURL), "/")
}

// RenderDayCard builds and saves the card for one day, recording how long the
// render stage took on the replier. at, when non-zero, is the reference instant
// parsed from the query; zero keeps the real current time.
// ok is false when the scope has no saved schedules.
func (e *Env) RenderDayCard(ctx context.Context, in *Inbound, r *Replier, day, at time.Time) (RenderedCard, bool, error) {
	start := time.Now()
	e.EnsureBotAvatar()
	members, err := e.ScheduleMembers(ctx, in)
	if err != nil {
		return RenderedCard{}, false, err
	}
	card, ok, err := e.Service.BuildDayCardForMembers(members, day, e.now(), at)
	if err != nil || !ok {
		return RenderedCard{}, ok, err
	}
	image := e.Renderer.DayCard(render.DayCardData{
		Title:         card.Title,
		Subtitle:      card.Subtitle,
		FoldedTitle:   card.FoldedTitle,
		Footer:        renderFooterText(card.Footer),
		Rows:          card.Rows,
		Folded:        card.Folded,
		BotAvatar:     e.BotAvatar(),
		BotName:       e.currentSettings().Nickname,
		Avatars:       e.memberAvatarsForMembers(ctx, members, rowUserIDs(card.Rows, card.Folded)),
		DurationLabel: "本节持续",
		FooterSince:   renderStart(in, start),
	})
	name, err := render.SaveJPEG(image, e.ImagesDir, "schedule_"+card.Selected.Format("20060102"))
	if err != nil {
		return RenderedCard{}, false, fmt.Errorf("保存课表图片失败: %w", err)
	}
	if r != nil {
		r.markRender(start)
	}
	bounds := image.Bounds()
	return RenderedCard{
		URL:    e.PublicImageURL(name),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
	}, true, nil
}

// SendCard sends a card image. When buttons are enabled it uses a markdown
// message (the only message type that renders keyboards) and falls back to a
// media message when the platform rejects the keyboard.
func (e *Env) SendCard(ctx context.Context, in *Inbound, r *Replier, card RenderedCard, keyboard *qqapi.Keyboard) error {
	return e.SendCardWithText(ctx, in, r, card, keyboard, "")
}

// SendCardWithText sends an optional markdown caption above the card image.
// A caption forces the markdown path even without buttons, because the media
// message type cannot carry text. When buttons are enabled the image also
// carries the inline keyboard; both fall back to a plain media message when
// the platform rejects markdown.
func (e *Env) SendCardWithText(ctx context.Context, in *Inbound, r *Replier, card RenderedCard, keyboard *qqapi.Keyboard, caption string) error {
	start := time.Now()
	caption = strings.TrimSpace(caption)
	useKeyboard := e.Buttons && keyboard != nil
	if useKeyboard || caption != "" {
		content := fmt.Sprintf("![课程表 #%dpx #%dpx](%s)", card.Width, card.Height, card.URL)
		if caption != "" {
			content = caption + "\n" + content
		}
		if !useKeyboard {
			keyboard = nil
		}
		err := e.sendCardMarkdown(ctx, in, r, content, keyboard)
		if err == nil {
			r.markSend(start, r.renderDone)
			e.recordStats(ctx, in, r, schedule.StatsStageCard, nil)
			return nil
		}
		slog.Warn("markdown 卡片发送失败，回退为媒体消息", "err", err)
	}
	// ReplyImage measures and reports the send stage itself. Do not re-measure
	// here: a second markSend would rewrite send_ms from a later origin, and the
	// reply-stage record it already filed would outrank the card record.
	err := r.ReplyImage(ctx, card.URL)
	e.recordStats(ctx, in, r, schedule.StatsStageCard, err)
	return err
}

// sendCardMarkdown sends one markdown message to the inbound's scope,
// replying to the originating message or interaction event.
func (e *Env) sendCardMarkdown(ctx context.Context, in *Inbound, r *Replier, content string, keyboard *qqapi.Keyboard) error {
	if in.MsgID == "" && in.EventID != "" {
		if in.Origin == OriginGroup {
			return e.Client.SendGroupMarkdownEvent(ctx, in.GroupOpenID, content, keyboard, in.EventID)
		}
		return e.Client.SendC2CMarkdownEvent(ctx, in.UserOpenID, content, keyboard, in.EventID)
	}
	seq, err := r.NextSeq()
	if err != nil {
		return err
	}
	if in.Origin == OriginGroup {
		return e.Client.SendGroupMarkdown(ctx, in.GroupOpenID, content, keyboard, in.MsgID, seq)
	}
	return e.Client.SendC2CMarkdown(ctx, in.UserOpenID, content, keyboard, in.MsgID, seq)
}

// RenderDayMarkdown builds the day schedule as a markdown list. ok is false
// when the scope has no saved schedules at all.
func (e *Env) RenderDayMarkdown(ctx context.Context, in *Inbound, day time.Time) (string, bool, error) {
	members, err := e.ScheduleMembers(ctx, in)
	if err != nil {
		return "", false, err
	}
	text, ok, err := e.Service.BuildDayMarkdownForMembers(members, day, e.now())
	if err != nil || !ok {
		return "", ok, err
	}
	return text, true, nil
}

// SendDayMarkdown sends the markdown list and falls back to plain text when
// the platform rejects markdown messages.
func (e *Env) SendDayMarkdown(ctx context.Context, in *Inbound, r *Replier, markdown string) error {
	return e.SendDayMarkdownWithKeyboard(ctx, in, r, markdown, nil)
}

// SendDayMarkdownWithKeyboard sends a markdown day list with an optional
// inline keyboard and falls back to plain text when the platform rejects it.
func (e *Env) SendDayMarkdownWithKeyboard(ctx context.Context, in *Inbound, r *Replier, markdown string, keyboard *qqapi.Keyboard) error {
	if !e.Buttons {
		keyboard = nil
	}
	err := r.ReplyMarkdownWithKeyboard(ctx, markdown, keyboard)
	if err == nil {
		return nil
	}
	if keyboard != nil {
		slog.Warn("带按钮的 markdown 课表发送失败，重试纯 markdown", "err", err)
		if retryErr := r.ReplyMarkdown(ctx, markdown); retryErr == nil {
			return nil
		} else {
			err = retryErr
		}
	}
	slog.Warn("markdown 课表发送失败，回退为文本", "err", err)
	return r.Reply(ctx, strings.ReplaceAll(markdown, "**", ""))
}

// RenderRankCard builds and saves the class-hours leaderboard for one period,
// recording how long the render stage took on the replier.
// ok is false when the scope has no countable courses.
func (e *Env) RenderRankCard(ctx context.Context, in *Inbound, r *Replier, period string) (RenderedCard, bool, error) {
	start := time.Now()
	e.EnsureBotAvatar()
	members, err := e.ScheduleMembers(ctx, in)
	if err != nil {
		return RenderedCard{}, false, err
	}
	rows, label, err := e.Service.RankBoardRowsForMembers(members, period, e.now())
	if err != nil {
		return RenderedCard{}, false, err
	}
	if len(rows) == 0 {
		return RenderedCard{}, false, nil
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
	footer = renderFooterText(footer)
	image := e.Renderer.DayCard(render.DayCardData{
		Title:    "群友上课时长榜",
		Subtitle: fmt.Sprintf("%s · 共 %d 位成员 · 合计 %s", label, len(rows), schedule.FormatDurationMinutes(total)),
		Footer:   footer,
		Rows:     dayRows,
		Avatars:  e.memberAvatarsForMembers(ctx, members, rowUserIDs(dayRows)),
		Legend: []render.LegendItem{
			{Key: "none", Label: "同一时段冲突的课程只计一次"},
			{Key: "none", Label: "全天日程不计入时长"},
		},
		BotAvatar:   e.BotAvatar(),
		BotName:     e.currentSettings().Nickname,
		FooterSince: renderStart(in, start),
	})
	name, err := render.SaveJPEG(image, e.ImagesDir, "rank_"+label)
	if err != nil {
		return RenderedCard{}, false, fmt.Errorf("保存榜单图片失败: %w", err)
	}
	if r != nil {
		r.markRender(start)
	}
	bounds := image.Bounds()
	return RenderedCard{
		URL:    e.PublicImageURL(name),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
	}, true, nil
}

// ImagesDirectory returns the absolute images directory (for tests/tools).
func (e *Env) ImagesDirectory() string {
	if e.ImagesDir != "" {
		return e.ImagesDir
	}
	return filepath.Join(e.DataDir, "images")
}

// renderFooterText appends the render-timing placeholder to a card footer. The
// renderer substitutes the measured value while drawing, because the duration
// is not known until the card has been built.
func renderFooterText(footer string) string {
	suffix := render.FooterTimingPlaceholder
	if strings.TrimSpace(footer) == "" {
		return suffix
	}
	return footer + " · " + suffix
}

// renderStart is the origin the card's render timing is measured from: the
// moment the message arrived, falling back to the current call's start.
func renderStart(in *Inbound, fallback time.Time) time.Time {
	if in != nil {
		if at := in.ReceivedAt(); !at.IsZero() {
			return at
		}
	}
	return fallback
}

// statsPruneInterval is how often expired handling records are swept during
// message dispatch. The throttle keeps the sweep cheap.
const statsPruneInterval = time.Hour

// pruneStats drops handling records past their retention, at most once per
// statsPruneInterval. Failures are logged and never disturb dispatch.
func (e *Env) pruneStats(now time.Time) {
	if e == nil || e.Service == nil {
		return
	}
	last := e.lastStatsPrune.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < statsPruneInterval {
		return
	}
	if !e.statsPruning.CompareAndSwap(false, true) {
		return
	}
	defer e.statsPruning.Store(false)
	e.lastStatsPrune.Store(now.UnixNano())
	removed, err := e.Service.PruneMessageStats(now)
	if err != nil {
		slog.Warn("清理耗时统计失败", "err", err)
		return
	}
	if removed > 0 {
		slog.Info("已清理过期的处理耗时记录", "removed", removed)
	}
}
