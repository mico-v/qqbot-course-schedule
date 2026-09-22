package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ScopeMemberSummary is one member row in the admin scope list.
type ScopeMemberSummary struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	EventCount int    `json:"event_count"`
	Revision   int64  `json:"revision"`
}

// ScopeSummary is one scope with its members (storage-level shape).
type ScopeSummary struct {
	ScopeID string               `json:"scope_id"`
	Members []ScopeMemberSummary `json:"members"`
}

// ScopeSummaries lists every scope that has saved schedules, plus scopes whose
// members only interacted with the bot (so their first schedule can be created).
func (s *Service) ScopeSummaries() ([]ScopeSummary, error) {
	summaries, err := s.store.ListScopeSummaries()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(summaries))
	for _, summary := range summaries {
		known[summary.ScopeID] = true
	}
	entries, err := s.store.ListKV("global", seenNamespace)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if known[entry.Key] {
			continue
		}
		if kind, _ := ParseScope(entry.Key); kind != "group" && kind != "private" {
			continue
		}
		summaries = append(summaries, ScopeSummary{ScopeID: entry.Key})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ScopeID < summaries[j].ScopeID })
	return summaries, nil
}

// WebEvent is one event as the admin page sees it.
type WebEvent struct {
	ID          int    `json:"id"`
	UID         string `json:"uid"`
	Course      string `json:"course"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Start       string `json:"start"`
	End         string `json:"end"`
	RRule       string `json:"rrule"`
}

// PageSchedule is the admin page payload for one member.
type PageSchedule struct {
	ScopeID  string     `json:"scope_id"`
	UserID   string     `json:"user_id"`
	Name     string     `json:"name"`
	Revision int64      `json:"revision"`
	Events   []WebEvent `json:"events"`
}

// PageSchedule loads one member for the admin editor.
func (s *Service) PageSchedule(scopeID, userID string) (*PageSchedule, bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return nil, false, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	member, found, err := s.store.GetMember(scopeID, userID)
	if err != nil || !found || member == nil {
		return nil, false, err
	}
	events := make([]WebEvent, 0, len(member.Events))
	for index, event := range member.Events {
		events = append(events, WebEvent{
			ID:          index + 1,
			UID:         event["UID"],
			Course:      event["SUMMARY"],
			Location:    event["LOCATION"],
			Description: event["DESCRIPTION"],
			Start:       webDateTimeValue(event["DTSTART"], event["DTSTART_TZID"]),
			End:         webDateTimeValue(event["DTEND"], event["DTEND_TZID"]),
			RRule:       event["RRULE"],
		})
	}
	return &PageSchedule{
		ScopeID:  scopeID,
		UserID:   userID,
		Name:     firstNonEmpty(member.Name, userID),
		Revision: member.Revision,
		Events:   events,
	}, true, nil
}

func webDateTimeValue(value, tzid string) string {
	parsed, ok := ParseICSTime(value, tzid)
	if !ok {
		return ""
	}
	return parsed.In(LocalTZ).Format("2006-01-02T15:04")
}

// WebEventInput is one event submitted by the admin page.
type WebEventInput struct {
	ID          int    `json:"id"`
	UID         string `json:"uid"`
	Course      string `json:"course"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Start       string `json:"start"`
	End         string `json:"end"`
	RRule       string `json:"rrule"`
}

// SavePagePayload is the admin save request.
type SavePagePayload struct {
	ScopeID  string          `json:"scope_id"`
	UserID   string          `json:"user_id"`
	Revision *int64          `json:"revision"`
	Name     *string         `json:"name"`
	Events   []WebEventInput `json:"events"`
}

