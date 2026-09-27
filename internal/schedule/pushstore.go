package schedule

// PushSubscription is one scope that receives the daily card.
type PushSubscription struct {
	Enabled     bool   `json:"enabled"`
	Origin      string `json:"origin"` // group / private
	OpenID      string `json:"openid"`
	EnabledBy   string `json:"enabled_by,omitempty"`
	EnabledAt   string `json:"enabled_at,omitempty"`
	Paused      bool   `json:"paused,omitempty"`
	PauseReason string `json:"pause_reason,omitempty"`
	// Cron overrides the global push_cron for this scope (5-field cron).
	Cron string `json:"cron,omitempty"`
	// LastRun records the minute of the last automatic push (dedupe guard).
	LastRun string `json:"last_run,omitempty"`
}

// PushStore persists per-scope daily push subscriptions.
type PushStore interface {
	GetPushSubscription(scope string) (PushSubscription, bool, error)
	SetPushSubscription(scope string, sub PushSubscription) error
	DeletePushSubscription(scope string) error
	ListPushSubscriptions() (map[string]PushSubscription, error)
}
