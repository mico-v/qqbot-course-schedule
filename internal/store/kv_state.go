package store

import (
	"context"
	"database/sql"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

var _ schedule.PanelStore = (*Store)(nil)

// GetPanelState reads the persisted state of one platform command panel.
func (s *Store) GetPanelState(ctx context.Context, scope string) (schedule.PanelState, bool, error) {
	var state schedule.PanelState
	err := s.db.QueryRowContext(
		ctx,
		`SELECT panel_id, items_hash, version FROM bot_panels WHERE panel_scope = ?`,
		scope,
	).Scan(&state.PanelID, &state.ItemsHash, &state.Version)
	if err == sql.ErrNoRows {
		return schedule.PanelState{}, false, nil
	}
	if err != nil {
		return schedule.PanelState{}, false, err
	}
	return state, true, nil
}

// SetPanelState stores the persisted state of one platform command panel.
func (s *Store) SetPanelState(ctx context.Context, scope string, state schedule.PanelState) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO bot_panels(panel_scope, panel_id, items_hash, version, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(panel_scope) DO UPDATE SET
		 panel_id = excluded.panel_id,
		 items_hash = excluded.items_hash,
		 version = excluded.version,
		 updated_at = CURRENT_TIMESTAMP`,
		scope, state.PanelID, state.ItemsHash, max(state.Version, 1),
	)
	return err
}

// DeletePanelState removes one persisted panel record.
func (s *Store) DeletePanelState(ctx context.Context, scope string) error {
	_, err := s.db.ExecContext(
		ctx,
		`DELETE FROM bot_panels WHERE panel_scope = ?`,
		scope,
	)
	return err
}
