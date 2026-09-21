package qqapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// textMessage is the request body for msg_type=0.
type textMessage struct {
	Content  string    `json:"content"`
	MsgType  int       `json:"msg_type"`
	Keyboard *Keyboard `json:"keyboard,omitempty"`
	MsgID    string    `json:"msg_id,omitempty"`
	EventID  string    `json:"event_id,omitempty"`
	MsgSeq   int       `json:"msg_seq,omitempty"`
}

// markdownMessage is the request body for msg_type=2.
type markdownMessage struct {
	MsgType  int             `json:"msg_type"`
	Markdown markdownContent `json:"markdown"`
	Keyboard *Keyboard       `json:"keyboard,omitempty"`
	MsgID    string          `json:"msg_id,omitempty"`
	EventID  string          `json:"event_id,omitempty"`
	MsgSeq   int             `json:"msg_seq,omitempty"`
}

type markdownContent struct {
	Content string `json:"content"`
}

// SendGroupText sends a passive or proactive text message to a group.
// msgID and msgSeq may be empty/zero for a proactive message.
func (c *Client) SendGroupText(ctx context.Context, groupOpenID, content, msgID string, msgSeq int) (*SendResult, error) {
	path := fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID))
	return c.sendText(ctx, path, content, msgID, "", msgSeq)
}

// SendC2CText sends a passive or proactive text message to a single chat.
func (c *Client) SendC2CText(ctx context.Context, userOpenID, content, msgID string, msgSeq int) (*SendResult, error) {
	path := fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID))
	return c.sendText(ctx, path, content, msgID, "", msgSeq)
}

// SendGroupTextEvent replies to an interaction/group event passively.
func (c *Client) SendGroupTextEvent(ctx context.Context, groupOpenID, content, eventID string) error {
	path := fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID))
	_, err := c.sendText(ctx, path, content, "", eventID, 1)
	return err
}

// SendC2CTextEvent replies to an interaction event passively.
func (c *Client) SendC2CTextEvent(ctx context.Context, userOpenID, content, eventID string) error {
	path := fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID))
	_, err := c.sendText(ctx, path, content, "", eventID, 1)
	return err
}

func (c *Client) sendText(ctx context.Context, path, content, msgID, eventID string, msgSeq int) (*SendResult, error) {
	if msgID == "" && eventID == "" {
		msgSeq = 0
	}
	body := textMessage{Content: content, MsgType: 0, MsgID: msgID, EventID: eventID, MsgSeq: msgSeq}
	var result SendResult
	if err := c.request(ctx, http.MethodPost, path, body, &result, true); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendGroupMarkdown sends a markdown message (optionally with a keyboard).
func (c *Client) SendGroupMarkdown(ctx context.Context, groupOpenID, content string, keyboard *Keyboard, msgID string, msgSeq int) error {
	path := fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID))
	return c.sendMarkdown(ctx, path, content, keyboard, msgID, "", msgSeq)
}

// SendC2CMarkdown sends a markdown message (optionally with a keyboard).
func (c *Client) SendC2CMarkdown(ctx context.Context, userOpenID, content string, keyboard *Keyboard, msgID string, msgSeq int) error {
	path := fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID))
	return c.sendMarkdown(ctx, path, content, keyboard, msgID, "", msgSeq)
}

// SendGroupMarkdownEvent replies to an event with markdown (buttons work on markdown).
func (c *Client) SendGroupMarkdownEvent(ctx context.Context, groupOpenID, content string, keyboard *Keyboard, eventID string) error {
	path := fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID))
	return c.sendMarkdown(ctx, path, content, keyboard, "", eventID, 1)
}

// SendC2CMarkdownEvent replies to an event with markdown.
func (c *Client) SendC2CMarkdownEvent(ctx context.Context, userOpenID, content string, keyboard *Keyboard, eventID string) error {
	path := fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID))
	return c.sendMarkdown(ctx, path, content, keyboard, "", eventID, 1)
}

func (c *Client) sendMarkdown(ctx context.Context, path, content string, keyboard *Keyboard, msgID, eventID string, msgSeq int) error {
	if msgID == "" && eventID == "" {
		msgSeq = 0
	}
	body := markdownMessage{
		MsgType:  2,
		Markdown: markdownContent{Content: content},
		Keyboard: keyboard,
		MsgID:    msgID,
		EventID:  eventID,
		MsgSeq:   msgSeq,
	}
	return c.request(ctx, http.MethodPost, path, body, nil, true)
}

// AckInteraction answers a button/menu interaction. QQ requires exactly one
// PUT per interaction id, otherwise the client keeps loading until timeout.
func (c *Client) AckInteraction(ctx context.Context, interactionID string) error {
	path := fmt.Sprintf("/interactions/%s", url.PathEscape(interactionID))
	body := map[string]int{"code": 0}
	return c.request(ctx, http.MethodPut, path, body, nil, true)
}
