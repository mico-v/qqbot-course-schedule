package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Mention is one @ user parsed from an inbound message.
type Mention struct {
	ID   string
	Name string
}

func isOwnQuery(query string) bool {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "自己", "我", "本人", "me", "my", "self":
		return true
	default:
		return false
	}
}

// ResolveOverrideTargets resolves who a 休假/调休/销假 command applies to.
// Targets are member ids, or DayOverrideAll for the whole scope.
func ResolveOverrideTargets(members map[string]*Member, senderID string, isGroup, isAdmin bool, person string, mentions []Mention) ([]string, string) {
	query := strings.TrimSpace(person)
	if query == "" || isOwnQuery(query) {
		if isAdmin && isGroup {
			return []string{DayOverrideAll}, ""
		}
		return []string{senderID}, ""
	}
	if !isGroup {
		return nil, "私聊只能标记自己的假期。"
	}
	switch strings.ToLower(query) {
	case "all", "全部", "所有", "大家", "全体", "全群":
		if isAdmin {
			return []string{DayOverrideAll}, ""
		}
		return nil, "只有管理员可以把标记应用到全体成员。"
	}

	checkPermission := func(target string) ([]string, string) {
		if target == senderID || isAdmin {
			return []string{target}, ""
		}
		return nil, "普通成员只能标记自己的假期；标记群内其他成员的假期需要管理员权限。"
	}

	if _, ok := members[query]; ok {
		return checkPermission(query)
	}

	var matched []string
	for userID, member := range members {
		if member == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(member.Name), query) {
			matched = append(matched, userID)
		}
	}
	sort.Strings(matched)
	switch {
	case len(matched) == 1:
		return checkPermission(matched[0])
	case len(matched) > 1:
		labels := make([]string, 0, 10)
		for _, userID := range matched {
			if len(labels) >= 10 {
				break
			}
			labels = append(labels, fmt.Sprintf("%s(%s)", OverrideLabel(members, userID), userID))
		}
		return nil, "昵称存在多个精确匹配，请改用成员 OpenID：\n" + strings.Join(labels, "\n")
	}

	if len(mentions) > 0 {
		var named []string
		for _, mention := range mentions {
			if mention.ID != "" && strings.EqualFold(strings.TrimSpace(mention.Name), query) {
				named = append(named, mention.ID)
			}
		}
		if len(named) == 1 {
			return checkPermission(named[0])
		}
		if len(named) > 1 {
			return nil, "消息中有多个同名 @ 用户，请直接提供成员 OpenID。"
		}
		if len(mentions) == 1 {
			return checkPermission(mentions[0].ID)
		}
	}
	return nil, fmt.Sprintf("没有找到成员“%s”的课表，请使用完整昵称。", query)
}

// ResolveMemberTarget resolves a single member for member-scoped commands
// (currently /导出课表). Returns the user id or a user-facing error message.
func ResolveMemberTarget(members map[string]*Member, senderID string, isGroup, isAdmin bool, person string, mentions []Mention) (string, string) {
	query := strings.TrimSpace(person)
	if query == "" || isOwnQuery(query) {
		return senderID, ""
	}
	if _, ok := members[query]; ok {
		if query == senderID || (isGroup && isAdmin) {
			return query, ""
		}
		return "", "普通成员只能操作自己的课表；操作群内其他成员的课表需要管理员权限。"
	}
	var matched []string
	for userID, member := range members {
		if member != nil && strings.EqualFold(strings.TrimSpace(member.Name), query) {
			matched = append(matched, userID)
		}
	}
	sort.Strings(matched)
	if len(matched) == 1 {
		if matched[0] == senderID || (isGroup && isAdmin) {
			return matched[0], ""
		}
		return "", "普通成员只能操作自己的课表；操作群内其他成员的课表需要管理员权限。"
	}
	if len(matched) > 1 {
		return "", "昵称存在多个精确匹配，请改用成员 OpenID。"
	}
	if len(mentions) > 0 {
		var named []string
		for _, mention := range mentions {
			if mention.ID != "" && strings.EqualFold(strings.TrimSpace(mention.Name), query) {
				named = append(named, mention.ID)
			}
		}
		if len(named) == 1 {
			if named[0] == senderID || (isGroup && isAdmin) {
				return named[0], ""
			}
			return "", "普通成员只能操作自己的课表；操作群内其他成员的课表需要管理员权限。"
		}
	}
	return "", fmt.Sprintf("没有找到成员“%s”的课表，请使用完整昵称。", query)
}

