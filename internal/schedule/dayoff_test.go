package schedule

import (
	"testing"
	"time"
)

func TestParseDayTokenRelativeAndAbsolute(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-09-17")
	cases := map[string]string{
		"今天":         "2026-09-17",
		"明天":         "2026-09-18",
		"昨天":         "2026-09-16",
		"后天":         "2026-09-19",
		"2026-09-01": "2026-09-01",
		"2026/9/1":   "2026-09-01",
		"2026年9月1日":  "2026-09-01",
		"9.1":        "2026-09-01",
		"9月1日":       "2026-09-01",
	}
	for input, want := range cases {
		day, found, err := ParseDayToken(input, today)
		if err != nil || !found {
			t.Errorf("ParseDayToken(%q) found=%v err=%v", input, found, err)
			continue
		}
		if got := day.Format("2006-01-02"); got != want {
			t.Errorf("ParseDayToken(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestParseDayTokenResolvesWithinAcademicYear(t *testing.T) {
	// Aug 1 is the academic-year boundary, so a year-less date lands in the
	// academic year that contains today.
	sept := mustTime(t, "2006-01-02", "2026-09-17")
	for input, want := range map[string]string{
		"1.3":  "2027-01-03",
		"9.1":  "2026-09-01",
		"8.1":  "2026-08-01",
		"7.31": "2027-07-31",
	} {
		day, _, err := ParseDayToken(input, sept)
		if err != nil {
			t.Fatalf("ParseDayToken(%q): %v", input, err)
		}
		if got := day.Format("2006-01-02"); got != want {
			t.Errorf("ParseDayToken(%q) = %s, want %s", input, got, want)
		}
	}

	// In spring the same semester still covers the previous autumn.
	spring := mustTime(t, "2006-01-02", "2027-03-15")
	for input, want := range map[string]string{
		"10-07": "2026-10-07",
		"9.1":   "2026-09-01",
		"3.20":  "2027-03-20",
	} {
		day, _, err := ParseDayToken(input, spring)
		if err != nil {
			t.Fatalf("ParseDayToken(%q): %v", input, err)
		}
		if got := day.Format("2006-01-02"); got != want {
			t.Errorf("ParseDayToken(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestParseDayTokenRejectsInvalidDate(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-09-17")
	if _, _, err := ParseDayToken("2月30日", today); err == nil {
		t.Fatal("2月30日 should fail")
	}
}

func TestAcademicYearHelpers(t *testing.T) {
	aug := mustTime(t, "2006-01-02", "2026-08-01")
	jul := mustTime(t, "2006-01-02", "2027-07-31")
	if !SameAcademicYear(aug, jul) {
		t.Fatal("2026-08-01 and 2027-07-31 should share an academic year")
	}
	if SameAcademicYear(jul, mustTime(t, "2006-01-02", "2027-08-01")) {
		t.Fatal("2027-08-01 should start a new academic year")
	}
	if got := AcademicYearLabel(aug); got != "2026-2027" {
		t.Errorf("label = %s", got)
	}
}

func TestSplitDayOverrideArgs(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-09-17")
	days, rest, err := SplitDayOverrideArgs("2026-10-01 @小明", today)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(days) != 1 || days[0].Format("2006-01-02") != "2026-10-01" {
		t.Errorf("days = %v", days)
	}
	if len(rest) != 1 || rest[0] != "小明" {
		t.Errorf("rest = %v", rest)
	}

	days, _, err = SplitDayOverrideArgs("10月1日至10月8日", today)
	if err != nil {
		t.Fatalf("range: %v", err)
	}
	if len(days) != 8 {
		t.Errorf("range days = %d, want 8", len(days))
	}

	days, _, err = SplitDayOverrideArgs("10月11日 上 10月8日 的课", today)
	if err != nil {
		t.Fatalf("particles: %v", err)
	}
	if len(days) != 2 {
		t.Errorf("particle days = %v", days)
	}
}

func TestSplitDayOverrideArgsResolvesShiftInSemester(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-10-08")
	days, rest, err := SplitDayOverrideArgs("10-10 10-7", today)
	if err != nil {
		t.Fatalf("shift args: %v", err)
	}
	if len(days) != 2 || len(rest) != 0 {
		t.Fatalf("days = %v rest = %v", days, rest)
	}
	if got := days[0].Format("2006-01-02"); got != "2026-10-10" {
		t.Errorf("target = %s, want 2026-10-10", got)
	}
	if got := days[1].Format("2006-01-02"); got != "2026-10-07" {
		t.Errorf("source = %s, want 2026-10-07", got)
	}
}

func TestSingleDayQuery(t *testing.T) {
	today := mustTime(t, "2006-01-02", "2026-09-17")
	day, message := SingleDayQuery("", today)
	if day != nil || message != "" {
		t.Errorf("empty query = %v %q", day, message)
	}
	day, message = SingleDayQuery("明天", today)
	if day == nil || message != "" || day.Format("2006-01-02") != "2026-09-18" {
		t.Errorf("明天 = %v %q", day, message)
	}
	day, message = SingleDayQuery("9.17..9.19", today)
	if day != nil || message == "" {
		t.Errorf("range should be rejected, got %v %q", day, message)
	}
	day, message = SingleDayQuery("abc", today)
	if day != nil || message == "" {
		t.Errorf("garbage should be rejected, got %v %q", day, message)
	}
}

func TestFormatHelpers(t *testing.T) {
	days := []time.Time{
		mustTime(t, "2006-01-02", "2026-10-01"),
		mustTime(t, "2006-01-02", "2026-10-02"),
	}
	if got := FormatDayList(days); got != "2026-10-01 至 2026-10-02" {
		t.Errorf("FormatDayList = %q", got)
	}
	if got := DayCountText(days); got != "，共 2 天" {
		t.Errorf("DayCountText = %q", got)
	}
	today := mustTime(t, "2006-01-02", "2026-09-17")
	if got := RelativeDayText(mustTime(t, "2006-01-02", "2026-09-20"), today); got != "大后天" {
		t.Errorf("RelativeDayText(+3) = %q", got)
	}
	if got := RelativeDayText(mustTime(t, "2006-01-02", "2026-09-21"), today); got != "4 天后" {
		t.Errorf("RelativeDayText(+4) = %q", got)
	}
}
