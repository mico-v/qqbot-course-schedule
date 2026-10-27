package schedule

import "context"

// PanelState is the persisted platform panel identity and content fingerprint.
type PanelState struct {
	PanelID   string `json:"panel_id"`
	ItemsHash string `json:"items_hash"`
	Version   int    `json:"version"`
}

// PanelStore persists command panel state by platform scope.
type PanelStore interface {
	GetPanelState(ctx context.Context, scope string) (PanelState, bool, error)
	SetPanelState(ctx context.Context, scope string, state PanelState) error
	DeletePanelState(ctx context.Context, scope string) error
}