// OverrideLabel renders "全体成员" or "昵称(openid)" for replies.
func OverrideLabel(members map[string]*Member, targetID string) string {
	if targetID == DayOverrideAll {
		return "全体成员"
	}
	if member, ok := members[targetID]; ok && member != nil && strings.TrimSpace(member.Name) != "" {
		return fmt.Sprintf("%s(%s)", member.Name, targetID)
	}
	return targetID
}

// SetDayOverrides writes holiday/shift markers and returns the user-facing reply.
func (s *Service) SetDayOverrides(scopeID string, targets []string, days []time.Time, kind string, sourceDay *time.Time, actor string, members map[string]*Member, today time.Time) (string, error) {
	if len(days) == 0 {
		return "", fmt.Errorf("请提供要标记的日期，例如 /休假 2026-10-01。")
	}
	if len(days) > MaxDayOverrideRangeDays {
		return "", fmt.Errorf("一次最多标记 %d 天，请拆分后重试。", MaxDayOverrideRangeDays)
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("没有确定要标记的成员。")
	}
	if kind == DayOverrideShift {
		if sourceDay == nil {
			return "", fmt.Errorf("调休需要同时提供来源日期，例如 /调休 2026-10-11 2026-10-08。")
		}
		for _, day := range days {
			if sameDay(day, *sourceDay) {
				return "", fmt.Errorf("调休的来源日期不能和调休日期相同。")
			}
		}
		if absInt(int(sourceDay.Sub(days[0]).Hours()/24)) > MaxDayOverrideSpanDays {
			return "", fmt.Errorf("调休的来源日期与目标日期相差不能超过 %d 天。", MaxDayOverrideSpanDays)
		}
	}

	existing, err := s.store.ListDayOverrides(scopeID)
	if err != nil {
		return "", err
	}
	existingKeys := make(map[string]DayOverrideRow, len(existing))
	for _, row := range existing {
		existingKeys[row.UserID+":"+row.Day] = row
	}
	added := 0
	for _, target := range targets {
		for _, day := range days {
			if _, ok := existingKeys[target+":"+day.Format("2006-01-02")]; !ok {
				added++
			}
		}
	}
	if len(existing)+added > MaxDayOverridesPerScope {
		return "", fmt.Errorf("本会话的休假/调休标记已达上限 %d 条，请先取消一些标记。", MaxDayOverridesPerScope)
	}

	sourceText := ""
	if kind == DayOverrideShift && sourceDay != nil {
		sourceText = sourceDay.Format("2006-01-02")
	}
	nowISO := NowISO()
	replaced := false
	for _, target := range targets {
		for _, day := range days {
			dayText := day.Format("2006-01-02")
			if row, ok := existingKeys[target+":"+dayText]; ok && row.Kind != kind {
				replaced = true
			}
			override := DayOverride{Kind: kind, SourceDay: sourceText}
			if err := s.store.SetDayOverride(scopeID, target, dayText, override, actor, nowISO); err != nil {
				return "", err
			}
		}
	}

	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		labels = append(labels, OverrideLabel(members, target))
	}
	label := strings.Join(labels, "、") + DayCountText(days)
	dayText := FormatDayList(days)
	var action string
	if kind == DayOverrideShift {
		action = fmt.Sprintf("已将 %s 标记为调休（%s）：当天课程改为 %s 的课程。", dayText, label, sourceDay.Format("2006-01-02"))
	} else {
		action = fmt.Sprintf("已将 %s 标记为休假（%s）：当天课程全部取消。", dayText, label)
	}
	var notes []string
	if replaced {
		notes = append(notes, "原有标记已被覆盖")
	}
	if minDay(days).Before(beginningOfDay(today)) {
		notes = append(notes, "包含已过去的日期，只影响查询与统计")
	}
	if len(notes) > 0 {
		action += "（" + strings.Join(notes, "，") + "）"
	}
	return action, nil
}

