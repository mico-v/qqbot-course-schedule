package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

const (
	handlerTimeout    = 30 * time.Second
	dedupTTL          = 10 * time.Minute
	dedupMaxEntries   = 5000
	interactionAckTTL = 5 * time.Second
)

// Dispatcher turns verified webhook payloads into command invocations.
type Dispatcher struct {
	client  *qqapi.Client
	secret  string
	handler *bot.Handler

	mu   sync.Mutex
	seen map[string]time.Time
}

// NewDispatcher builds a dispatcher for one bot application.
func NewDispatcher(client *qqapi.Client, secret string, handler *bot.Handler) *Dispatcher {
	return &Dispatcher{
		client:  client,
		secret:  secret,
		handler: handler,
		seen:    make(map[string]time.Time),
	}
}

// Handle is the gin handler mounted at POST /webhook.
func (d *Dispatcher) Handle(c *gin.Context) {
	var payload Payload
	if err := c.ShouldBindJSON(&payload); err != nil {
		slog.Warn("webhook payload 解析失败", "err", err)
		c.Status(http.StatusOK)
		return
	}

	if payload.Op == OpValidation {
		d.handleValidation(c, payload)
		return
	}

	// Events are acknowledged immediately and processed asynchronously.
	c.Status(http.StatusOK)
	if payload.Op != 0 || payload.T == "" {
		return
	}
	if !d.markSeen(payload.ID) {
		slog.Debug("webhook 重复事件已忽略", "event", payload.T, "id", shortID(payload.ID))
		return
	}
	go d.process(payload)
}

func (d *Dispatcher) handleValidation(c *gin.Context, payload Payload) {
	var data ValidationData
	if err := json.Unmarshal(payload.D, &data); err != nil {
		slog.Warn("op=13 数据解析失败", "err", err)
		c.Status(http.StatusBadRequest)
		return
	}
	signature, err := SignValidation(d.secret, data.EventTS, data.PlainToken)
	if err != nil {
		slog.Error("生成 op=13 签名失败", "err", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	slog.Info("webhook 验证请求已应答")
	c.JSON(http.StatusOK, gin.H{"plain_token": data.PlainToken, "signature": signature})
}

func (d *Dispatcher) process(payload Payload) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("事件处理 panic", "event", payload.T, "panic", recovered)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	switch payload.T {
	case EventGroupAtMessage, EventGroupMessage:
		data, err := decodeMessage(payload.D)
		if err != nil {
			slog.Warn("群消息解析失败", "err", err)
			return
		}
		if data.Author.Bot {
			return
		}
		message := &bot.Message{
			Origin:      bot.OriginGroup,
			GroupOpenID: data.GroupOpenID,
			UserOpenID:  firstNonEmpty(data.Author.MemberOpenID, data.Author.UnionOpenID, data.Author.ID),
			MsgID:       data.ID,
			Content:     data.Content,
			Username:    data.Author.Username,
			MemberRole:  data.Author.MemberRole,
			Attachments: toBotAttachments(data.Attachments),
			Mentions:    toBotMentions(data.Mentions),
			Client:      d.client,
		}
		d.handler.Dispatch(ctx, message)

	case EventC2CMessage:
		data, err := decodeMessage(payload.D)
		if err != nil {
			slog.Warn("单聊消息解析失败", "err", err)
			return
		}
		if data.Author.Bot {
			return
		}
		d.handler.Dispatch(ctx, &bot.Message{
			Origin:      bot.OriginPrivate,
			UserOpenID:  firstNonEmpty(data.Author.UserOpenID, data.Author.UnionOpenID, data.Author.ID),
			MsgID:       data.ID,
			Content:     data.Content,
			Username:    data.Author.Username,
			MemberRole:  data.Author.MemberRole,
			Attachments: toBotAttachments(data.Attachments),
			Mentions:    toBotMentions(data.Mentions),
			Client:      d.client,
		})

	case EventInteraction:
		ackCtx, ackCancel := context.WithTimeout(context.Background(), interactionAckTTL)
		defer ackCancel()
		var data InteractionData
		_ = json.Unmarshal(payload.D, &data)
		if err := d.client.AckInteraction(ackCtx, payload.ID); err != nil {
			slog.Error("互动事件应答失败", "id", shortID(payload.ID), "err", err)
			return
		}
		slog.Info("互动事件已应答", "button", data.Data.Resolved.ButtonID, "data", data.Data.Resolved.ButtonData)

	case EventGroupMsgRecv, EventC2CMsgRecv:
		var data BroadcastData
		_ = json.Unmarshal(payload.D, &data)
		slog.Info("主动推送开关变更", "event", payload.T,
			"group", shortID(data.GroupOpenID), "openid", shortID(firstNonEmpty(data.OpenID, data.OPMemberOpenID)))

	default:
		slog.Debug("未处理的事件", "event", payload.T)
	}
}

func decodeMessage(raw json.RawMessage) (*MessageData, error) {
	var data MessageData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func toBotMentions(mentions []Author) []bot.Mention {
	var result []bot.Mention
	for _, mention := range mentions {
		if mention.Bot {
			continue
		}
		id := firstNonEmpty(mention.MemberOpenID, mention.UserOpenID, mention.UnionOpenID, mention.ID)
		if id == "" {
			continue
		}
		result = append(result, bot.Mention{ID: id, Name: mention.Username})
	}
	return result
}

func toBotAttachments(attachments []Attachment) []bot.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]bot.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		result = append(result, bot.Attachment{
			URL:         attachment.URL,
			Filename:    attachment.Filename,
			ContentType: attachment.ContentType,
			Size:        attachment.Size,
		})
	}
	return result
}

// markSeen reports whether id is new, keeping the dedup table bounded.
func (d *Dispatcher) markSeen(id string) bool {
	if id == "" {
		return true
	}
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.seen[id]; exists {
		return false
	}
	if len(d.seen) >= dedupMaxEntries {
		for key, at := range d.seen {
			if now.Sub(at) > dedupTTL {
				delete(d.seen, key)
			}
		}
	}
	d.seen[id] = now
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}
