package schedule

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestRankUnionMetricCountsOverlapOnce(t *testing.T) {
	day := "2026-09-17"
	members := map[string]*Member{
		"A": memberWithEvents(t, "A", "小明",
			eventOn(t, day, "09:00", "10:00", "高数"),
			eventOn(t, day, "09:30", "10:30", "英语"),
		),
	}
	start := mustTime(t, "2006-01-02", day)
	end := start.AddDate(0, 0, 1)
	rows := BuildRankRows(members, start, end, mustTime(t, "2006-01-02 15:04", day+" 20:00"))
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Minutes != 90 {
		t.Errorf("minutes = %d, want 90 (union)", rows[0].Minutes)
	}
	if rows[0].HoursText != "1小时30分钟" {
		t.Errorf("hours text = %q", rows[0].HoursText)
	}
	if rows[0].CourseCount != 2 || rows[0].CourseNames != 2 {
		t.Errorf("courses = %d/%d", rows[0].CourseCount, rows[0].CourseNames)
	}
}

func TestRankClipsCrossDayEventToWindow(t *testing.T) {
	members := map[string]*Member{
		"A": memberWithEvents(t, "A", "小明", eventOn(t, "2026-09-17", "23:00", "23:59", "晚课")),
	}
	start := mustTime(t, "2006-01-02", "2026-09-17")
	end := start.AddDate(0, 0, 1)
	rows := BuildRankRows(members, start, end, mustTime(t, "2006-01-02 15:04", "2026-09-18 10:00"))
	if len(rows) != 1 || rows[0].Minutes != 59 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRankIgnoresAllDayEvents(t *testing.T) {
	allDay := Event{"DTSTART": "20260917", "DTEND": "20260918", "SUMMARY": "放假"}
	members := map[string]*Member{
		"A": memberWithEvents(t, "A", "小明", allDay),
	}
	start := mustTime(t, "2006-01-02", "2026-09-17")
	end := start.AddDate(0, 0, 1)
	rows := BuildRankRows(members, start, end, mustTime(t, "2006-01-02 15:04", "2026-09-17 20:00"))
	if len(rows) != 0 {
		t.Fatalf("all-day event must not rank: %+v", rows)
	}
}

func TestRankElapsedMinutes(t *testing.T) {
	day := "2026-09-17"
	members := map[string]*Member{
		"A": memberWithEvents(t, "A", "小明",
			eventOn(t, day, "09:00", "10:00", "已上"),
			eventOn(t, day, "13:00", "14:00", "未上"),
		),
	}
	start := mustTime(t, "2006-01-02", day)
	end := start.AddDate(0, 0, 1)
	rows := BuildRankRows(members, start, end, mustTime(t, "2006-01-02 15:04", day+" 12:00"))
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Minutes != 120 || rows[0].ElapsedMinutes != 60 {
		t.Errorf("minutes/elapsed = %d/%d, want 120/60", rows[0].Minutes, rows[0].ElapsedMinutes)
	}
	if rows[0].ElapsedText != "1小时" {
		t.Errorf("elapsed text = %q", rows[0].ElapsedText)
	}
}

func TestRankOrderAndSharedRank(t *testing.T) {
	day := "2026-09-17"
	members := map[string]*Member{
		"A": memberWithEvents(t, "A", "甲", eventOn(t, day, "08:00", "10:00", "两小时")),
		"B": memberWithEvents(t, "B", "乙", eventOn(t, day, "08:00", "10:00", "两小时")),
		"C": memberWithEvents(t, "C", "丙", eventOn(t, day, "08:00", "09:00", "一小时")),
	}
	start := mustTime(t, "2006-01-02", day)
	end := start.AddDate(0, 0, 1)
	rows := BuildRankRows(members, start, end, mustTime(t, "2006-01-02 15:04", day+" 20:00"))
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Rank != 1 || rows[1].Rank != 1 {
		t.Errorf("top two should share rank 1: %d %d", rows[0].Rank, rows[1].Rank)
	}
	if rows[2].Rank != 3 {
		t.Errorf("third rank = %d, want 3", rows[2].Rank)
	}
	if rows[2].Progress != 0.5 {
		t.Errorf("third progress = %v, want 0.5", rows[2].Progress)
	}
}

func TestRankBoardRowsPeriodAndLimit(t *testing.T) {
	store := newMemoryStorage()
	service := NewService(store)
	now := mustTime(t, "2006-01-02 15:04", "2026-09-17 12:00")
	day := "2026-09-17"
	store.members["group:G1"] = map[string]*Member{
		"A": memberWithEvents(t, "A", "小明", eventOn(t, day, "09:00", "10:30", "高数")),
	}
	rows, label, err := service.RankBoardRows("group:G1", "", now)
	if err != nil {
		t.Fatalf("RankBoardRows: %v", err)
	}
	if label != "2026-09-14..2026-09-20" || len(rows) != 1 || rows[0].Minutes != 90 {
		t.Fatalf("rows=%+v label=%q", rows, label)
	}
	if _, _, err := service.RankBoardRows("group:G1", "2020-01-01..2026-09-17", now); err == nil {
		t.Fatal("range over 366 days should fail")
	}
	if _, _, err := service.RankBoardRows("group:G1", "下周八", now); err == nil || !strings.Contains(err.Error(), "无法识别时间范围") {
		t.Fatalf("bad period err = %v", err)
	}
}

// memoryStorage is a tiny in-memory schedule.Storage for service tests.
type memoryStorage struct {
	members   map[string]map[string]*Member
	overrides map[string][]DayOverrideRow
	kv        map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{
		members:   make(map[string]map[string]*Member),
		overrides: make(map[string][]DayOverrideRow),
		kv:        make(map[string][]byte),
	}
}

func (m *memoryStorage) GetMember(scopeID, userID string) (*Member, bool, error) {
	member, ok := m.members[scopeID][userID]
	return member, ok, nil
}

func (m *memoryStorage) GetScopeMembers(scopeID string) (map[string]*Member, error) {
	result := make(map[string]*Member)
	for id, member := range m.members[scopeID] {
		result[id] = member
	}
	return result, nil
}

func (m *memoryStorage) PutMember(scopeID, userID string, member *Member, expectedRevision *int64) error {
	if m.members[scopeID] == nil {
		m.members[scopeID] = make(map[string]*Member)
	}
	if expectedRevision != nil && *expectedRevision != 0 {
		existing, ok := m.members[scopeID][userID]
		if !ok || existing.Revision != *expectedRevision {
			return ErrConflict
		}
	}
	copied := *member
	copied.Revision++
	m.members[scopeID][userID] = &copied
	return nil
}

func (m *memoryStorage) ListScopeSummaries() ([]ScopeSummary, error) {
	var summaries []ScopeSummary
	for scopeID, members := range m.members {
		summary := ScopeSummary{ScopeID: scopeID}
		for userID, member := range members {
			summary.Members = append(summary.Members, ScopeMemberSummary{
				UserID: userID, Name: member.Name, EventCount: len(member.Events), Revision: member.Revision,
			})
		}
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ScopeID < summaries[j].ScopeID })
	return summaries, nil
}

