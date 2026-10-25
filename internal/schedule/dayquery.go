package schedule

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DayQuery is one /课表 target: the selected day plus an optional reference
// instant. At is zero when the user did not ask for a specific time, in which
// case callers keep using the real current time.
type DayQuery struct {
	Day time.Time
	At  time.Time
}

// HasTime reports whether the query pins a clock time.
func (q DayQuery) HasTime() bool { return !q.At.IsZero() }

// periodSpec maps a time-of-day word to the clock used as its reference. The
// reference is the start of the period so courses later in it stay visible.
type periodSpec struct {
	word   string
	hour   int
	minute int
}

var periodSpecs = []periodSpec{
	{"凌晨", 0, 0},
	{"清晨", 6, 0},
	{"早晨", 6, 0},
	{"早上", 6, 0},
	{"上午", 8, 0},
	{"中午", 12, 0},
	{"下午", 13, 0},
	{"傍晚", 17, 0},
	{"晚上", 18, 0},
	{"夜里", 20, 0},
	{"夜晚", 20, 0},
}

var combinedDayPeriods = []struct{ from, to string }{
	{"今晚", "今天 晚上"},
	{"明晚", "明天 晚上"},
	{"昨晚", "昨天 晚上"},
	{"今早", "今天 早上"},
	{"明早", "明天 早上"},
	{"昨早", "昨天 早上"},
}

var clockSuffixRe = regexp.MustCompile(`(?:^|[^\d])(\d{1,2})\s*[:：点时]\s*(\d{1,2})?\s*[分半]?$`)

// ParseDayQuery resolves a /课表 argument to one day and an optional clock.
// A nil query means "today, using the real current time".
func ParseDayQuery(value string, today time.Time) (*DayQuery, string) {
	rest, offset, hasTime, err := splitTimeOfDay(value)
	if err != nil {
		return nil, err.Error()
	}
	days, tail, err := SplitDayOverrideArgs(rest, today)
	if err != nil {
		return nil, err.Error()
	}
	if len(tail) > 0 {
		return nil, fmt.Sprintf("无法识别日期“%s”。%s", strings.Join(tail, " "), scheduleQueryHint)
	}
	if len(days) > 1 {
		if hasTime {
			return nil, fmt.Sprintf("指定时间时只能查看一天（当前解析出 %d 天），例如 /课表 明天 晚上 或 /课表 10-10 8:00。", len(days))
		}
		return nil, fmt.Sprintf("一次只能查看一天（当前解析出 %d 天），例如 /课表 9.17 或 /课表 明天。", len(days))
	}
	if !hasTime && len(days) == 0 {
		return nil, ""
	}

	day := beginningOfDay(today)
	if len(days) == 1 {
		day = days[0]
	}
	query := &DayQuery{Day: day}
	if hasTime {
		query.At = day.Add(offset)
	}
	return query, ""
}

const scheduleQueryHint = DayHint + " 也可以在日期后追加时间，例如 明天 晚上、10-10 8:00。"

// splitTimeOfDay removes a trailing clock and/or period word from value. It
// accepts forms such as 8:00, 8点, 8时30分, 晚上, 明天晚上 and 下午3点.
func splitTimeOfDay(value string) (rest string, offset time.Duration, found bool, err error) {
	text := normalizeDayPeriods(value)
	hour, minute := -1, 0
	period := periodSpec{}
	hasPeriod, hasClock := false, false

	for attempt := 0; attempt < 2; attempt++ {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			break
		}
		if !hasPeriod {
			if spec, ok := trailingPeriod(trimmed); ok {
				period, hasPeriod = spec, true
				text = strings.TrimSpace(strings.TrimSuffix(trimmed, spec.word))
				found = true
				continue
			}
		}
		if !hasClock {
			clockHour, clockMinute, prefix, ok := trailingClock(trimmed)
			if ok {
				hour, minute, hasClock = clockHour, clockMinute, true
				text = strings.TrimSpace(prefix)
				found = true
				continue
			}
		}
		break
	}

	if !found {
		return value, 0, false, nil
	}
	if hasClock {
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return value, 0, false, fmt.Errorf("时间“%s”无效，请使用 8:00、20:30 这样的写法。", strings.TrimSpace(value))
		}
		if hasPeriod {
			switch period.word {
			case "下午", "傍晚", "晚上", "夜里", "夜晚":
				if hour < 12 {
					hour += 12
				}
			case "凌晨":
				if hour == 12 {
					hour = 0
				}
			}
		}
	} else {
		hour, minute = period.hour, period.minute
	}
	return strings.TrimSpace(text), time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, true, nil
}

// normalizeDayPeriods expands compact words such as 今晚 into 今天 晚上 so the
// regular day and period parsers can handle them.
func normalizeDayPeriods(value string) string {
	text := strings.TrimSpace(value)
	for _, item := range combinedDayPeriods {
		if strings.HasPrefix(text, item.from) {
			return strings.TrimSpace(item.to + strings.TrimPrefix(text, item.from))
		}
	}
	return text
}

func trailingPeriod(value string) (periodSpec, bool) {
	for _, spec := range periodSpecs {
		if strings.HasSuffix(value, spec.word) {
			return spec, true
		}
	}
	return periodSpec{}, false
}

// trailingClock returns the hour/minute of a clock expression at the end of
// value plus the text before it. The leading boundary in the regex keeps the
// hour from starting in the middle of a date such as 10-10.
func trailingClock(value string) (hour, minute int, prefix string, ok bool) {
	loc := clockSuffixRe.FindStringSubmatchIndex(value)
	if loc == nil {
		return 0, 0, "", false
	}
	hour = atoi(value[loc[2]:loc[3]])
	if loc[4] >= 0 {
		minute = atoi(value[loc[4]:loc[5]])
	}
	if strings.HasSuffix(value[:loc[1]], "半") {
		minute = 30
	}
	return hour, minute, value[:loc[2]], true
}
