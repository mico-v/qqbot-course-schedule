package admin

import (
	"context"
	"strings"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// PageSchedule loads one member for the admin editor.
func (s *Service) PageSchedule(scopeID, userID string) (*PageSchedule, bool, error) {
	return s.PageScheduleContext(context.Background(), scopeID, userID)
}

// PageScheduleContext loads the admin editor with request cancellation.
func (s *Service) PageScheduleContext(ctx context.Context, scopeID, userID string) (*PageSchedule, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key, err := schedule.GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return nil, false, err
	}
	member, found, err := s.schedules.GetMember(key.Group().ScopeID(), key.MemberOpenID)
	if err != nil {
		return nil, false, err
	}
	if !found || member == nil {
		return nil, false, nil
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
		ScopeID:  key.Group().ScopeID(),
		UserID:   key.MemberOpenID,
		Name:     firstNonEmpty(member.Name, key.MemberOpenID),
		QQ:       member.QQ,
		Revision: member.Revision,
		Events:   events,
	}, true, nil
}

// PageScheduleAt loads a member by exact storage coordinates.
func (s *Service) PageScheduleAt(scopeID, userID string) (*PageSchedule, bool, error) {
	return s.PageScheduleAtContext(context.Background(), scopeID, userID)
}

// PageScheduleAtContext loads exact coordinates with request cancellation.
func (s *Service) PageScheduleAtContext(ctx context.Context, scopeID, userID string) (*PageSchedule, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key, err := schedule.GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return nil, false, err
	}
	member, found, err := s.schedules.GetMember(key.Group().ScopeID(), key.MemberOpenID)
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
		ScopeID:  key.Group().ScopeID(),
		UserID:   key.MemberOpenID,
		Name:     firstNonEmpty(member.Name, key.MemberOpenID),
		QQ:       member.QQ,
		Revision: member.Revision,
		Events:   events,
	}, true, nil
}

func webDateTimeValue(value, tzid string) string {
	parsed, ok := schedule.ParseICSTime(value, tzid)
	if !ok {
		return ""
	}
	return parsed.In(schedule.LocalTZ).Format("2006-01-02T15:04")
}

// SavePageSchedule validates and stores one complete schedule.
func (s *Service) SavePageSchedule(payload SavePagePayload, actor string) (*SavePageResult, error) {
	return s.SavePageScheduleContext(context.Background(), payload, actor)
}

// SavePageScheduleContext validates and saves an admin page with cancellation.
func (s *Service) SavePageScheduleContext(ctx context.Context, payload SavePagePayload, actor string) (*SavePageResult, error) {
	key, err := schedule.GroupMemberKeyFromLegacy(payload.ScopeID, payload.UserID)
	if err != nil {
		return nil, err
	}
	payload.ScopeID = key.Group().ScopeID()
	payload.UserID = key.MemberOpenID
	result, err := s.SavePageScheduleAtContext(ctx, payload, actor)
	if err != nil {
		return nil, err
	}
	result.ScopeID = payload.ScopeID
	result.UserID = payload.UserID
	return result, nil
}

func hasCourses(member *schedule.Member) bool {
	return member != nil && len(member.Events) > 0
}

// SavePageScheduleAt saves a member by exact storage coordinates.
func (s *Service) SavePageScheduleAt(payload SavePagePayload, actor string) (*SavePageResult, error) {
	return s.SavePageScheduleAtContext(context.Background(), payload, actor)
}

// SavePageScheduleAtContext saves exact coordinates with cancellation.
func (s *Service) SavePageScheduleAtContext(ctx context.Context, payload SavePagePayload, actor string) (*SavePageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := schedule.GroupMemberKeyFromLegacy(payload.ScopeID, payload.UserID)
	if err != nil {
		return nil, err
	}
	if payload.Revision == nil {
		return nil, schedule.ValidationErrorf("缺少有效的课程表 revision，请刷新后重试。")
	}
	expected := *payload.Revision
	if len(payload.Events) > schedule.MaxEventsPerFile {
		return nil, schedule.ValidationErrorf("单个成员最多保存 %d 节课程。", schedule.MaxEventsPerFile)
	}
	current, found, err := s.schedules.GetMember(key.Group().ScopeID(), key.MemberOpenID)
	if err != nil {
		return nil, err
	}
	if !found || current == nil {
		return nil, schedule.NotFoundErrorf("找不到指定群组中的成员课程表。")
	}

	events := make([]schedule.Event, 0, len(payload.Events))
	for index, input := range payload.Events {
		course := strings.TrimSpace(input.Course)
		if course == "" {
			return nil, schedule.ValidationErrorf("第 %d 节课程缺少课程名称。", index+1)
		}
		if len([]rune(course)) > schedule.MaxCourseNameLength {
			return nil, schedule.ValidationErrorf(
				"第 %d 节课程名称不能超过 %d 个字符。",
				index+1,
				schedule.MaxCourseNameLength,
			)
		}
		location := strings.TrimSpace(input.Location)
		description := strings.TrimSpace(input.Description)
		rrule := strings.TrimSpace(input.RRule)
		if len([]rune(location)) > schedule.MaxCourseNameLength ||
			len([]rune(description)) > schedule.MaxDescriptionLength ||
			len([]rune(rrule)) > schedule.MaxRRuleLength {
			return nil, schedule.ValidationErrorf("第 %d 节课程的文本字段过长。", index+1)
		}
		event, err := schedule.MakeEvent(
			course,
			strings.TrimSpace(input.Start),
			strings.TrimSpace(input.End),
			location,
			description,
			rrule,
			strings.TrimSpace(input.UID),
		)
		if err != nil {
			return nil, schedule.ValidationErrorf("第 %d 节课程：%v", index+1, err)
		}
		if original := findOriginalEvent(current.Events, input); original != nil {
			if rawICAL := original["RAW_ICAL"]; rawICAL != "" {
				event["RAW_ICAL"] = rawICAL
			}
		}
		events = append(events, event)
	}
	schedule.SortEvents(events)

	now := schedule.NowISO()
	updated := *current
	if payload.Name != nil {
		name := strings.TrimSpace(*payload.Name)
		if name == "" {
			return nil, schedule.ValidationErrorf("成员名称不能为空。")
		}
		if len([]rune(name)) > schedule.MaxMemberNameLength {
			return nil, schedule.ValidationErrorf(
				"成员名称不能超过 %d 个字符。",
				schedule.MaxMemberNameLength,
			)
		}
		updated.Name = name
	}
	if payload.QQ != nil {
		qq, qqErr := schedule.NormalizeQQ(*payload.QQ)
		if qqErr != nil {
			return nil, qqErr
		}
		updated.QQ = qq
	}
	updated.Events = events
	updated.ICS = schedule.SerializeScheduleICS(events, current.ICS, updated.Name)
	updated.Schedule = schedule.FormatICSSchedule(events)
	updated.Source = "ics"
	updated.EventCount = len(events)
	updated.UpdatedAt = now
	updated.ScheduleUpdatedAt = now
	updated.LastModifiedAt = now
	updated.LastModifiedBy = actor
	if err := s.schedules.PutMember(
		key.Group().ScopeID(),
		key.MemberOpenID,
		&updated,
		&expected,
	); err != nil {
		return nil, err
	}
	return &SavePageResult{
		ScopeID:    key.Group().ScopeID(),
		UserID:     key.MemberOpenID,
		Name:       firstNonEmpty(updated.Name, key.MemberOpenID),
		Revision:   expected + 1,
		EventCount: len(events),
	}, nil
}

func findOriginalEvent(events []schedule.Event, input WebEventInput) schedule.Event {
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