// SavePageResult is the admin save response.
type SavePageResult struct {
	ScopeID    string `json:"scope_id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Revision   int64  `json:"revision"`
	EventCount int    `json:"event_count"`
}

// SavePageSchedule validates and stores a member's whole schedule with the
// revision the page read, preserving RAW_ICAL for events matched by uid/index.
func (s *Service) SavePageSchedule(payload SavePagePayload, actor string) (*SavePageResult, error) {
	scopeID := strings.TrimSpace(payload.ScopeID)
	userID := strings.TrimSpace(payload.UserID)
	if scopeID == "" || userID == "" {
		return nil, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	kind, _ := ParseScope(scopeID)
	if kind != "group" && kind != "private" {
		return nil, fmt.Errorf("无效的 scope_id。")
	}
	if payload.Revision == nil {
		return nil, fmt.Errorf("缺少有效的课程表 revision，请刷新后重试。")
	}
	expected := *payload.Revision
	if len(payload.Events) > MaxEventsPerFile {
		return nil, fmt.Errorf("单个成员最多保存 %d 节课程。", MaxEventsPerFile)
	}

	current, found, err := s.store.GetMember(scopeID, userID)
	if err != nil {
		return nil, err
	}
	if !found || current == nil {
		return nil, fmt.Errorf("找不到指定群组中的成员课程表。")
	}

	events := make([]Event, 0, len(payload.Events))
	for index, input := range payload.Events {
		course := strings.TrimSpace(input.Course)
		if course == "" {
			return nil, fmt.Errorf("第 %d 节课程缺少课程名称。", index+1)
		}
		if len([]rune(course)) > MaxCourseNameLength {
			return nil, fmt.Errorf("第 %d 节课程名称不能超过 %d 个字符。", index+1, MaxCourseNameLength)
		}
		location := strings.TrimSpace(input.Location)
		description := strings.TrimSpace(input.Description)
		rrule := strings.TrimSpace(input.RRule)
		if len([]rune(location)) > MaxCourseNameLength ||
			len([]rune(description)) > MaxDescriptionLength ||
			len([]rune(rrule)) > MaxRRuleLength {
			return nil, fmt.Errorf("第 %d 节课程的文本字段过长。", index+1)
		}
		event, err := MakeEvent(course, strings.TrimSpace(input.Start), strings.TrimSpace(input.End), location, description, rrule, strings.TrimSpace(input.UID))
		if err != nil {
			return nil, fmt.Errorf("第 %d 节课程：%v", index+1, err)
		}
		if original := findOriginalEvent(current.Events, input); original != nil {
			if rawICAL := original["RAW_ICAL"]; rawICAL != "" {
				event["RAW_ICAL"] = rawICAL
			}
		}
		events = append(events, event)
	}
	sortEvents(events)

	now := NowISO()
	updated := *current
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		if name == "" {
			return nil, fmt.Errorf("成员名称不能为空。")
		}
		if len([]rune(name)) > MaxMemberNameLength {
			return nil, fmt.Errorf("成员名称不能超过 %d 个字符。", MaxMemberNameLength)
		}
		updated.Name = name
	}
	updated.Events = events
	updated.ICS = SerializeScheduleICS(events, current.ICS, updated.Name)
	updated.Schedule = FormatICSSchedule(events)
	updated.Source = "ics"
	updated.EventCount = len(events)
	updated.UpdatedAt = now
	updated.ScheduleUpdatedAt = now
	updated.LastModifiedAt = now
	updated.LastModifiedBy = actor
	if err := s.store.PutMember(scopeID, userID, &updated, &expected); err != nil {
		return nil, err
	}
	return &SavePageResult{
		ScopeID:    scopeID,
		UserID:     userID,
		Name:       firstNonEmpty(updated.Name, userID),
		Revision:   expected + 1,
		EventCount: len(events),
	}, nil
}

func findOriginalEvent(events []Event, input WebEventInput) Event {
	if uid := strings.TrimSpace(input.UID); uid != "" {
		for _, event := range events {
			if event["UID"] == uid {
				return event
			}
		}
	}
	if input.ID >= 1 && input.ID <= len(events) {
		return events[input.ID-1]
	}
	return nil
}

// NewMember is one member to create an empty schedule for.
type NewMember struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// CreateMemberSchedules creates empty schedules, skipping existing members.
func (s *Service) CreateMemberSchedules(scopeID string, inputs []NewMember, actor string) ([]NewMember, error) {
	scopeID = strings.TrimSpace(scopeID)
	kind, _ := ParseScope(scopeID)
	if kind != "group" && kind != "private" {
		return nil, fmt.Errorf("无效的 scope_id。")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("members 不能为空。")
	}
	if len(inputs) > MaxMembersPerCreate {
		return nil, fmt.Errorf("一次最多创建 %d 位成员。", MaxMembersPerCreate)
	}
	var created []NewMember
	for _, input := range inputs {
		userID := strings.TrimSpace(input.UserID)
		if userID == "" {
			continue
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = userID
		}
		if len([]rune(name)) > MaxMemberNameLength {
			return nil, fmt.Errorf("成员名称不能超过 %d 个字符。", MaxMemberNameLength)
		}
		member := &Member{
			UserID:         userID,
			Name:           name,
			Source:         "manual",
			UpdatedAt:      NowISO(),
			LastModifiedBy: actor,
		}
		zero := int64(0)
		if err := s.store.PutMember(scopeID, userID, member, &zero); err != nil {
			if err == ErrConflict {
				continue
			}
			return nil, err
		}
		created = append(created, NewMember{UserID: userID, Name: name})
	}
	if len(created) == 0 {
		return nil, fmt.Errorf("所选成员都已经有课表了。")
	}
	return created, nil
}

const (
	seenNamespace   = "seen"
	seenMemberLimit = 500
)

// RecordSeenMember remembers a member who interacted with the bot, so the admin
// page can offer empty schedules for people without one.
func (s *Service) RecordSeenMember(scopeID, userID, name string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	var seen map[string]string
	if _, err := s.store.GetKV("global", seenNamespace, scopeID, &seen); err != nil {
		return err
	}
	if seen == nil {
		seen = make(map[string]string)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = userID
	}
	if existing, ok := seen[userID]; ok && existing == name {
		return nil
	}
	if len(seen) >= seenMemberLimit {
		if _, exists := seen[userID]; !exists {
			for key := range seen {
				delete(seen, key)
				break
			}
		}
	}
	seen[userID] = name
	return s.store.SetKV("global", seenNamespace, scopeID, seen)
}

// WebDayOverride is one holiday/shift marker in the admin page.
type WebDayOverride struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	SourceDay string `json:"source_day,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// WebDayOverrides lists one scope's markers with display names.
func (s *Service) WebDayOverrides(scopeID string) ([]WebDayOverride, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, fmt.Errorf("scope_id 不能为空。")
	}
	rows, err := s.store.ListDayOverrides(scopeID)
	if err != nil {
		return nil, err
	}
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, err
	}
	result := make([]WebDayOverride, 0, len(rows))
	for _, row := range rows {
		result = append(result, WebDayOverride{
			UserID:    row.UserID,
			Name:      overrideDisplayName(members, row.UserID),
			Day:       row.Day,
			Kind:      row.Kind,
			SourceDay: row.SourceDay,
			CreatedBy: row.CreatedBy,
			CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func overrideDisplayName(members map[string]*Member, userID string) string {
	if userID == DayOverrideAll {
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
		return fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	target, err := parseWebDay(day, "日期")
	if err != nil {
		return err
	}
	if kind != DayOverrideHoliday && kind != DayOverrideShift {
		return fmt.Errorf("标记类型只能是休假或调休。")
	}
	if userID != DayOverrideAll {
		members, err := s.store.GetScopeMembers(scopeID)
		if err != nil {
			return err
		}
		if _, ok := members[userID]; !ok {
			return fmt.Errorf("找不到该成员，请先为其创建课表。")
		}
	}
	var source *time.Time
	if kind == DayOverrideShift {
		parsed, err := parseWebDay(sourceDay, "调休来源日期")
		if err != nil {
			return err
		}
		if sameDay(parsed, target) {
			return fmt.Errorf("调休的来源日期不能和调休日期相同。")
		}
		if absInt(int(parsed.Sub(target).Hours()/24)) > MaxDayOverrideSpanDays {
			return fmt.Errorf("调休的来源日期与目标日期相差不能超过 %d 天。", MaxDayOverrideSpanDays)
		}
		source = &parsed
	}

	dayText := target.Format("2006-01-02")
	rows, err := s.store.ListDayOverrides(scopeID)
	if err != nil {
		return err
	}
	found := false
	for _, row := range rows {
		if row.UserID == userID && row.Day == dayText {
			found = true
			break
		}
	}
	if !found && len(rows) >= MaxDayOverridesPerScope {
		return fmt.Errorf("本会话的休假/调休标记已达上限 %d 条，请先取消一些标记。", MaxDayOverridesPerScope)
	}
	override := DayOverride{Kind: kind}
	if source != nil {
		override.SourceDay = source.Format("2006-01-02")
	}
	return s.store.SetDayOverride(scopeID, userID, dayText, override, firstNonEmpty(actor, "webui"), NowISO())
}

// DeleteWebDayOverride removes one marker from the admin page.
func (s *Service) DeleteWebDayOverride(scopeID, userID, day string) (bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return false, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	target, err := parseWebDay(day, "日期")
	if err != nil {
		return false, err
	}
	return s.store.DeleteDayOverride(scopeID, userID, target.Format("2006-01-02"))
}

func parseWebDay(value, label string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("请提供%s。", label)
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, LocalTZ)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s格式应为 YYYY-MM-DD。", label)
	}
	return parsed, nil
}

// PendingMembers returns observed members who do not have a schedule yet.
func (s *Service) PendingMembers(scopeID string) ([]NewMember, error) {
	var seen map[string]string
	if _, err := s.store.GetKV("global", seenNamespace, scopeID, &seen); err != nil {
		return nil, err
	}
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, err
	}
	var pending []NewMember
	for userID, name := range seen {
		if _, exists := members[userID]; exists {
			continue
		}
		pending = append(pending, NewMember{UserID: userID, Name: name})
	}
	sort.Slice(pending, func(i, j int) bool {
		left, right := strings.ToLower(pending[i].Name), strings.ToLower(pending[j].Name)
		if left != right {
			return left < right
		}
		return pending[i].UserID < pending[j].UserID
	})
	return pending, nil
}
