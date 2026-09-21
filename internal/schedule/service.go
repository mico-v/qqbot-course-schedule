package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Service is the application layer every entry point funnels through.
type Service struct {
	store Storage
}

// NewService wires the service to its storage.
func NewService(store Storage) *Service {
	return &Service{store: store}
}

// SaveResult describes a completed ICS import.
type SaveResult struct {
	UserID     string
	Name       string
	EventCount int
	Revision   int64
	Created    bool
}

// SaveICS parses one ICS document and replaces a member's whole schedule.
func (s *Service) SaveICS(scopeID, userID, name, content, sourceFile, actor string) (*SaveResult, error) {
	text := strings.TrimSpace(content)
	if text == "" {
		return nil, fmt.Errorf("ICS 内容为空，未保存。")
	}
	if len([]byte(text)) > MaxICSBytes {
		return nil, fmt.Errorf("ICS 文件超过 %d MiB，未保存。", MaxICSBytes>>20)
	}
	events, scheduleText, err := ParseICSEventsAndSchedule(text)
	if err != nil {
		return nil, fmt.Errorf("ICS 解析失败，未保存：%v", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("ICS 中没有 VEVENT，未保存。")
	}

	previous, found, err := s.store.GetMember(scopeID, userID)
	if err != nil {
		return nil, err
	}
	if !found || previous == nil {
		previous = &Member{}
	}
	now := NowISO()
	updated := *previous
	// An existing name wins: a member renamed in the WebUI keeps that name.
	updated.Name = firstNonEmpty(previous.Name, name, userID)
	updated.Events = events
	updated.ICS = text
	updated.Schedule = scheduleText
	updated.Source = "ics"
	updated.SourceFile = sourceFile
	updated.EventCount = len(events)
	updated.UpdatedAt = now
	updated.ScheduleUpdatedAt = now
	updated.LastModifiedAt = now
	updated.LastModifiedBy = actor

	expected := int64(0)
	if found {
		expected = previous.Revision
	}
	if err := s.store.PutMember(scopeID, userID, &updated, &expected); err != nil {
		return nil, err
	}
	return &SaveResult{
		UserID:     userID,
		Name:       updated.Name,
		EventCount: len(events),
		Revision:   expected + 1,
		Created:    !found,
	}, nil
}

// EnsureMember creates an empty schedule for a member if none exists yet and
// refreshes the stored nickname when the platform reports a new one.
func (s *Service) EnsureMember(scopeID, userID, name string) error {
	member, found, err := s.store.GetMember(scopeID, userID)
	if err != nil {
		return err
	}
	if !found || member == nil {
		created := &Member{
			UserID:     userID,
			Name:       firstNonEmpty(name, userID),
			Events:     nil,
			Source:     "manual",
			UpdatedAt:  NowISO(),
			EventCount: 0,
		}
		zero := int64(0)
		if err := s.store.PutMember(scopeID, userID, created, &zero); err != nil && err != ErrConflict {
			return err
		}
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" || member.Name == name || len([]rune(name)) > MaxMemberNameLength {
		return nil
	}
	member.Name = name
	expected := member.Revision
	return s.store.PutMember(scopeID, userID, member, &expected)
}

// DayCard is everything the renderer needs for one day view.
type DayCard struct {
	Title       string
	Subtitle    string
	FoldedTitle string
	Footer      string
	Rows        []DayRow
	Folded      []DayRow
	Selected    time.Time
	Today       time.Time
	TotalCount  int
	WithClass   int
}

// BuildDayCard loads a scope and builds the card data for the selected day.
// ok is false when the scope has no saved schedules at all.
func (s *Service) BuildDayCard(scopeID string, selected, now time.Time) (*DayCard, bool, error) {
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, false, err
	}
	if len(members) == 0 {
		return nil, false, nil
	}
	now = now.In(LocalTZ)
	today := beginningOfDay(now)
	selected = beginningOfDay(selected)
	allRows := DailyMemberRows(members, selected, now, nil)
	rows, folded := SplitFoldedRows(allRows)

	weekdayNames := []string{"一", "二", "三", "四", "五", "六", "日"}
	title := fmt.Sprintf("课程表 · %s 周%s", selected.Format("2006-01-02"), weekdayNames[(int(selected.Weekday())+6)%7])

	subtitle := ""
	if !selected.Equal(today) {
		withClass := 0
		for _, row := range allRows {
			if row.CourseCount > 0 {
				withClass++
			}
		}
		subtitle = fmt.Sprintf("%s · 共 %d 位成员 · %d 人有课", RelativeDayText(selected, today), len(allRows), withClass)
	}
	foldedTitle := "今天已经没有课的群友"
	if !selected.Equal(today) {
		foldedTitle = selected.Format("01-02") + " 没有课的群友"
	}
	return &DayCard{
		Title:       title,
		Subtitle:    subtitle,
		FoldedTitle: foldedTitle,
		Footer:      ScheduleFooter(selected, today),
		Rows:        rows,
		Folded:      folded,
		Selected:    selected,
		Today:       today,
		TotalCount:  len(allRows),
	}, true, nil
}

// ScopeMemberIDs returns the sorted member ids of a scope (used by tests/tools).
func (s *Service) ScopeMemberIDs(scopeID string) ([]string, error) {
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
