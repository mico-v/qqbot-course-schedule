package schedule

import (
	"testing"
	"time"
)

const wakeUpShareSample = `{"courseLen":50,"id":1,"name":"默认","sameBreakLen":false,"sameLen":false,"theBreakLen":10}
[{"endTime":"08:45","node":1,"startTime":"08:00"},{"endTime":"09:35","node":2,"startTime":"08:50"},{"endTime":"10:25","node":3,"startTime":"09:40"}]
{"background":"","maxWeek":20,"nodes":3,"school":"家里蹲大学","startDate":"2026-8-31","tableName":"26秋","sundayFirst":false}
{"courseName":"高等数学","teacher":"张三","room":"A101","day":1,"startNode":1,"step":2,"startWeek":1,"endWeek":16}
{"courseName":"大学英语","room":"B202","day":3,"startNode":3,"step":1,"startWeek":2,"endWeek":8}`

func TestParseWakeUpShareEvents(t *testing.T) {
	share, err := ParseWakeUpShare(wakeUpShareSample)
	if err != nil {
		t.Fatalf("ParseWakeUpShare: %v", err)
	}
	if len(share.Nodes) != 3 || len(share.Courses) != 2 {
		t.Fatalf("nodes=%d courses=%d, want 3/2", len(share.Nodes), len(share.Courses))
	}

	events, scheduleText, err := ParseWakeUpEventsAndSchedule(wakeUpShareSample)
	if err != nil {
		t.Fatalf("ParseWakeUpEventsAndSchedule: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if scheduleText == "" {
		t.Error("schedule text is empty")
	}

	math := events[0]
	if math["SUMMARY"] != "高等数学" || math["LOCATION"] != "A101" || math["DESCRIPTION"] != "张三" {
		t.Errorf("math event = %+v", math)
	}
	start, end, ok := EventDatetimes(math)
	if !ok {
		t.Fatal("math datetimes not parseable")
	}
	if got := start.Format("2006-01-02 15:04"); got != "2026-08-31 08:00" {
		t.Errorf("math start = %s, want 2026-08-31 08:00", got)
	}
	if got := end.Format("2006-01-02 15:04"); got != "2026-08-31 09:35" {
		t.Errorf("math end = %s, want 2026-08-31 09:35", got)
	}
	if math["RRULE"] != "FREQ=WEEKLY;COUNT=16;WKST=MO" {
		t.Errorf("math RRULE = %q", math["RRULE"])
	}

	english := events[1]
	englishStart, _, ok := EventDatetimes(english)
	if !ok {
		t.Fatal("english datetimes not parseable")
	}
	// Week 2 Wednesday: 2026-08-31 (Mon) + 7 + 2 days.
	if got := englishStart.Format("2006-01-02 15:04"); got != "2026-09-09 09:40" {
		t.Errorf("english start = %s, want 2026-09-09 09:40", got)
	}
	if english["RRULE"] != "FREQ=WEEKLY;COUNT=7;WKST=MO" {
		t.Errorf("english RRULE = %q", english["RRULE"])
	}

	// The weekly recurrence must expand to one meeting per covered week.
	bound := start.AddDate(0, 0, 16*7)
	if occurrences := ExpandEventOccurrences(math, start.AddDate(0, 0, -1), bound); len(occurrences) != 16 {
		t.Errorf("math occurrences = %d, want 16", len(occurrences))
	}
}

func TestParseWakeUpShareRejectsEmpty(t *testing.T) {
	if _, err := ParseWakeUpShare(`{"shareData":""}`); err == nil {
		t.Fatal("payload without nodes should fail")
	}
}

func TestParseWakeUpClock(t *testing.T) {
	if hour, minute, ok := parseWakeUpClock("8:05"); !ok || hour != 8 || minute != 5 {
		t.Errorf("parseWakeUpClock(8:05) = %d:%d %v", hour, minute, ok)
	}
	if _, _, ok := parseWakeUpClock("25:00"); ok {
		t.Error("parseWakeUpClock(25:00) should fail")
	}
}

func TestParseWakeUpDate(t *testing.T) {
	parsed, err := parseWakeUpDate("2026-8-31")
	if err != nil {
		t.Fatalf("parseWakeUpDate: %v", err)
	}
	if parsed.Weekday() != time.Monday {
		t.Errorf("2026-8-31 weekday = %v, want Monday", parsed.Weekday())
	}
}

// TestParseWakeUpShareBounds checks that out-of-range weeks, oversized steps,
// missing periods and malformed rows are handled without panicking or emitting
// impossible events.
func TestParseWakeUpShareBounds(t *testing.T) {
	const shareData = `{"tableName":"默认"}
[{"node":1,"startTime":"08:00","endTime":"08:45"},{"node":2,"startTime":"08:50","endTime":"09:35"},{"node":4,"startTime":"10:00","endTime":"10:45"}]
{"maxWeek":8,"nodes":3,"startDate":"2026-8-31","tableName":"26秋"}
{"courseName":"A","day":1,"startNode":1,"step":5,"startWeek":1,"endWeek":20}
{"courseName":"B","day":2,"startNode":1,"step":1,"startWeek":10,"endWeek":12}
this line is not json
{"courseName":""}`

	events, _, err := ParseWakeUpEventsAndSchedule(shareData)
	if err != nil {
		t.Fatalf("ParseWakeUpEventsAndSchedule: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (only course A)", len(events))
	}
	if events[0]["SUMMARY"] != "A" {
		t.Fatalf("SUMMARY = %v, want A", events[0]["SUMMARY"])
	}
	if events[0]["RRULE"] != "FREQ=WEEKLY;COUNT=8;WKST=MO" {
		t.Errorf("RRULE = %q, want endWeek clamped to maxWeek", events[0]["RRULE"])
	}
	_, end, ok := EventDatetimes(events[0])
	if !ok {
		t.Fatal("event datetimes not parseable")
	}
	if got := end.Format("15:04"); got != "10:45" {
		t.Errorf("end = %s, want 10:45 (last period within the step)", got)
	}
}

func TestParseWakeUpShareWithoutUsableCourses(t *testing.T) {
	const shareData = `{}
[{"node":1,"startTime":"08:00","endTime":"08:45"}]
{"maxWeek":8,"nodes":1,"startDate":"2026-8-31","tableName":"26秋"}
{"courseName":"X","day":9,"startNode":1,"step":1,"startWeek":1,"endWeek":2}`
	if _, _, err := ParseWakeUpEventsAndSchedule(shareData); err == nil {
		t.Fatal("day out of range should leave no usable courses")
	}
}
