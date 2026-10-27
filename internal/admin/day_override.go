package admin

import (
	"sort"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// WebDayOverrides lists one group's markers with display names.
func (s *Service) WebDayOverrides(scopeID string) ([]WebDayOverride, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, schedule.ValidationErrorf("scope_id 不能为空。")
	}
	members, err := s.domain.VisibleMembers(scopeID)
	if err != nil {
		return nil, err
	}
	targets, err := s.dayOverrideTargets(scopeID, members)
	if err != nil {
		return nil, err
	}
	rows, err := s.domain.DayOverrideRowsForTargets(targets)
	if err != nil {
		return nil, err
	}
	result := make([]WebDayOverride, 0, len(rows))
	for _, row := range rows {
		result = append(result, WebDayOverride{
			UserID: row.UserID, Name: overrideDisplayName(members, row.UserID),
			Day: row.Day, Kind: row.Kind, SourceDay: row.SourceDay,
			CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func (s *Service) dayOverrideTargets(scopeID string, members map[string]*schedule.Member) ([]schedule.DayOverrideTarget, error) {
	targets := []schedule.DayOverrideTarget{{
		ScopeID: scopeID, UserID: schedule.DayOverrideAll, DisplayID: schedule.DayOverrideAll,
	}}
	userIDs := make([]string, 0, len(members))
	for userID := range members {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)
	for _, userID := range userIDs {
		target, err := s.domain.ResolveStorageTarget(scopeID, userID, "")
		if err != nil {
			return nil, err
		}
		targets = append(targets, schedule.DayOverrideTarget{
			ScopeID: target.ScopeID, UserID: target.UserID, DisplayID: userID,
		})
	}
	return targets, nil
}

func overrideDisplayName(members map[string]*schedule.Member, userID string) string {
	if userID == schedule.DayOverrideAll {
		return "全体成员"
	}
	if member, ok := members[userID]; ok && member != nil && strings.TrimSpace(member.Name) != "" {
		return member.Name
	}
	return userID
}

// SetWebDayOverride writes one marker from the admin page.
func (s *Service) SetWebDayOverride(scopeID, userID, day, kind, sourceDay, actor string) error {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return schedule.ValidationErrorf("scope_id 和 user_id 不能为空。")
	}
	target, err := parseWebDay(day, "日期")
	if err != nil {
		return err
	}
	if kind != schedule.DayOverrideHoliday && kind != schedule.DayOverrideShift {
		return schedule.ValidationErrorf("标记类型只能是休假或调休。")
	}
	storageTarget := schedule.StorageTarget{ScopeID: scopeID, UserID: userID, LocalUserID: userID}
	if userID != schedule.DayOverrideAll {
		members, err := s.domain.VisibleMembers(scopeID)
		if err != nil {
			return err
		}
		if _, ok := members[userID]; !ok {
			return schedule.NotFoundErrorf("找不到该成员，请先为其创建课表。")
		}
		storageTarget, err = s.domain.ResolveStorageTarget(scopeID, userID, "")
		if err != nil {
			return err
		}
	}
	var source *time.Time
	if kind == schedule.DayOverrideShift {
		parsed, err := parseWebDay(sourceDay, "调休来源日期")
		if err != nil {
			return err
		}
		if sameDay(parsed, target) {
			return schedule.ValidationErrorf("调休的来源日期不能和调休日期相同。")
		}
		if !schedule.SameAcademicYear(parsed, target) {
			return schedule.ValidationErrorf("调休只能在同一个学期内进行（8 月 1 日为学期分界）。")
		}
		source = &parsed
	}
	dayText := target.Format("2006-01-02")
	rows, err := s.overrides.ListDayOverrides(storageTarget.ScopeID)
	if err != nil {
		return err
	}
	found := false
	for _, row := range rows {
		if row.UserID == storageTarget.UserID && row.Day == dayText {
			found = true
			break
		}
	}
	if !found && len(rows) >= schedule.MaxDayOverridesPerScope {
		return schedule.ValidationErrorf(
			"本会话的休假/调休标记已达上限 %d 条，请先取消一些标记。",
			schedule.MaxDayOverridesPerScope,
		)
	}
	override := schedule.DayOverride{Kind: kind}
	if source != nil {
		override.SourceDay = source.Format("2006-01-02")
	}
	return s.overrides.SetDayOverride(
		storageTarget.ScopeID,
		storageTarget.UserID,
		dayText,
		override,
		firstNonEmpty(actor, "webui"),
		schedule.NowISO(),
	)
}

// DeleteWebDayOverride removes one marker from the admin page.
func (s *Service) DeleteWebDayOverride(scopeID, userID, day string) (bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return false, schedule.ValidationErrorf("scope_id 和 user_id 不能为空。")
	}
	target, err := parseWebDay(day, "日期")
	if err != nil {
		return false, err
	}
	storageTarget := schedule.StorageTarget{ScopeID: scopeID, UserID: userID, LocalUserID: userID}
	if userID != schedule.DayOverrideAll {
		storageTarget, err = s.domain.ResolveStorageTarget(scopeID, userID, "")
		if err != nil {
			return false, err
		}
	}
	return s.overrides.DeleteDayOverride(
		storageTarget.ScopeID,
		storageTarget.UserID,
		target.Format("2006-01-02"),
	)
}

func parseWebDay(value, label string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, schedule.ValidationErrorf("请提供%s。", label)
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, schedule.LocalTZ)
	if err != nil {
		return time.Time{}, schedule.ValidationErrorf("%s格式应为 YYYY-MM-DD。", label)
	}
	return parsed, nil
}
