package schedule

import (
	"strings"
	"testing"
)

// WakeUpSchedule-style export: every VEVENT nests a VALARM.
const wakeUpICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//YZune//WakeUpSchedule//EN
BEGIN:VEVENT
DTSTAMP:20260916T022436Z
UID:WakeUpSchedule-4c739d44
SUMMARY:数学课程
DTSTART;TZID=/Asia/Shanghai:20260902T080000
DTEND;TZID=/Asia/Shanghai:20260902T094000
RRULE:FREQ=WEEKLY;UNTIL=20261222T160000Z;INTERVAL=1
LOCATION:教九楼C4-207 南侧
DESCRIPTION:第1 - 2节\n教九楼C4-207\n南侧
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER;RELATED=START:-PT20M
DESCRIPTION:数学课程@教九楼C4-207
END:VALARM
END:VEVENT
BEGIN:VEVENT
DTSTAMP:20260916T022436Z
UID:WakeUpSchedule-07a9d86c
SUMMARY:C/C++程序设计
DTSTART;TZID=/Asia/Shanghai:20260902T095500
DTEND;TZID=/Asia/Shanghai:20260902T113500
RRULE:FREQ=WEEKLY;UNTIL=20261027T160000Z;INTERVAL=1
BEGIN:VALARM
ACTION:DISPLAY
TRIGGER;RELATED=START:-PT20M
DESCRIPTION:C/C++程序设计@教九楼C6-208
END:VALARM
END:VEVENT
END:VCALENDAR
`

func TestParseICSEventsWithNestedAlarm(t *testing.T) {
	events, err := ParseICSEvents(wakeUpICS)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	first := events[0]
	if first["SUMMARY"] != "数学课程" {
		t.Errorf("SUMMARY = %q", first["SUMMARY"])
	}
	// The VALARM's DESCRIPTION must not shadow the event's own description.
	if !strings.HasPrefix(first["DESCRIPTION"], "第1 - 2节") {
		t.Errorf("DESCRIPTION = %q", first["DESCRIPTION"])
	}
	if !strings.Contains(first["RAW_ICAL"], "BEGIN:VALARM") {
		t.Errorf("RAW_ICAL lost the nested alarm: %q", first["RAW_ICAL"])
	}
	start, ok := ParseICSTime(first["DTSTART"], first["DTSTART_TZID"])
	if !ok || start.Format("2006-01-02 15:04") != "2026-09-02 08:00" {
		t.Errorf("DTSTART parse = %v %v", start, ok)
	}
}

func TestSerializeKeepsNestedAlarm(t *testing.T) {
	events, err := ParseICSEvents(wakeUpICS)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	serialized := SerializeScheduleICS(events, "", "测试")
	if !strings.Contains(serialized, "BEGIN:VALARM") || !strings.Contains(serialized, "TRIGGER;RELATED=START:-PT20M") {
		t.Errorf("serialized ICS lost the alarm:\n%s", serialized)
	}
	reparsed, err := ParseICSEvents(serialized)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(reparsed) != 2 || reparsed[0]["SUMMARY"] != "数学课程" {
		t.Errorf("reparsed = %+v", reparsed)
	}
}

func TestExpandWakeUpScheduleOccurrences(t *testing.T) {
	events, err := ParseICSEvents(wakeUpICS)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	// 2026-09-02 is a Wednesday; the weekly class should expand inside a month.
	start := mustTime(t, "2006-01-02", "2026-09-01")
	end := mustTime(t, "2006-01-02", "2026-10-01")
	occurrences := ExpandEventOccurrences(events[0], start, end)
	if len(occurrences) != 5 {
		t.Fatalf("occurrences = %d, want 5", len(occurrences))
	}
}
