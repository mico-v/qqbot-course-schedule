package bot

import (
	"context"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// Command is one registered chat command.
type Command struct {
	Prefix      string
	Aliases     []string
	Description string
	Handle      func(ctx context.Context, in *Inbound, r *Replier) error
	// Ready marks a command that is implemented and safe to advertise in the
	// platform command panel.
	Ready bool
}

// Handler routes inbound messages to registered commands.
type Handler struct {
	mu       sync.RWMutex
	commands map[string]*Command
	env      *Env
}

// NewHandler returns an empty command router.
func NewHandler() *Handler {
	return &Handler{commands: make(map[string]*Command)}
}

// Register adds a command. Registering the same prefix twice replaces it.
func (h *Handler) Register(cmd *Command) {
	if cmd == nil || cmd.Prefix == "" {
		return
	}
	if cmd.Handle == nil {
		slog.Warn("忽略没有处理函数的指令", "prefix", cmd.Prefix)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.commands[cmd.Prefix]; exists {
		slog.Warn("指令前缀重复，旧指令被覆盖", "prefix", cmd.Prefix)
	}
	h.commands[cmd.Prefix] = cmd
	for _, alias := range cmd.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == cmd.Prefix {
			continue
		}
		if _, exists := h.commands[alias]; exists {
			slog.Warn("指令别名重复，旧指令被覆盖", "alias", alias)
		}
		h.commands[alias] = cmd
	}
}

// Commands returns the registered commands sorted by prefix.
func (h *Handler) Commands() []*Command {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[*Command]bool, len(h.commands))
	result := make([]*Command, 0, len(h.commands))
	for _, cmd := range h.commands {
		if seen[cmd] {
			continue
		}
		seen[cmd] = true
		result = append(result, cmd)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Prefix < result[j].Prefix })
	return result
}

// Command looks up one command by prefix.
func (h *Handler) Command(prefix string) (*Command, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cmd, ok := h.commands[prefix]
	return cmd, ok
}

// mentionPrefix strips leading "<@...>" mention markers some payloads keep.
var mentionPrefix = regexp.MustCompile(`^(?:<@!?[0-9A-Za-z_=-]+>\s*)+`)

// Dispatch matches the message against registered commands. Unknown messages
// are ignored without a reply, matching the platform's group-chat etiquette.
func (h *Handler) Dispatch(ctx context.Context, in *Inbound, r *Replier) {
	// dispatchErr is what the record below reports; only the command stage can
	// set it, so the paths that never reach a command record a success.
	var dispatchErr error
	if h.env != nil {
		// Expired handling records are swept here (throttled to once an hour)
		// so retention no longer depends on a background ticker.
		h.env.pruneStats(h.env.now())
		// Install the recorder here rather than at every construction site, so a
		// caller that builds a bare replier still gets its stages recorded.
		if r != nil {
			r.WithRecorder(ctx, h.env.recordStats)
		}
		// Every exit is recorded, including the ones that send nothing (unknown
		// command, bot off, reply policy off). A later send stage supersedes it,
		// so a card does not also log its fallback reply.
		defer func() {
			h.env.recordStats(ctx, in, r, schedule.StatsStageHandler, dispatchErr)
		}()
	}

	settings := h.currentSettings()

	// Trim first so a mention preceded by whitespace is still stripped.
	raw := strings.TrimSpace(in.Content)
	mentioned := mentionPrefix.MatchString(raw)
	content := strings.TrimSpace(mentionPrefix.ReplaceAllString(raw, ""))
	in.Content = content

	// Member discovery and .ics auto-import only run while the bot is enabled.
	if settings.Enabled && h.env != nil {
		// Every accepted message marks the sender as seen, so the admin page can
		// offer an empty schedule even for members who only chat (full-message mode).
		h.recordSeen(in)
		for _, attachment := range in.Attachments {
			if isICSFile(attachment) {
				if err := h.env.ImportICS(ctx, in, r, attachment); err != nil {
					slog.Error("导入课表失败", "err", err)
				}
				return
			}
		}
		// A pasted WakeUp share message imports without an explicit command.
		if h.env.WakeUp != nil && isWakeUpShareText(content) {
			if err := h.env.ImportWakeUp(ctx, in, r, content); err != nil {
				slog.Error("导入 WakeUp 课表失败", "err", err)
			}
			return
		}
	}

	if content == "" {
		return
	}
	fields := strings.Fields(content)
	prefix := fields[0]

	h.mu.RLock()
	cmd, ok := h.commands[prefix]
	// The platform command panel may drop the leading slash, so "课表" and
	// "/课表" must both hit the same command.
	if !ok && !strings.HasPrefix(prefix, "/") {
		cmd, ok = h.commands["/"+prefix]
	}
	h.mu.RUnlock()
	if !ok {
		if settings.Enabled {
			slog.Debug("未命中指令", "prefix", prefix, "origin", in.Origin)
		}
		return
	}

	// An administrator may always manage the switches, even while the bot is off,
	// so it can be turned back on from chat.
	managingSettings := managesBotSettings(cmd, in)
	if !settings.Enabled && !managingSettings {
		slog.Debug("机器人已关闭，忽略指令", "prefix", prefix, "origin", in.Origin)
		return
	}
	mode := classifyReplyMode(mentioned, strings.HasPrefix(prefix, "/"))
	if !managingSettings && !allowsReply(settings, mode) {
		slog.Debug("回复策略已关闭，忽略指令", "prefix", prefix, "mode", string(mode), "origin", in.Origin)
		return
	}

	in.Args = strings.TrimSpace(strings.TrimPrefix(content, prefix))
	in.Command = cmd.Prefix
	slog.Info("执行指令", "prefix", prefix, "origin", in.Origin, "user", shortID(in.UserOpenID))
	err := cmd.Handle(ctx, in, r)
	if err != nil {
		slog.Error("指令执行失败", "prefix", prefix, "err", err)
	}
	dispatchErr = err
}

