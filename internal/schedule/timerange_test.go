package schedule

import (
	"testing"
)

func TestParseTimeRangeKeywords(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-09-17") // Thursday
	cases := []struct {
		input     string
		start     string
		end       string
		wantLabel string
	}{
		{"今日", "2026-09-17", "2026-09-17", "2026-09-17"},
		{"明天", "2026-09-18", "2026-09-18", "2026-09-18"},
		{"本周", "2026-09-14", "2026-09-20", "2026-09-14..2026-09-20"},
		{"上周", "2026-09-07", "2026-09-13", "2026-09-07..2026-09-13"},
		{"本月", "2026-09-01", "2026-09-30", "2026-09-01..2026-09-30"},
		{"上月", "2026-08-01", "2026-08-31", "2026-08-01..2026-08-31"},
		{"2026-09-01..2026-09-30", "2026-09-01", "2026-09-30", "2026-09-01..2026-09-30"},
		{"2026-09-01 .. 2026-09-03", "2026-09-01", "2026-09-03", "2026-09-01..2026-09-03"},
	}
	for _, testCase := range cases {
		parsed, err := ParseTimeRange(testCase.input, today)
		if err != nil {
			t.Errorf("ParseTimeRange(%q): %v", testCase.input, err)
			continue
		}
		if got := parsed.Start.Format("2006-01-02"); got != testCase.start {
			t.Errorf("%q start = %s, want %s", testCase.input, got, testCase.start)
		}
		if got := parsed.End.AddDate(0, 0, -1).Format("2006-01-02"); got != testCase.end {
			t.Errorf("%q end = %s, want %s", testCase.input, got, testCase.end)
		}
		if parsed.Label != testCase.wantLabel {
			t.Errorf("%q label = %s, want %s", testCase.input, parsed.Label, testCase.wantLabel)
		}
	}
}

func TestParseTimeRangeRejectsBadInput(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-09-17")
	if _, err := ParseTimeRange("下周八", today); err == nil {
		t.Fatal("invalid range should fail")
	}
	if _, err := ParseTimeRange("2026-09-30..2026-09-01", today); err == nil {
		t.Fatal("reversed range should fail")
	}
}
