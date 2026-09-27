package store

import (
	"encoding/json"
	"log/slog"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

var (
	_ schedule.PushStore  = (*Store)(nil)
	_ schedule.PanelStore = (*Store)(nil)
)

// GetPushSubscription reads one daily push subscription.
func (s *Store) GetPushSubscription(scope string) (schedule.PushSubscription, bool, error) {
	var sub schedule.PushSubscription
	found, err := s.GetKV(schedule.KVScopeGlobal, schedule.KVNamespacePush, scope, &sub)
	return sub, found, err
}

// SetPushSubscription stores one daily push subscription.
func (s *Store) SetPushSubscription(scope string, sub schedule.PushSubscription) error {
	return s.SetKV(schedule.KVScopeGlobal, schedule.KVNamespacePush, scope, sub)
}

// DeletePushSubscription removes one daily push subscription.
func (s *Store) DeletePushSubscription(scope string) error {
	return s.DeleteKV(schedule.KVScopeGlobal, schedule.KVNamespacePush, scope)
}

// ListPushSubscriptions returns every stored daily push subscription.
func (s *Store) ListPushSubscriptions() (map[string]schedule.PushSubscription, error) {
	entries, err := s.ListKV(schedule.KVScopeGlobal, schedule.KVNamespacePush)
	if err != nil {
		return nil, err
	}
	result := make(map[string]schedule.PushSubscription, len(entries))
	for _, entry := range entries {
		var sub schedule.PushSubscription
		if err := json.Unmarshal(entry.Value, &sub); err != nil {
			slog.Warn("解析推送订阅失败", "scope", entry.Key, "err", err)
			continue
		}
		result[entry.Key] = sub
	}
	return result, nil
}

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