// managesBotSettings reports whether a command may run while the bot is off.
// Administrators keep access to the settings command so they can turn the bot
// back on from chat.
func managesBotSettings(cmd *Command, in *Inbound) bool {
	if cmd == nil || !canManageSettings(in) {
		return false
	}
	return cmd.Prefix == settingsCommandPrefix
}

// Callback is one button press delivered as an INTERACTION_CREATE event.
type Callback struct {
	Data        string
	EventID     string
	GroupOpenID string
	UserOpenID  string
}

// HandleCallback renders the card a button asked for and replies passively to
// the interaction event.
func (h *Handler) HandleCallback(ctx context.Context, cb *Callback) {
	if h.env == nil || cb == nil || cb.Data == "" {
		return
	}
	in := (&Inbound{EventID: cb.EventID, UserOpenID: cb.UserOpenID, GroupOpenID: cb.GroupOpenID}).WithReceived(ctx)
	if cb.GroupOpenID != "" {
		in.Origin = OriginGroup
	} else {
		in.Origin = OriginPrivate
	}
	r := h.env.newReplier(ctx, in)
	action, param, ok := strings.Cut(cb.Data, ":")
	if !ok {
		return
	}
	switch action {
	case "day":
		day, err := time.ParseInLocation("2006-01-02", param, schedule.LocalTZ)
		if err != nil {
			return
		}
		if h.currentSettings().SendFormat == schedule.SendFormatMarkdown {
			text, found, err := h.env.RenderDayMarkdown(in, day)
			if err != nil || !found {
				_ = r.Reply(ctx, "当前会话还没有可展示的课程表。")
				return
			}
			if err := h.env.SendDayMarkdown(ctx, in, r, text); err != nil {
				slog.Error("按钮课表发送失败", "err", err)
			}
			return
		}
		card, found, err := h.env.RenderDayCard(ctx, in, r, day)
		if err != nil || !found {
			_ = r.Reply(ctx, "当前会话还没有可展示的课程表。")
			return
		}
		keyboard := cardKeyboardForDay(day.Format("2006-01-02"), h.env.now().Format("2006-01-02"), cb.UserOpenID)
		if err := h.env.SendCard(ctx, in, r, card, keyboard); err != nil {
			slog.Error("按钮卡片发送失败", "err", err)
		}
	case "dayrich":
		day, err := time.ParseInLocation("2006-01-02", param, schedule.LocalTZ)
		if err != nil {
			return
		}
		text, found, err := h.env.RenderDayMarkdown(in, day)
		if err != nil || !found {
			_ = r.Reply(ctx, "当前会话还没有可展示的课程表。")
			return
		}
		text = scheduleParseResult(day) + text
		keyboard := dayNavigationKeyboard(day.Format("2006-01-02"), cb.UserOpenID)
		if err := h.env.SendDayMarkdownWithKeyboard(ctx, in, r, text, keyboard); err != nil {
			slog.Error("按钮课表发送失败", "err", err)
		}
	case "rank":
		period := map[string]string{"thisweek": "本周", "lastweek": "上周", "thismonth": "本月"}[param]
		if period == "" {
			return
		}
		card, found, err := h.env.RenderRankCard(ctx, in, r, period)
		if err != nil || !found {
			_ = r.Reply(ctx, "当前会话还没有可统计的课程。")
			return
		}
		if err := h.env.SendCard(ctx, in, r, card, cardKeyboardForRank(cb.UserOpenID)); err != nil {
			slog.Error("按钮榜单发送失败", "err", err)
		}
	}
}

// recordSeen remembers the sender so the admin page can offer an empty schedule.
func (h *Handler) recordSeen(in *Inbound) {
	if h.env == nil || in.UserOpenID == "" {
		return
	}
	if err := h.env.Service.RecordSeenMember(h.env.Scope(in), in.UserOpenID, in.Username); err != nil {
		slog.Warn("记录成员失败", "err", err)
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}