func (m *memoryStorage) ListDayOverrides(scopeID string) ([]DayOverrideRow, error) {
	return append([]DayOverrideRow(nil), m.overrides[scopeID]...), nil
}

func (m *memoryStorage) SetDayOverride(scopeID, userID, day string, override DayOverride, createdBy, createdAt string) error {
	rows := m.overrides[scopeID]
	for index, row := range rows {
		if row.UserID == userID && row.Day == day {
			rows[index].Kind = override.Kind
			rows[index].SourceDay = override.SourceDay
			m.overrides[scopeID] = rows
			return nil
		}
	}
	m.overrides[scopeID] = append(rows, DayOverrideRow{
		ScopeID: scopeID, UserID: userID, Day: day, Kind: override.Kind,
		SourceDay: override.SourceDay, CreatedBy: createdBy, CreatedAt: createdAt,
	})
	return nil
}

func (m *memoryStorage) DeleteDayOverride(scopeID, userID, day string) (bool, error) {
	rows := m.overrides[scopeID]
	for index, row := range rows {
		if row.UserID == userID && row.Day == day {
			m.overrides[scopeID] = append(rows[:index], rows[index+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (m *memoryStorage) GetKV(scope, namespace, key string, out any) (bool, error) {
	_, ok := m.kv[scope+"/"+namespace+"/"+key]
	return ok, nil
}

func (m *memoryStorage) ListKV(scope, namespace string) ([]KVEntry, error) {
	prefix := scope + "/" + namespace + "/"
	var entries []KVEntry
	for key, value := range m.kv {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, KVEntry{Key: strings.TrimPrefix(key, prefix), Value: value})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}

func (m *memoryStorage) SetKV(scope, namespace, key string, value any) error {
	m.kv[scope+"/"+namespace+"/"+key] = []byte{}
	return nil
}

func (m *memoryStorage) DeleteKV(scope, namespace, key string) error {
	delete(m.kv, scope+"/"+namespace+"/"+key)
	return nil
}

var _ Storage = (*memoryStorage)(nil)

func TestClippedOccurrencesKeywordFilter(t *testing.T) {
	day := "2026-09-17"
	member := memberWithEvents(t, "A", "小明",
		eventOn(t, day, "09:00", "10:00", "高数"),
		eventOn(t, day, "10:00", "11:00", "自习"),
	)
	start := mustTime(t, "2006-01-02", day)
	end := start.AddDate(0, 0, 1)
	occurrences := ExpandMemberOccurrences(member, start, end)
	clipped := ClippedOccurrences(occurrences, start, end, false, []string{"自习"})
	if len(clipped) != 1 || clipped[0].Event["SUMMARY"] != "高数" {
		t.Fatalf("clipped = %+v", clipped)
	}
	_ = time.Now
}
