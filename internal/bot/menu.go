package bot

import (
	"context"
	"log/slog"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

// menuItems is the c2c custom menu (global, replaced wholesale).
var menuItems = []qqapi.MenuItem{
	{Name: "今日课表", Type: "send_message", SendMessage: "/今日课表"},
	{Name: "明日课表", Type: "send_message", SendMessage: "/明日课表"},
	{Name: "上课时长榜", Type: "send_message", SendMessage: "/上课时长榜"},
	{Name: "导入课表", Type: "send_message", SendMessage: "/导入课表"},
	{Name: "帮助", Type: "send_message", SendMessage: "/help"},
}

// SyncMenu writes the c2c menu when it differs from the desired items.
func SyncMenu(ctx context.Context, env *Env) (bool, error) {
	if env == nil || env.Client == nil {
		return false, nil
	}
	current, err := env.Client.GetMenu(ctx)
	if err == nil && menuEqual(current, menuItems) {
		return false, nil
	}
	if err != nil {
		slog.Warn("读取自定义菜单失败，尝试覆盖", "err", err)
	}
	if err := env.Client.SetMenu(ctx, qqapi.Menu{Items: menuItems}); err != nil {
		return false, err
	}
	return true, nil
}

func menuEqual(current *qqapi.Menu, want []qqapi.MenuItem) bool {
	if current == nil || len(current.Items) != len(want) {
		return false
	}
	for index := range want {
		got := current.Items[index]
		if got.Name != want[index].Name || got.Type != want[index].Type || got.SendMessage != want[index].SendMessage {
			return false
		}
	}
	return true
}
