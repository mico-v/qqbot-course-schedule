package webhook

import (
	"log/slog"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
)

// messageTypeText maps the platform message_type to a readable label.
func messageTypeText(messageType int) string {
	switch messageType {
	case 0:
		return "text"
	case 3:
		return "ark"
	case 101:
		return "parallel"
	case 102:
		return "forward"
	case 103:
		return "quote"
	default:
		return ""
	}
}

// previewText collapses whitespace and truncates long message content.
func previewText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}

// logInboundMessage records every accepted chat message at info level so
// operators can verify full-message reception with journalctl.
func logInboundMessage(event string, messageType int, message *bot.Message) {
	attributes := []any{
		"event", event,
		"origin", string(message.Origin),
	}
	if message.Origin == bot.OriginGroup {
		attributes = append(attributes, "group", shortID(message.GroupOpenID))
	}
	attributes = append(attributes,
		"user", shortID(message.UserOpenID),
		"name", message.Username,
	)
	if message.MemberRole != "" {
		attributes = append(attributes, "role", message.MemberRole)
	}
	if label := messageTypeText(messageType); label != "" {
		attributes = append(attributes, "type", label)
	}
	if content := previewText(message.Content, 120); content != "" {
		attributes = append(attributes, "content", content)
	}
	if len(message.Attachments) > 0 {
		names := make([]string, 0, len(message.Attachments))
		for _, attachment := range message.Attachments {
			name := attachment.Filename
			if name == "" {
				name = attachment.ContentType
			}
			names = append(names, name)
		}
		attributes = append(attributes, "attachments", strings.Join(names, ", "))
	}
	slog.Info("收到消息", attributes...)
}
