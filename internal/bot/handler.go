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
	Description string
	Handle      func(ctx context.Context, msg *Message) error
}

// Handler routes inbound messages to registered commands.
type Handler struct {
	mu       sync.RWMutex
	commands map[string]*Command
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
}

// Commands returns the registered commands sorted by prefix.
func (h *Handler) Commands() []*Command {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]*Command, 0, len(h.commands))
	for _, cmd := range h.commands {
		result = append(result, cmd)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Prefix < result[j].Prefix })
	return result
}

// mentionPrefix strips leading "<@...>" mention markers some payloads keep.
var mentionPrefix = regexp.MustCompile(`^(?:<@!?[0-9A-Za-z_=-]+>\s*)+`)

// Dispatch matches the message against registered commands. Unknown messages
// are ignored without a reply, matching the platform's group-chat etiquette.
func (h *Handler) Dispatch(ctx context.Context, msg *Message) {
	content := strings.TrimSpace(mentionPrefix.ReplaceAllString(msg.Content, ""))
	msg.Content = content
	if content == "" {
		return
	}
	fields := strings.Fields(content)
	prefix := fields[0]

	h.mu.RLock()
	cmd, ok := h.commands[prefix]
	h.mu.RUnlock()
	if !ok {
		slog.Debug("未命中指令", "prefix", prefix, "origin", msg.Origin)
		return
	}
	slog.Info("执行指令", "prefix", prefix, "origin", msg.Origin, "user", shortID(msg.UserOpenID))
	if err := cmd.Handle(ctx, msg); err != nil {
		slog.Error("指令执行失败", "prefix", prefix, "err", err)
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}