// ClearDayOverrides removes markers and returns the user-facing reply.
func (s *Service) ClearDayOverrides(scopeID string, targets []string, days []time.Time, members map[string]*Member) (string, error) {
	if len(days) == 0 {
		return "", fmt.Errorf("请提供要取消标记的日期，例如 /销假 2026-10-01。")
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("没有确定要取消标记的成员。")
	}
	existing, err := s.store.ListDayOverrides(scopeID)
	if err != nil {
		return "", err
	}
	existingKeys := make(map[string]DayOverrideRow, len(existing))
	for _, row := range existing {
		existingKeys[row.UserID+":"+row.Day] = row
	}

	var removed []time.Time
	removedKinds := make(map[string]bool)
	var scopeWide []time.Time
	for _, target := range targets {
		for _, day := range days {
			dayText := day.Format("2006-01-02")
			row, ok := existingKeys[target+":"+dayText]
			if !ok {
				if target != DayOverrideAll {
					if _, ok := existingKeys[DayOverrideAll+":"+dayText]; ok {
						scopeWide = append(scopeWide, day)
					}
				}
				continue
			}
			deleted, err := s.store.DeleteDayOverride(scopeID, target, dayText)
			if err != nil {
				return "", err
			}
			if deleted {
				removed = append(removed, day)
				removedKinds[row.Kind] = true
			}
		}
	}

	if len(removed) == 0 && len(scopeWide) == 0 {
		return fmt.Sprintf("%s 没有可取消的休假/调休标记。", FormatDayList(days)), nil
	}
	kindText := "休假/调休"
	if len(removedKinds) == 1 {
		for kind := range removedKinds {
			if kind == DayOverrideHoliday {
				kindText = "休假"
			} else {
				kindText = "调休"
			}
		}
	}
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		labels = append(labels, OverrideLabel(members, target))
	}
	displayDays := removed
	if len(displayDays) == 0 {
		displayDays = days
	}
	label := strings.Join(labels, "、") + DayCountText(displayDays)
	var parts []string
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("已取消 %s 的%s标记（%s）。", FormatDayList(removed), kindText, label))
	}
	if len(scopeWide) > 0 {
		parts = append(parts, fmt.Sprintf("%s 是面向全体成员的标记，请让管理员使用 /销假 取消。", FormatDayList(scopeWide)))
	}
	return strings.Join(parts, ""), nil
}

// DayOverrideListText renders the /假期 reply for a scope.
func (s *Service) DayOverrideListText(scopeID string, members map[string]*Member, today time.Time) (string, error) {
	rows, err := s.store.ListDayOverrides(scopeID)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "当前会话还没有休假/调休标记。\n" +
			"/休假 <日期> [成员] 取消当天全部课程；" +
			"/调休 <日期> <来源日期> [成员] 改为上来源日期的课程。", nil
	}
	shown := rows
	if len(shown) > 50 {
		shown = shown[:50]
	}
	today = beginningOfDay(today)
	lines := []string{fmt.Sprintf("当前会话的休假/调休标记（共 %d 条）：", len(rows))}
	for _, row := range shown {
		day, err := time.ParseInLocation("2006-01-02", row.Day, LocalTZ)
		if err != nil {
			continue
		}
		dayText := day.Format("2006-01-02")
		if day.Before(today) {
			dayText += "（已过去）"
		}
		label := OverrideLabel(members, row.UserID)
		if row.Kind == DayOverrideShift && row.SourceDay != "" {
			source, err := time.ParseInLocation("2006-01-02", row.SourceDay, LocalTZ)
			if err == nil {
				lines = append(lines, fmt.Sprintf("- %s 调休：按 %s 的课程上课（%s）", dayText, source.Format("2006-01-02"), label))
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("- %s 休假：当天课程全部取消（%s）", dayText, label))
	}
	if len(rows) > len(shown) {
		lines = append(lines, fmt.Sprintf("共 %d 条，仅展示前 %d 条。", len(rows), len(shown)))
	}
	lines = append(lines, "取消标记：/销假 <日期> [成员]")
	return strings.Join(lines, "\n"), nil
}

func sameDay(left, right time.Time) bool {
	return left.Format("2006-01-02") == right.Format("2006-01-02")
}

func minDay(days []time.Time) time.Time {
	min := beginningOfDay(days[0])
	for _, day := range days[1:] {
		if day.Before(min) {
			min = beginningOfDay(day)
		}
	}
	return min
}
