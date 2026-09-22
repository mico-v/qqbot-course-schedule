package schedule

import (
	"fmt"
	"sort"
	"strings"
)

// Backup/import limits.
const (
	MaxBackupBytes   = 20 << 20
	MaxBackupMembers = 500
)

// BackupFile is the "原始备份" export format.
type BackupFile struct {
	Version    int              `json:"version"`
	ScopeID    string           `json:"scope_id"`
	ExportedAt string           `json:"exported_at"`
	Members    []BackupMember   `json:"members"`
	Overrides  []DayOverrideRow `json:"overrides"`
}

// BackupMember is one member inside a backup file.
type BackupMember struct {
	UserID string  `json:"user_id"`
	Name   string  `json:"name"`
	Source string  `json:"source"`
	Events []Event `json:"events"`
	ICS    string  `json:"ics,omitempty"`
}

// ImportResult summarises a bulk import.
type ImportResult struct {
	MemberCount   int      `json:"member_count"`
	CreatedCount  int      `json:"created_count"`
	UpdatedCount  int      `json:"updated_count"`
	EventCount    int      `json:"event_count"`
	OverrideCount int      `json:"day_override_count"`
	Skipped       []string `json:"skipped,omitempty"`
}

// ExportBackup snapshots a scope (members + day overrides).
func (s *Service) ExportBackup(scopeID string) (*BackupFile, error) {
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, err
	}
	overrides, err := s.store.ListDayOverrides(scopeID)
	if err != nil {
		return nil, err
	}
	backup := &BackupFile{
		Version:    1,
		ScopeID:    scopeID,
		ExportedAt: NowISO(),
		Overrides:  overrides,
	}
	ids := make([]string, 0, len(members))
	for userID := range members {
		ids = append(ids, userID)
	}
	sort.Strings(ids)
	for _, userID := range ids {
		member := members[userID]
		backup.Members = append(backup.Members, BackupMember{
			UserID: userID,
			Name:   member.Name,
			Source: member.Source,
			Events: member.Events,
			ICS:    member.ICS,
		})
	}
	return backup, nil
}

// ImportBackup restores a backup into a scope. Existing day overrides are
// replaced by the backup's set, matching "完整还原" semantics.
func (s *Service) ImportBackup(scopeID string, backup *BackupFile, actor string) (*ImportResult, error) {
	if backup == nil {
		return nil, fmt.Errorf("备份内容为空。")
	}
	if backup.Version != 1 {
		return nil, fmt.Errorf("不支持的备份版本 %d。", backup.Version)
	}
	if len(backup.Members) > MaxBackupMembers {
		return nil, fmt.Errorf("备份包含 %d 位成员，超过上限 %d。", len(backup.Members), MaxBackupMembers)
	}
	result := &ImportResult{}
	for _, entry := range backup.Members {
		userID := strings.TrimSpace(entry.UserID)
		if userID == "" {
			continue
		}
		_, found, err := s.store.GetMember(scopeID, userID)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = userID
		}
		member := &Member{
			UserID:            userID,
			Name:              name,
			Events:            entry.Events,
			ICS:               entry.ICS,
			Source:            firstNonEmpty(entry.Source, "ics"),
			EventCount:        len(entry.Events),
			UpdatedAt:         NowISO(),
			ScheduleUpdatedAt: NowISO(),
			LastModifiedAt:    NowISO(),
			LastModifiedBy:    actor,
		}
		member.Schedule = FormatICSSchedule(member.Events)
		if err := s.store.PutMember(scopeID, userID, member, nil); err != nil {
			return nil, err
		}
		result.MemberCount++
		if found {
			result.UpdatedCount++
		} else {
			result.CreatedCount++
		}
		result.EventCount += len(member.Events)
	}

	if err := s.store.DeleteScopeDayOverrides(scopeID); err != nil {
		return nil, err
	}
	for _, override := range backup.Overrides {
		if override.Day == "" || override.Kind == "" {
			continue
		}
		createdBy := firstNonEmpty(override.CreatedBy, actor)
		createdAt := firstNonEmpty(override.CreatedAt, NowISO())
		if err := s.store.SetDayOverride(scopeID, override.UserID, override.Day,
			DayOverride{Kind: override.Kind, SourceDay: override.SourceDay}, createdBy, createdAt); err != nil {
			return nil, err
		}
		result.OverrideCount++
	}
	return result, nil
}
