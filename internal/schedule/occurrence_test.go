package schedule

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(layout, value, LocalTZ)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func TestExpandEventOccurrencesNonRecurring(t *testing.T) {
	events, err := ParseICSEvents(sampleICS)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	english := events[1]
	start := mustTime(t, "2006-01-02", "2026-09-03")
	end := start.AddDate(0, 0, 1)
	occurrences := ExpandEventOccurrences(english, start, end)
	if len(occurrences) != 1 {
		t.Fatalf("occurrences = %d, want 1", len(occurrences))
	}
	if got := occurrences[0].Start.Format("15:04"); got != "10:00" {
		t.Errorf("start = %s", got)
	}
}

func TestExpandEventOccurrencesWeekly(t *testing.T) {
	events, _ := ParseICSEvents(sampleICS)
	math := events[0]
	start := mustTime(t, "2006-01-02", "2026-09-01")
	end := start.AddDate(0, 1, 0) // one month window
	occurrences := ExpandEventOccurrences(math, start, end)
	if len(occurrences) != 5 {
		t.Fatalf("occurrences = %d, want 5 (Sep 1/8/15/22/29)", len(occurrences))
	}
	for _, occurrence := range occurrences {
		if occurrence.Start.Weekday() != time.Tuesday {
			t.Errorf("occurrence on %s is %s", occurrence.Start, occurrence.Start.Weekday())
		}
	}
}

func TestExpandIndexedOccurrencesHolidayAndShift(t *testing.T) {
	events, _ := ParseICSEvents(sampleICS)
	math := events[0] // weekly Tuesday
	english := events[1]
	events = []Event{math, english}

	// Holiday on Sep 8 cancels that Tuesday's math class.
	holiday := map[string]DayOverride{
		"2026-09-08": {Kind: DayOverrideHoliday},
	}
	start := mustTime(t, "2006-01-02", "2026-09-07")
	end := mustTime(t, "2006-01-02", "2026-09-09")
	occurrences := ExpandIndexedOccurrences(events, holiday, start, end)
	if len(occurrences) != 0 {
		t.Fatalf("holiday occurrences = %d, want 0", len(occurrences))
	}

	// Shift Sunday Sep 13 to use Thursday Sep 3's schedule (English class).
	shift := map[string]DayOverride{
		"2026-09-13": {Kind: DayOverrideShift, SourceDay: "2026-09-03"},
	}
	start = mustTime(t, "2006-01-02", "2026-09-13")
	end = start.AddDate(0, 0, 1)
	occurrences = ExpandIndexedOccurrences(events, shift, start, end)
	if len(occurrences) != 1 {
		t.Fatalf("shift occurrences = %d, want 1", len(occurrences))
	}
	if occurrences[0].Event["SUMMARY"] != "大学英语" {
		t.Errorf("shifted course = %q", occurrences[0].Event["SUMMARY"])
	}
	if occurrences[0].ShiftedFrom != "2026-09-03" {
		t.Errorf("shifted from = %q", occurrences[0].ShiftedFrom)
	}
	if got := occurrences[0].Start.Format("2006-01-02 15:04"); got != "2026-09-13 10:00" {
		t.Errorf("shifted start = %s", got)
	}
}

func TestExpandEventOccurrencesHonorsExdate(t *testing.T) {
	source := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:ex
SUMMARY:实验
DTSTART;TZID=Asia/Shanghai:20260901T140000
DTEND;TZID=Asia/Shanghai:20260901T160000
RRULE:FREQ=WEEKLY;BYDAY=TU
EXDATE;TZID=Asia/Shanghai:20260908T140000
END:VEVENT
END:VCALENDAR
`
	events, err := ParseICSEvents(source)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	start := mustTime(t, "2006-01-02", "2026-09-01")
	end := start.AddDate(0, 0, 15)
	occurrences := ExpandEventOccurrences(events[0], start, end)
	if len(occurrences) != 2 {
		t.Fatalf("occurrences = %d, want 2 (Sep 1 and Sep 15)", len(occurrences))
	}
	for _, occurrence := range occurrences {
		if occurrence.Start.Format("2006-01-02") == "2026-09-08" {
			t.Error("Sep 8 should be excluded by EXDATE")
		}
	}
}

func TestExpandEventOccurrencesHonorsRdate(t *testing.T) {
	source := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:rd
SUMMARY:讲座
DTSTART;TZID=Asia/Shanghai:20260901T180000
DTEND;TZID=Asia/Shanghai:20260901T190000
RDATE;TZID=Asia/Shanghai:20260903T180000
END:VEVENT
END:VCALENDAR
`
	events, _ := ParseICSEvents(source)
	start := mustTime(t, "2006-01-02", "2026-09-01")
	end := start.AddDate(0, 0, 7)
	occurrences := ExpandEventOccurrences(events[0], start, end)
	if len(occurrences) != 2 {
		t.Fatalf("occurrences = %d, want 2", len(occurrences))
	}
}
