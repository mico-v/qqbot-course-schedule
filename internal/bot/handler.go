package bot

import (
	"context"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Command is one registered chat command.
type Command struct {
	Prefix      string
	Aliases     []string
	Description string
	Handle      func(ctx context.Context, msg *Message) error
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
func (h *Handler) Dispatch(ctx context.Context, msg *Message) {
	content := strings.TrimSpace(mentionPrefix.ReplaceAllString(msg.Content, ""))
	msg.Content = content

	// An .ics attachment imports itself, whether or not it came with a command.
	if h.env != nil {
		for _, attachment := range msg.Attachments {
			if isICSFile(attachment) {
				h.recordSeen(msg)
				if err := h.env.ImportICS(ctx, msg, attachment); err != nil {
					slog.Error("导入课表失败", "err", err)
				}
				return
			}
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
		slog.Debug("未命中指令", "prefix", prefix, "origin", msg.Origin)
		return
	}
	msg.Args = strings.TrimSpace(strings.TrimPrefix(content, prefix))
	h.recordSeen(msg)
	slog.Info("执行指令", "prefix", prefix, "origin", msg.Origin, "user", shortID(msg.UserOpenID))
	if err := cmd.Handle(ctx, msg); err != nil {
		slog.Error("指令执行失败", "prefix", prefix, "err", err)
	}
}

// recordSeen remembers the sender so the admin page can offer an empty schedule.
func (h *Handler) recordSeen(msg *Message) {
	if h.env == nil || msg.UserOpenID == "" {
		return
	}
	if err := h.env.Service.RecordSeenMember(h.env.Scope(msg), msg.UserOpenID, msg.Username); err != nil {
		slog.Warn("记录成员失败", "err", err)
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}
