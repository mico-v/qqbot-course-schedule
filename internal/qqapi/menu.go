package qqapi

import (
	"context"
	"fmt"
	"net/http"
)

// MenuItem is one custom menu entry (c2c only).
type MenuItem struct {
	Name         string     `json:"name,omitempty"`
	Type         string     `json:"type,omitempty"`
	SendMessage  string     `json:"send_message,omitempty"`
	Link         string     `json:"link,omitempty"`
	SubMenuItems []MenuItem `json:"sub_menu_items,omitempty"`
}

// Menu is the full custom menu; PUT replaces it entirely.
type Menu struct {
	Items []MenuItem `json:"items,omitempty"`
}

// GetMenu returns the current c2c menu, or nil when none is set.
func (c *Client) GetMenu(ctx context.Context) (*Menu, error) {
	var result struct {
		Version int  `json:"version"`
		Menu    Menu `json:"menu"`
	}
	if err := c.request(ctx, http.MethodGet, "/v2/menu", nil, &result, true); err != nil {
		return nil, err
	}
	if len(result.Menu.Items) == 0 {
		return nil, nil
	}
	return &result.Menu, nil
}

// SetMenu replaces the c2c custom menu.
func (c *Client) SetMenu(ctx context.Context, menu Menu) error {
	if len(menu.Items) > 10 {
		return fmt.Errorf("自定义菜单最多 10 项")
	}
	return c.request(ctx, http.MethodPut, "/v2/menu", map[string]any{"menu": menu}, nil, true)
}
