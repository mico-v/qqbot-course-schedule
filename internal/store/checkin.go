package store

import (
	"database/sql"
	"fmt"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// GetCheckinRecord reads one member's check-in on one day.
func (s *Store) GetCheckinRecord(scopeID, userID, day string) (*schedule.CheckinRecord, bool, error) {
	var record schedule.CheckinRecord
	err := s.db.QueryRow(
		`SELECT scope_id, user_id, name, day, points, created_at
		 FROM checkin_records WHERE scope_id = ? AND user_id = ? AND day = ?`,
		scopeID, userID, day,
	).Scan(&record.ScopeID, &record.UserID, &record.Name, &record.Day, &record.Points, &record.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取签到记录失败: %w", err)
	}
	return &record, true, nil
}

// InsertCheckinRecord stores one record. It reports false when the member
// already checked in that day, so a repeated call never adds points twice.
func (s *Store) InsertCheckinRecord(record schedule.CheckinRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(
		`INSERT OR IGNORE INTO checkin_records(scope_id, user_id, name, day, points, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.ScopeID, record.UserID, record.Name, record.Day, record.Points, record.CreatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("写入签到记录失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取签到写入结果失败: %w", err)
	}
	return affected > 0, nil
}

// CountCheckinRecords counts one scope's stored records.
func (s *Store) CountCheckinRecords(scopeID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM checkin_records WHERE scope_id = ?`, scopeID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("统计签到记录失败: %w", err)
	}
	return count, nil
}

// ListCheckinRecords lists a scope's records, newest day first. An empty
// userID means every member; limit <= 0 means no limit.
func (s *Store) ListCheckinRecords(scopeID, userID string, limit int) ([]schedule.CheckinRecord, error) {
	query := `SELECT scope_id, user_id, name, day, points, created_at
		FROM checkin_records WHERE scope_id = ?`
	args := []any{scopeID}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY day DESC, user_id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("读取签到记录失败: %w", err)
	}
	defer rows.Close()
	var records []schedule.CheckinRecord
	for rows.Next() {
		var record schedule.CheckinRecord
		if err := rows.Scan(&record.ScopeID, &record.UserID, &record.Name, &record.Day, &record.Points, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取签到记录行失败: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// DeleteCheckinRecord removes one record, reporting whether it existed.
func (s *Store) DeleteCheckinRecord(scopeID, userID, day string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(
		`DELETE FROM checkin_records WHERE scope_id = ? AND user_id = ? AND day = ?`,
		scopeID, userID, day,
	)
	if err != nil {
		return false, fmt.Errorf("删除签到记录失败: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}
