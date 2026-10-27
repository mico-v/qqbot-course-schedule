package schedule

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Service is the application layer every entry point funnels through.
type Service struct {
	members   GroupMemberRepository
	schedules GroupScheduleRepository
	overrides DayOverrideRepository
	settings  SettingsStore
	editLinks EditLinkStore
	stats     StatsRepository
}

// NewService wires the service to its storage.
func NewService(store ServiceStorage) *Service {
	return &Service{
		members: store, schedules: store, overrides: store,
		settings: store, editLinks: store, stats: store,
	}
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
	return s.SaveICSWithSource(scopeID, userID, name, content, sourceFile, actor, ImportSource{})
}

// SaveICSWithSource saves an ICS document together with its import origin.
func (s *Service) SaveICSWithSource(scopeID, userID, name, content, sourceFile, actor string, source ImportSource) (*SaveResult, error) {
	return s.SaveICSWithSourceContext(
		context.Background(), scopeID, userID, name, content, sourceFile, actor, source,
	)
}

// SaveICSWithSourceContext is the cancellable ICS import path.
func (s *Service) SaveICSWithSourceContext(ctx context.Context, scopeID, userID, name, content, sourceFile, actor string, source ImportSource) (*SaveResult, error) {
	text := strings.TrimSpace(content)
	if text == "" {
		return nil, ValidationErrorf("ICS 内容为空，未保存。")
	}
	if len([]byte(text)) > MaxICSBytes {
		return nil, ValidationErrorf("ICS 文件超过 %d MiB，未保存。", MaxICSBytes>>20)
	}
	events, scheduleText, err := ParseICSEventsAndSchedule(text)
	if err != nil {
		return nil, ValidationErrorf("ICS 解析失败，未保存：%v", err)
	}
	if len(events) == 0 {
		return nil, ValidationErrorf("ICS 中没有 VEVENT，未保存。")
	}
	return s.saveEventsContext(ctx, scopeID, userID, name, events, text, scheduleText, "ics", sourceFile, actor, source)
}

// SaveWakeUpShare parses a decoded WakeUp share payload and replaces a
// member's whole schedule.
func (s *Service) SaveWakeUpShare(scopeID, userID, name, shareData, sourceFile, actor string) (*SaveResult, error) {
	return s.SaveWakeUpShareWithSource(scopeID, userID, name, shareData, sourceFile, actor, ImportSource{})
}

// SaveWakeUpShareWithSource saves a WakeUp schedule together with its import
// origin.
func (s *Service) SaveWakeUpShareWithSource(scopeID, userID, name, shareData, sourceFile, actor string, source ImportSource) (*SaveResult, error) {
	return s.SaveWakeUpShareWithSourceContext(
		context.Background(), scopeID, userID, name, shareData, sourceFile, actor, source,
	)
}

// SaveWakeUpShareWithSourceContext is the cancellable WakeUp import path.
func (s *Service) SaveWakeUpShareWithSourceContext(ctx context.Context, scopeID, userID, name, shareData, sourceFile, actor string, source ImportSource) (*SaveResult, error) {
	events, scheduleText, err := ParseWakeUpEventsAndSchedule(shareData)
	if err != nil {
		return nil, ValidationErrorf("WakeUp 分享解析失败，未保存：%v", err)
	}
	ics := SerializeScheduleICS(events, "", firstNonEmpty(name, "WakeUp课表"))
	return s.saveEventsContext(ctx, scopeID, userID, name, events, ics, scheduleText, "wakeup", sourceFile, actor, source)
}

func (s *Service) saveEvents(scopeID, userID, name string, events []Event, icsText, scheduleText, source, sourceFile, actor string, importSource ImportSource) (*SaveResult, error) {
	return s.saveEventsContext(
		context.Background(), scopeID, userID, name, events, icsText,
		scheduleText, source, sourceFile, actor, importSource,
	)
}

