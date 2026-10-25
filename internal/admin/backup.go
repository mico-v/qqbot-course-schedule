package admin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// Backup/import limits.
const (
	MaxBackupBytes   = 20 << 20
	MaxBackupMembers = 500
)

// BackupFile is the "原始备份" export format.
type BackupFile struct {
	Version    int                       `json:"version"`
	ScopeID    string                    `json:"scope_id"`
	ExportedAt string                    `json:"exported_at"`
	Members    []BackupMember            `json:"members"`
	Overrides  []schedule.DayOverrideRow `json:"overrides"`
}

// BackupMember is one member inside a backup file.
type BackupMember struct {
	UserID string           `json:"user_id"`
	Name   string           `json:"name"`
	QQ     string           `json:"qq,omitempty"`
	Source string           `json:"source"`
	Events []schedule.Event `json:"events"`
	ICS    string           `json:"ics,omitempty"`
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

// ExportBackup snapshots a scope (members + day overrides). In global storage
// mode, personal schedules and markers are read from global-user scopes while
// whole-conversation markers remain attached to the displayed scope.
func (s *Service) ExportBackup(scopeID string) (*BackupFile, error) {
	settings, err := s.BotSettings()
	if err != nil {
		return nil, err
	}

	var members map[string]*schedule.Member
	var overrides []schedule.DayOverrideRow
	if settings.StorageMode == schedule.StorageModeGlobal {
		members, err = s.VisibleMembers(scopeID)
		if err != nil {
			return nil, err
		}
		targets, err := s.dayOverrideTargets(scopeID, members)
		if err != nil {
			return nil, err
		}
		overrides, err = s.DayOverrideRowsForTargets(targets)
		if err != nil {
			return nil, err
		}
	} else {
		members, err = s.storage.GetScopeMembers(scopeID)
		if err != nil {
			return nil, err
		}
		overrides, err = s.storage.ListDayOverrides(scopeID)
		if err != nil {
			return nil, err
		}
	}

	backup := &BackupFile{
		Version:    1,
		ScopeID:    scopeID,
		ExportedAt: schedule.NowISO(),
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
			QQ:     member.QQ,
			Source: member.Source,
			Events: member.Events,
			ICS:    member.ICS,
		})
	}
	return backup, nil
}

// ImportBackup restores a backup into a scope. Existing day overrides for the
// schedules visible in that scope are replaced by the backup's set, matching
// "完整还原" semantics.
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
	settings, err := s.BotSettings()
	if err != nil {
		return nil, err
	}

	type resolvedMember struct {
		entry  BackupMember
		target schedule.StorageTarget
	}
	resolvedMembers := make([]resolvedMember, 0, len(backup.Members))
	for _, entry := range backup.Members {
		localUserID := strings.TrimSpace(entry.UserID)
		if localUserID == "" {
			continue
		}
		target, err := s.ResolveStorageTarget(scopeID, localUserID, "")
		if err != nil {
			return nil, err
		}
		resolvedMembers = append(resolvedMembers, resolvedMember{entry: entry, target: target})
	}

	type resolvedOverride struct {
		row    schedule.DayOverrideRow
		target schedule.StorageTarget
	}
	resolvedOverrides := make([]resolvedOverride, 0, len(backup.Overrides))
	for _, row := range backup.Overrides {
		localUserID := strings.TrimSpace(row.UserID)
		if row.Day == "" || row.Kind == "" || localUserID == "" {
			continue
		}
		target := schedule.StorageTarget{
			ScopeID:     scopeID,
			UserID:      schedule.DayOverrideAll,
			LocalUserID: schedule.DayOverrideAll,
		}
		if localUserID != schedule.DayOverrideAll {
			target, err = s.ResolveStorageTarget(scopeID, localUserID, "")
			if err != nil {
				return nil, err
			}
		}
		resolvedOverrides = append(resolvedOverrides, resolvedOverride{row: row, target: target})
	}

	result := &ImportResult{}
	for _, resolved := range resolvedMembers {
		entry := resolved.entry
		target := resolved.target
		_, found, err := s.storage.GetMember(target.ScopeID, target.UserID)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = target.LocalUserID
		}
		qq, qqErr := schedule.NormalizeQQ(entry.QQ)
		if qqErr != nil {
			qq = ""
		}
		member := &schedule.Member{
			UserID:            target.UserID,
			Name:              name,
			QQ:                qq,
			Events:            entry.Events,
			ICS:               entry.ICS,
			Source:            firstNonEmpty(entry.Source, "ics"),
			EventCount:        len(entry.Events),
			UpdatedAt:         schedule.NowISO(),
			ScheduleUpdatedAt: schedule.NowISO(),
			LastModifiedAt:    schedule.NowISO(),
			LastModifiedBy:    actor,
		}
		member.Schedule = schedule.FormatICSSchedule(member.Events)
		if err := s.storage.PutMember(target.ScopeID, target.UserID, member, nil); err != nil {
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

	if err := s.clearBackupOverrides(scopeID, settings); err != nil {
		return nil, err
	}
	seenOverrides := make(map[string]bool)
	for _, resolved := range resolvedOverrides {
		override := resolved.row
		target := resolved.target
		key := target.ScopeID + "\x00" + target.UserID + "\x00" + override.Day
		if seenOverrides[key] {
			continue
		}
		seenOverrides[key] = true
		createdBy := firstNonEmpty(override.CreatedBy, actor)
		createdAt := firstNonEmpty(override.CreatedAt, schedule.NowISO())
		if err := s.storage.SetDayOverride(target.ScopeID, target.UserID, override.Day,
			schedule.DayOverride{Kind: override.Kind, SourceDay: override.SourceDay}, createdBy, createdAt); err != nil {
			return nil, err
		}
		result.OverrideCount++
	}
	return result, nil
}

func (s *Service) clearBackupOverrides(scopeID string, settings schedule.Settings) error {
	if err := s.storage.DeleteScopeDayOverrides(scopeID); err != nil {
		return err
	}
	if settings.StorageMode != schedule.StorageModeGlobal {
		return nil
	}

	members, err := s.VisibleMembers(scopeID)
	if err != nil {
		return err
	}
	targets, err := s.dayOverrideTargets(scopeID, members)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target.UserID == schedule.DayOverrideAll {
			continue
		}
		rows, err := s.storage.ListDayOverrides(target.ScopeID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.UserID != target.UserID {
				continue
			}
			if _, err := s.storage.DeleteDayOverride(target.ScopeID, target.UserID, row.Day); err != nil {
				return err
			}
		}
	}
	return nil
}
