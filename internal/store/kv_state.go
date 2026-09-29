package store

import (
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

var _ schedule.PanelStore = (*Store)(nil)

// GetPanelState reads the persisted state of one platform command panel.
func (s *Store) GetPanelState(scope string) (schedule.PanelState, bool, error) {
	var state schedule.PanelState
	found, err := s.GetKV(schedule.KVScopeGlobal, schedule.KVNamespacePanel, scope, &state)
	return state, found, err
}

// SetPanelState stores the persisted state of one platform command panel.
func (s *Store) SetPanelState(scope string, state schedule.PanelState) error {
	return s.SetKV(schedule.KVScopeGlobal, schedule.KVNamespacePanel, scope, state)
}