func (s *Service) saveEventsContext(ctx context.Context, scopeID, userID, name string, events []Event, icsText, scheduleText, source, sourceFile, actor string, importSource ImportSource) (*SaveResult, error) {
	previous, found, err := s.getMemberContext(ctx, scopeID, userID)
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
	updated.ICS = icsText
	updated.Schedule = scheduleText
	updated.Source = source
	updated.SourceFile = sourceFile
	updated.EventCount = len(events)
	updated.UpdatedAt = now
	updated.ScheduleUpdatedAt = now
	updated.LastModifiedAt = now
	updated.LastModifiedBy = actor
	if importSource != (ImportSource{}) {
		updated.ImportGroupOpenID = strings.TrimSpace(importSource.GroupOpenID)
		updated.ImportUserOpenID = strings.TrimSpace(importSource.UserOpenID)
		updated.ImportUnionOpenID = strings.TrimSpace(importSource.UnionOpenID)
	}

	expected := int64(0)
	if found {
		expected = previous.Revision
	}
	if err := s.putMemberContext(ctx, scopeID, userID, &updated, expected); err != nil {
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

// EnsureMember creates an empty schedule for a member if none exists yet. An
// existing member is left untouched so a self-chosen /nickname is never
// overwritten by the platform-provided name.
func (s *Service) EnsureMember(scopeID, userID, name string) error {
	return s.EnsureMemberContext(context.Background(), scopeID, userID, name)
}

// EnsureMemberContext creates an empty member schedule when absent.
func (s *Service) EnsureMemberContext(ctx context.Context, scopeID, userID, name string) error {
	member, found, err := s.getMemberContext(ctx, scopeID, userID)
	if err != nil {
		return err
	}
	if found && member != nil {
		return nil
	}
	created := &Member{
		UserID:     userID,
		Name:       firstNonEmpty(name, userID),
		Events:     nil,
		Source:     "manual",
		UpdatedAt:  NowISO(),
		EventCount: 0,
	}
	zero := int64(0)
	if err := s.putMemberContext(ctx, scopeID, userID, created, zero); err != nil && !errors.Is(err, ErrConflict) {
		return err
	}
	return nil
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
// at, when non-zero, is the reference instant used for statuses and countdowns
// (for example the time parsed from "/课表 10-10 8:00").
// ok is false when the scope has no saved schedules at all.
func (s *Service) BuildDayCard(scopeID string, selected, now, at time.Time) (*DayCard, bool, error) {
	return s.BuildDayCardContext(context.Background(), scopeID, selected, now, at)
}

// BuildDayCardContext loads and builds a day card with cancellation.
func (s *Service) BuildDayCardContext(ctx context.Context, scopeID string, selected, now, at time.Time) (*DayCard, bool, error) {
	members, err := s.listMembersContext(ctx, scopeID)
	if err != nil {
		return nil, false, err
	}
	return s.BuildDayCardForMembers(members, selected, now, at)
}

// BuildDayCardForMembers builds one day view from an already resolved member
// set, allowing callers to project global schedules onto local identities.
func (s *Service) BuildDayCardForMembers(members map[string]*Member, selected, now, at time.Time) (*DayCard, bool, error) {
	if len(members) == 0 {
		return nil, false, nil
	}
	now = now.In(LocalTZ)
	today := beginningOfDay(now)
	selected = beginningOfDay(selected)
	allRows := DailyMemberRows(members, selected, now, at, nil)
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
		Footer:      ScheduleFooterAt(selected, today, at),
		Rows:        rows,
		Folded:      folded,
		Selected:    selected,
		Today:       today,
		TotalCount:  len(allRows),
	}, true, nil
}

// BuildDayMarkdown renders the same day view as BuildDayCard as a markdown
// list of every member's courses. ok is false when the scope has no saved
// schedules at all.
func (s *Service) BuildDayMarkdown(scopeID string, selected, now time.Time) (string, bool, error) {
	return s.BuildDayMarkdownContext(context.Background(), scopeID, selected, now)
}

// BuildDayMarkdownContext loads markdown day data with cancellation.
func (s *Service) BuildDayMarkdownContext(ctx context.Context, scopeID string, selected, now time.Time) (string, bool, error) {
	members, err := s.listMembersContext(ctx, scopeID)
	if err != nil {
		return "", false, err
	}
	return s.BuildDayMarkdownForMembers(members, selected, now)
}

// BuildDayMarkdownForMembers renders a resolved member set as markdown.
func (s *Service) BuildDayMarkdownForMembers(members map[string]*Member, selected, now time.Time) (string, bool, error) {
	if len(members) == 0 {
		return "", false, nil
	}
	return MarkdownDayList(members, selected, now), true, nil
}

// ScopeMembers returns every member of a scope with overrides attached.
func (s *Service) ScopeMembers(scopeID string) (map[string]*Member, error) {
	return s.ScopeMembersContext(context.Background(), scopeID)
}

// ScopeMembersContext loads one group's schedules with cancellation.
func (s *Service) ScopeMembersContext(ctx context.Context, scopeID string) (map[string]*Member, error) {
	return s.listMembersContext(ctx, scopeID)
}

// ScopeMemberIDs returns the sorted member ids of a scope (used by tests/tools).
func (s *Service) ScopeMemberIDs(scopeID string) ([]string, error) {
	return s.ScopeMemberIDsContext(context.Background(), scopeID)
}

// ScopeMemberIDsContext loads sorted member IDs with cancellation.
func (s *Service) ScopeMemberIDsContext(ctx context.Context, scopeID string) ([]string, error) {
	members, err := s.listMembersContext(ctx, scopeID)
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
