package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// The methods in this file exist only for v6 migration and read-only
// compatibility. New application code must use named repository ports.

// ListKV returns every key/value in one namespace.
func (s *Store) ListKV(scope, namespace string) ([]schedule.KVEntry, error) {
	rows, err := s.db.Query(
		`SELECT key, value FROM kv_data WHERE scope = ? AND namespace = ? ORDER BY key`,
		scope, namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("读取 KV 列表失败: %w", err)
	}
	defer rows.Close()
	var entries []schedule.KVEntry
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("读取 KV 行失败: %w", err)
		}
		entries = append(entries, schedule.KVEntry{Key: key, Value: value})
	}
	return entries, rows.Err()
}

// GetKV reads a namespaced value.
func (s *Store) GetKV(scope, namespace, key string, out any) (bool, error) {
	var raw []byte
	err := s.db.QueryRow(
		`SELECT value FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?`,
		scope, namespace, key,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取 KV 失败: %w", err)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return false, fmt.Errorf("解析 KV 失败: %w", err)
		}
	}
	return true, nil
}

// SetKV writes a namespaced value.
func (s *Store) SetKV(scope, namespace, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("编码 KV 失败: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(
		`INSERT INTO kv_data(scope, namespace, key, value, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(scope, namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP`,
		scope, namespace, key, raw,
	)
	if err != nil {
		return fmt.Errorf("写入 KV 失败: %w", err)
	}
	return nil
}

// DeleteKV removes a namespaced value.
func (s *Store) DeleteKV(scope, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(
		`DELETE FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?`,
		scope, namespace, key,
	); err != nil {
		return fmt.Errorf("删除 KV 失败: %w", err)
	}
	return nil
}
