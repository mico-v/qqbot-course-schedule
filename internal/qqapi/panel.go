package qqapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// PanelItem is one command/link entry of a command panel.
type PanelItem struct {
	Name      string `json:"name,omitempty"`
	Desc      string `json:"desc,omitempty"`
	Type      string `json:"type,omitempty"`
	OnlyAdmin bool   `json:"only_admin,omitempty"`
	Link      string `json:"link,omitempty"`
}

// Panel is the panel payload (items + developer-only remark).
type Panel struct {
	Items   []PanelItem `json:"items,omitempty"`
	Remark  string      `json:"remark,omitempty"`
	Version int         `json:"version,omitempty"`
}

// PanelRecord is one panel returned by the list API.
type PanelRecord struct {
	PanelID    string `json:"panel_id"`
	Scope      string `json:"scope"`
	TargetType string `json:"target_type"`
	Panel      Panel  `json:"panel"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Version    int    `json:"version"`
}

// CreatePanel creates a command panel and returns its panel_id.
// scope is c2c/group/channel/dm; targetType is all/specific.
func (c *Client) CreatePanel(ctx context.Context, scope, targetType string, groupOpenIDs, userOpenIDs []string, panel Panel) (string, error) {
	body := map[string]any{
		"scope":       scope,
		"target_type": targetType,
		"panel":       panel,
	}
	if len(groupOpenIDs) > 0 {
		body["group_openids"] = groupOpenIDs
	}
	if len(userOpenIDs) > 0 {
		body["user_openids"] = userOpenIDs
	}
	var result struct {
		PanelID string `json:"panel_id"`
	}
	if err := c.request(ctx, http.MethodPost, "/v2/panels", body, &result, true); err != nil {
		return "", err
	}
	if result.PanelID == "" {
		return "", fmt.Errorf("创建指令面板失败: 响应缺少 panel_id")
	}
	return result.PanelID, nil
}

// UpdatePanel replaces a panel's items and remark (targets are unaffected).
func (c *Client) UpdatePanel(ctx context.Context, panelID string, panel Panel) error {
	path := fmt.Sprintf("/v2/panels/%s", url.PathEscape(panelID))
	return c.request(ctx, http.MethodPut, path, map[string]any{"panel": panel}, nil, true)
}

// DeletePanel removes a panel.
func (c *Client) DeletePanel(ctx context.Context, panelID string) error {
	path := fmt.Sprintf("/v2/panels/%s", url.PathEscape(panelID))
	return c.request(ctx, http.MethodDelete, path, nil, nil, true)
}

// ListPanels lists panels of one scope, one page at a time.
func (c *Client) ListPanels(ctx context.Context, scope, cursor string, limit int) ([]PanelRecord, string, bool, error) {
	path := "/v2/panels?scope=" + url.QueryEscape(scope)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}
	var result struct {
		Records    []PanelRecord `json:"records"`
		NextCursor string        `json:"next_cursor"`
		IsEnd      bool          `json:"is_end"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &result, true); err != nil {
		return nil, "", false, err
	}
	return result.Records, result.NextCursor, result.IsEnd, nil
}

// UpdatePanelTargets adds or removes specific users/groups from a panel.
func (c *Client) UpdatePanelTargets(ctx context.Context, panelID, op string, groupOpenIDs, userOpenIDs []string) error {
	path := fmt.Sprintf("/v2/panels/%s/target", url.PathEscape(panelID))
	body := map[string]any{"op": op}
	if len(groupOpenIDs) > 0 {
		body["group_openids"] = groupOpenIDs
	}
	if len(userOpenIDs) > 0 {
		body["user_openids"] = userOpenIDs
	}
	return c.request(ctx, http.MethodPut, path, body, nil, true)
}
