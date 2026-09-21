package schedule

import (
	"testing"
	"time"
)

func memberWithEvents(t *testing.T, userID, name string, events ...Event) *Member {
	t.Helper()
	return &Member{UserID: userID, Name: name, Events: events}
}

func eventOn(t *testing.T, day, start, end, summary string) Event {
	t.Helper()
	date, err := time.ParseInLocation("2006-01-02", day, LocalTZ)
	if err != nil {
		t.Fatal(err)
	}
	event, err := MakeEvent(summary,
		date.Format("2006-01-02")+" "+start,
		date.Format("2006-01-02")+" "+end,
		"", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestDailyMemberRowsStatuses(t *testing.T) {
	day := "2026-09-17"
	now := mustTime(t, "2006-01-02 15:04", day+" 09:30")

	members := map[string]*Member{
		"active":   memberWithEvents(t, "active", "在课", eventOn(t, day, "09:00", "10:30", "高等数学")),
		"upcoming": memberWithEvents(t, "upcoming", "待课", eventOn(t, day, "11:00", "12:00", "大学英语")),
		"none":     memberWithEvents(t, "none", "无课"),
		"holiday": {
			UserID:       "holiday",
			Name:         "休假",
			Events:       []Event{eventOn(t, day, "14:00", "16:00", "实验")},
			DayOverrides: map[string]DayOverride{day: {Kind: DayOverrideHoliday}},
		},
	}

	rows := DailyMemberRows(members, mustTime(t, "2006-01-02", day), now, nil)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	byID := make(map[string]DayRow, len(rows))
	for _, row := range rows {
		byID[row.UserID] = row
	}
	if byID["active"].StatusKey != "active" || byID["active"].CountdownLabel != "距下课" {
		t.Errorf("active row = %+v", byID["active"])
	}
	if byID["upcoming"].StatusKey != "upcoming" {
		t.Errorf("upcoming row = %+v", byID["upcoming"])
	}
	if byID["none"].StatusKey != "none" || byID["none"].Course != "暂无课程安排" {
		t.Errorf("none row = %+v", byID["none"])
	}
	if byID["holiday"].StatusKey != "holiday" || byID["holiday"].Course != "休假 · 无课程安排" {
		t.Errorf("holiday row = %+v", byID["holiday"])
	}
	if rows[0].UserID != "active" {
		t.Errorf("first row = %s, want active", rows[0].UserID)
	}
}

func TestDailyMemberRowsFinishedAndFolded(t *testing.T) {
	day := "2026-09-17"
	now := mustTime(t, "2006-01-02 15:04", day+" 20:00")
	members := map[string]*Member{
		"done": memberWithEvents(t, "done", "已下课", eventOn(t, day, "09:00", "10:30", "高数")),
		"none": memberWithEvents(t, "none", "无课"),
	}
	rows := DailyMemberRows(members, mustTime(t, "2006-01-02", day), now, nil)
	if rows[0].StatusKey != "finished" {
		t.Fatalf("status = %s, want finished", rows[0].StatusKey)
	}
	shown, folded := SplitFoldedRows(rows)
	if len(shown) != 0 || len(folded) != 2 {
		t.Errorf("shown=%d folded=%d, want 0/2", len(shown), len(folded))
	}
}

func TestMergeIntervals(t *testing.T) {
	base := mustTime(t, "2006-01-02 15:04", "2026-09-17 08:00")
	intervals := [][2]time.Time{
		{base, base.Add(time.Hour)},
		{base.Add(30 * time.Minute), base.Add(2 * time.Hour)},
		{base.Add(3 * time.Hour), base.Add(4 * time.Hour)},
	}
	merged := MergeIntervals(intervals)
	if len(merged) != 2 {
		t.Fatalf("merged = %d, want 2", len(merged))
	}
	if merged[0][1].Sub(merged[0][0]) != 2*time.Hour {
		t.Errorf("first merged span = %v", merged[0][1].Sub(merged[0][0]))
	}
}

func TestFormatDurationAndRemaining(t *testing.T) {
	if got := FormatDurationMinutes(90); got != "1小时30分钟" {
		t.Errorf("90 = %q", got)
	}
	if got := FormatDurationMinutes(120); got != "2小时" {
		t.Errorf("120 = %q", got)
	}
	if got := FormatDurationMinutes(30); got != "30分钟" {
		t.Errorf("30 = %q", got)
	}
	if got := FormatRemaining(30 * time.Second); got != "1分钟" {
		t.Errorf("30s = %q", got)
	}
	if got := FormatRemaining(2*time.Hour + 15*time.Minute); got != "2小时15分钟" {
		t.Errorf("2h15m = %q", got)
	}
}
