package schedule

import (
	"strings"
	"testing"
	"time"
)

const sampleICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//test//CN
BEGIN:VEVENT
UID:math-1
SUMMARY:高等数学
DTSTART;TZID=Asia/Shanghai:20260901T080000
DTEND;TZID=Asia/Shanghai:20260901T093000
LOCATION:教一101
DESCRIPTION:带教材
RRULE:FREQ=WEEKLY;BYDAY=TU
END:VEVENT
BEGIN:VEVENT
UID:eng-1
SUMMARY:大学英语
DTSTART;TZID=Asia/Shanghai:20260903T100000
DTEND;TZID=Asia/Shanghai:20260903T113000
END:VEVENT
END:VCALENDAR
`

func TestParseICSEvents(t *testing.T) {
	events, err := ParseICSEvents(sampleICS)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	first := events[0]
	if first["SUMMARY"] != "高等数学" {
		t.Errorf("SUMMARY = %q", first["SUMMARY"])
	}
	if first["DTSTART"] != "20260901T080000" || first["DTSTART_TZID"] != "Asia/Shanghai" {
		t.Errorf("DTSTART = %q tzid = %q", first["DTSTART"], first["DTSTART_TZID"])
	}
	if first["LOCATION"] != "教一101" {
		t.Errorf("LOCATION = %q", first["LOCATION"])
	}
	if !strings.Contains(first["RAW_ICAL"], "BEGIN:VEVENT") {
		t.Errorf("RAW_ICAL missing component: %q", first["RAW_ICAL"])
	}
	if events[1]["SUMMARY"] != "大学英语" {
		t.Errorf("second SUMMARY = %q", events[1]["SUMMARY"])
	}
}

func TestParseICSEventsRejectsGarbage(t *testing.T) {
	if _, err := ParseICSEvents("hello world"); err == nil {
		t.Fatal("garbage should fail")
	}
}

func TestMakeEventValidation(t *testing.T) {
	if _, err := MakeEvent("数学", "2026-09-01 09:00", "2026-09-01 08:00", "", "", "", ""); err == nil {
		t.Fatal("end before start should fail")
	}
	if _, err := MakeEvent("数学", "2026-09-01 08:00", "2026-09-01 09:00", "", "", "BYDAY=MO", ""); err == nil {
		t.Fatal("rrule without FREQ should fail")
	}
	event, err := MakeEvent("数学", "2026-09-01 08:00", "2026-09-01 09:30", "教一", "备注", "FREQ=WEEKLY;BYDAY=TU", "uid-1")
	if err != nil {
		t.Fatalf("MakeEvent: %v", err)
	}
	if event["DTSTART"] != "20260901T080000" || event["DTEND"] != "20260901T093000" {
		t.Errorf("times = %q..%q", event["DTSTART"], event["DTEND"])
	}
	if event["UID"] != "uid-1" || event["SUMMARY"] != "数学" || event["LOCATION"] != "教一" {
		t.Errorf("event = %+v", event)
	}
}

func TestNormalizeDateTime(t *testing.T) {
	cases := map[string]string{
		"2026-09-01 08:00": "20260901T080000",
		"2026/09/01 08:00": "20260901T080000",
		"2026-09-01T08:00": "20260901T080000",
		"20260901T080000":  "20260901T080000",
	}
	for input, want := range cases {
		got, err := NormalizeDateTime(input)
		if err != nil || got != want {
			t.Errorf("NormalizeDateTime(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := NormalizeDateTime("not a time"); err == nil {
		t.Fatal("invalid time should fail")
	}
}

func TestSerializeScheduleICSPreservesUnknownProperties(t *testing.T) {
	source := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:u1
SUMMARY:体育
DTSTART;TZID=Asia/Shanghai:20260901T080000
DTEND;TZID=Asia/Shanghai:20260901T093000
RDATE;TZID=Asia/Shanghai:20260908T080000
EXDATE;TZID=Asia/Shanghai:20260915T080000
X-CUSTOM:keep-me
END:VEVENT
END:VCALENDAR
`
	events, err := ParseICSEvents(source)
	if err != nil {
		t.Fatalf("ParseICSEvents: %v", err)
	}
	serialized := SerializeScheduleICS(events, "", "测试课表")
	for _, needle := range []string{"RDATE;TZID=Asia/Shanghai:20260908T080000", "EXDATE;TZID=Asia/Shanghai:20260915T080000", "X-CUSTOM:keep-me", "X-WR-CALNAME:测试课表"} {
		if !strings.Contains(serialized, needle) {
			t.Errorf("serialized ICS missing %q:\n%s", needle, serialized)
		}
	}
	reparsed, err := ParseICSEvents(serialized)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(reparsed) != 1 || reparsed[0]["SUMMARY"] != "体育" {
		t.Errorf("reparsed = %+v", reparsed)
	}
}

func TestFormatICSSchedule(t *testing.T) {
	events, _ := ParseICSEvents(sampleICS)
	text := FormatICSSchedule(events)
	if !strings.Contains(text, "1. 高等数学") || !strings.Contains(text, "每周 TU") {
		t.Errorf("schedule text = %q", text)
	}
	if !strings.Contains(text, "2026-09-01 08:00") {
		t.Errorf("schedule text missing time: %q", text)
	}
}

func TestParseICSTimeFormats(t *testing.T) {
	start, ok := ParseICSTime("20260901T080000", "Asia/Shanghai")
	if !ok || start.Format("2006-01-02 15:04") != "2026-09-01 08:00" {
		t.Errorf("parse failed: %v %v", start, ok)
	}
	dateOnly, ok := ParseICSTime("20260901", "")
	if !ok || dateOnly.Hour() != 0 {
		t.Errorf("date-only parse failed: %v", dateOnly)
	}
	utc, ok := ParseICSTime("20260901T000000Z", "")
	if !ok || utc.In(LocalTZ).Format("15:04") != "08:00" {
		t.Errorf("UTC parse failed: %v", utc.In(LocalTZ))
	}
}

func TestEventDatetimesFallback(t *testing.T) {
	event := Event{"DTSTART": "20260901T080000"}
	start, end, ok := EventDatetimes(event)
	if !ok {
		t.Fatal("EventDatetimes failed")
	}
	if end.Sub(start) != 90*time.Minute {
		t.Errorf("duration = %v, want 1h30m", end.Sub(start))
	}
	allDay := Event{"DTSTART": "20260901"}
	start, end, ok = EventDatetimes(allDay)
	if !ok || end.Sub(start) != 24*time.Hour {
		t.Errorf("all-day duration = %v, ok=%v", end.Sub(start), ok)
	}
}
