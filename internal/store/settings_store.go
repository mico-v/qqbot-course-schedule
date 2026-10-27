package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// LoadBotSettings reads the singleton settings aggregate.
func (s *Store) LoadBotSettings(ctx context.Context) (schedule.Settings, bool, error) {
	var raw string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT settings_json FROM bot_settings WHERE id = 1`,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return schedule.Settings{}, false, nil
	}
	if err != nil {
		return schedule.Settings{}, false, fmt.Errorf("读取机器人设置失败: %w", err)
	}
	settings := schedule.DefaultSettings()
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return schedule.Settings{}, false, fmt.Errorf("解析机器人设置失败: %w", err)
	}
	return settings, true, nil
}

// SaveBotSettings writes the singleton settings aggregate.
func (s *Store) SaveBotSettings(ctx context.Context, settings schedule.Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("编码机器人设置失败: %w", err)
	}
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO bot_settings(id, settings_json, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		 settings_json = excluded.settings_json,
		 updated_at = excluded.updated_at`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("保存机器人设置失败: %w", err)
	}
	return nil
}
