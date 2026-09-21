package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

const (
	panelRemark    = "qqbot-course-schedule"
	panelKVScope   = "global"
	panelNamespace = "panel"
	maxPanelItems  = 20
	// Panel item limits are counted in display columns: a CJK rune counts 2.
	panelNameLimit = 14
	panelDescLimit = 30
)

// panelOrder is the curated order of commands shown in the panel.
var panelOrder = []string{
	"/今日课表", "/明日课表", "/课表",
	"/上课时长榜",
	"/休假", "/调休", "/销假", "/假期",
	"/导入课表",
	"/help",
}

type panelState struct {
	PanelID   string `json:"panel_id"`
	ItemsHash string `json:"items_hash"`
	Version   int    `json:"version"`
}

// SyncPanels creates or updates the c2c and group command panels. It is safe to
// call on every startup: unchanged panels are skipped.
func SyncPanels(ctx context.Context, env *Env, handler *Handler) (created, updated int, err error) {
	if env == nil || env.Store == nil || env.Client == nil || handler == nil {
		return 0, 0, fmt.Errorf("同步指令面板缺少依赖")
	}
	items := panelItems(handler)
	if len(items) == 0 {
		return 0, 0, nil
	}
	itemsHash := hashPanelItems(items)

	for _, scope := range []string{"c2c", "group"} {
		state, found, err := loadPanelState(env, scope)
		if err != nil {
			return created, updated, err
		}
		if found && state.PanelID != "" && state.ItemsHash == itemsHash {
			continue
		}
		if found && state.PanelID != "" {
			if updateErr := env.Client.UpdatePanel(ctx, state.PanelID, qqapi.Panel{Items: items, Remark: panelRemark}); updateErr == nil {
				state.ItemsHash = itemsHash
				state.Version++
				if err := savePanelState(env, scope, state); err != nil {
					return created, updated, err
				}
				updated++
				slog.Info("已更新指令面板", "scope", scope, "panel_id", state.PanelID, "items", len(items))
				continue
			} else {
				// The panel may have been deleted on the platform; verify before recreating.
				existing, listErr := findOurPanel(ctx, env, scope)
				if listErr != nil {
					return created, updated, fmt.Errorf("更新 %s 面板失败: %w", scope, updateErr)
				}
				if existing != nil {
					return created, updated, fmt.Errorf("更新 %s 面板失败: %w", scope, updateErr)
				}
				slog.Warn("指令面板已不存在，重新创建", "scope", scope, "err", updateErr)
				state.PanelID = ""
			}
		}
		if state.PanelID == "" {
			if existing, listErr := findOurPanel(ctx, env, scope); listErr == nil && existing != nil {
				state = panelState{PanelID: existing.PanelID, ItemsHash: itemsHash, Version: existing.Version}
				if err := savePanelState(env, scope, state); err != nil {
					return created, updated, err
				}
				continue
			}
		}
		panelID, createErr := env.Client.CreatePanel(ctx, scope, "all", nil, nil, qqapi.Panel{Items: items, Remark: panelRemark})
		if createErr != nil {
			return created, updated, fmt.Errorf("创建 %s 面板失败: %w", scope, createErr)
		}
		if err := savePanelState(env, scope, panelState{PanelID: panelID, ItemsHash: itemsHash, Version: 1}); err != nil {
			return created, updated, err
		}
		created++
		slog.Info("已创建指令面板", "scope", scope, "panel_id", panelID, "items", len(items))
	}
	return created, updated, nil
}

// panelItems builds the platform items from ready commands in curated order.
func panelItems(handler *Handler) []qqapi.PanelItem {
	var items []qqapi.PanelItem
	for _, prefix := range panelOrder {
		command, ok := handler.Command(prefix)
		if !ok || !command.Ready || command.Description == "" {
			continue
		}
		items = append(items, qqapi.PanelItem{
			Type: "command",
			Name: truncateDisplay(prefix, panelNameLimit),
			Desc: truncateDisplay(command.Description, panelDescLimit),
		})
		if len(items) >= maxPanelItems {
			break
		}
	}
	return items
}

func findOurPanel(ctx context.Context, env *Env, scope string) (*qqapi.PanelRecord, error) {
	records, _, _, err := env.Client.ListPanels(ctx, scope, "", 50)
	if err != nil {
		return nil, err
	}
	for index := range records {
		record := &records[index]
		if record.Panel.Remark == panelRemark {
			return record, nil
		}
	}
	return nil, nil
}

func loadPanelState(env *Env, scope string) (panelState, bool, error) {
	var state panelState
	found, err := env.Store.GetKV(panelKVScope, panelNamespace, scope, &state)
	return state, found, err
}

func savePanelState(env *Env, scope string, state panelState) error {
	return env.Store.SetKV(panelKVScope, panelNamespace, scope, state)
}

func hashPanelItems(items []qqapi.PanelItem) string {
	encoded, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}

// truncateDisplay cuts text to max display columns (CJK rune = 2 columns).
func truncateDisplay(text string, maxColumns int) string {
	if displayColumns(text) <= maxColumns {
		return text
	}
	var builder strings.Builder
	width := 0
	for _, r := range text {
		runeWidth := 1
		if isWideRune(r) {
			runeWidth = 2
		}
		if width+runeWidth > maxColumns-1 {
			break
		}
		builder.WriteRune(r)
		width += runeWidth
	}
	return builder.String() + "…"
}

func displayColumns(text string) int {
	width := 0
	for _, r := range text {
		if isWideRune(r) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0xA4CF, // CJK radicals .. Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE4F, // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60, // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x20000 && r <= 0x3FFFD: // CJK extensions
		return true
	default:
		return false
	}
}
