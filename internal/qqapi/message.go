package qqapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// textMessage is the request body for msg_type=0.
type textMessage struct {
	Content string `json:"content"`
	MsgType int    `json:"msg_type"`
	MsgID   string `json:"msg_id,omitempty"`
	MsgSeq  int    `json:"msg_seq,omitempty"`
}

// SendGroupText sends a passive or proactive text message to a group.
// msgID and msgSeq may be empty/zero for a proactive message.
func (c *Client) SendGroupText(ctx context.Context, groupOpenID, content, msgID string, msgSeq int) (*SendResult, error) {
	path := fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID))
	return c.sendText(ctx, path, content, msgID, msgSeq)
}

// SendC2CText sends a passive or proactive text message to a single chat.
func (c *Client) SendC2CText(ctx context.Context, userOpenID, content, msgID string, msgSeq int) (*SendResult, error) {
	path := fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID))
	return c.sendText(ctx, path, content, msgID, msgSeq)
}

func (c *Client) sendText(ctx context.Context, path, content, msgID string, msgSeq int) (*SendResult, error) {
	if msgID == "" {
		msgSeq = 0
	}
	body := textMessage{Content: content, MsgType: 0, MsgID: msgID, MsgSeq: msgSeq}
	var result SendResult
	if err := c.request(ctx, http.MethodPost, path, body, &result, true); err != nil {
		return nil, err
	}
	return &result, nil
}

// AckInteraction answers a button/menu interaction. QQ requires exactly one
// PUT per interaction id, otherwise the client keeps loading until timeout.
func (c *Client) AckInteraction(ctx context.Context, interactionID string) error {
	path := fmt.Sprintf("/interactions/%s", url.PathEscape(interactionID))
	body := map[string]int{"code": 0}
	return c.request(ctx, http.MethodPut, path, body, nil, true)
}
