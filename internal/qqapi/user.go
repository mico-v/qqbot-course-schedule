package qqapi

import (
	"context"
	"net/http"
)

// BotInfo is the response of GET /users/@me.
type BotInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Bot      bool   `json:"bot"`
}

// GetBotInfo fetches the bot's own profile, including its avatar URL.
func (c *Client) GetBotInfo(ctx context.Context) (*BotInfo, error) {
	var info BotInfo
	if err := c.request(ctx, http.MethodGet, "/users/@me", nil, &info, true); err != nil {
		return nil, err
	}
	return &info, nil
}
