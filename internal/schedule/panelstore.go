package schedule

// PanelState is the persisted platform panel identity and content fingerprint.
type PanelState struct {
	PanelID   string `json:"panel_id"`
	ItemsHash string `json:"items_hash"`
	Version   int    `json:"version"`
}

// PanelStore persists command panel state by platform scope.
type PanelStore interface {
	GetPanelState(scope string) (PanelState, bool, error)
	SetPanelState(scope string, state PanelState) error
}
