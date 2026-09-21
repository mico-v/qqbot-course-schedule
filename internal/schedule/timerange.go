package schedule

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TimeRange is a half-open [Start, End) window plus a display label.
type TimeRange struct {
	Start time.Time
	End   time.Time
	Label string
}

var timeRangeSplitRe = regexp.MustCompile(`(?i)\s*(?:\.\.|~|至|到|—|-{2,}|\bto\b)\s*`)

var singleDayTokens = map[string]int{
	"": 0, "today": 0, "今天": 0, "今日": 0,
	"tomorrow": 1, "明天": 1, "明日": 1,
	"yesterday": -1, "昨天": -1, "昨日": -1,
}

var (
	thisWeek  = map[string]bool{"thisweek": true, "currentweek": true, "week": true, "本周": true, "这周": true, "这一周": true}
	nextWeek  = map[string]bool{"nextweek": true, "下周": true, "下一周": true}
	lastWeek  = map[string]bool{"lastweek": true, "prevweek": true, "上周": true, "上一周": true}
	thisMonth = map[string]bool{"thismonth": true, "currentmonth": true, "month": true, "本月": true, "这个月": true}
	nextMonth = map[string]bool{"nextmonth": true, "下月": true, "下个月": true}
	lastMonth = map[string]bool{"lastmonth": true, "prevmonth": true, "上月": true, "上个月": true}
)

const timeRangeHint = "无法识别时间范围“%s”，请使用 今日、本周、上周、本月、上月、下月，或 2026-09-01..2026-09-30 这样的日期范围。"

// ParseTimeRange understands the shared range vocabulary.
func ParseTimeRange(value string, today time.Time) (TimeRange, error) {
	today = beginningOfDay(today)
	normalized := strings.ToLower(strings.TrimSpace(value))
	compact := strings.Join(strings.Fields(normalized), "")

	var startDate, endDate time.Time
	switch {
	case singleDayTokens[compact] != 0 || compact == "":
		startDate = today.AddDate(0, 0, singleDayTokens[compact])
		endDate = startDate
	case thisWeek[compact]:
		startDate, endDate = weekBounds(today, 0)
	case nextWeek[compact]:
		startDate, endDate = weekBounds(today, 1)
	case lastWeek[compact]:
		startDate, endDate = weekBounds(today, -1)
	case thisMonth[compact]:
		startDate, endDate = monthBounds(today, 0)
	case nextMonth[compact]:
		startDate, endDate = monthBounds(today, 1)
	case lastMonth[compact]:
		startDate, endDate = monthBounds(today, -1)
	default:
		parts := splitNonEmpty(timeRangeSplitRe.Split(normalized, -1))
		var err error
		if len(parts) == 2 {
			startDate, err = parseRangeDateToken(parts[0], today)
			if err != nil {
				return TimeRange{}, err
			}
			endDate, err = parseRangeDateToken(parts[1], today)
			if err != nil {
				return TimeRange{}, err
			}
		} else {
			startDate, err = parseRangeDateToken(normalized, today)
			if err != nil {
				return TimeRange{}, err
			}
			endDate = startDate
		}
	}

	if endDate.Before(startDate) {
		return TimeRange{}, fmt.Errorf("时间范围的结束日期不能早于开始日期。")
	}
	label := startDate.Format("2006-01-02")
	if !startDate.Equal(endDate) {
		label = fmt.Sprintf("%s..%s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	}
	return TimeRange{
		Start: startDate,
		End:   endDate.AddDate(0, 0, 1),
		Label: label,
	}, nil
}

func parseRangeDateToken(token string, today time.Time) (time.Time, error) {
	normalized := strings.ToLower(strings.TrimSpace(token))
	if delta, ok := singleDayTokens[normalized]; ok {
		return beginningOfDay(today).AddDate(0, 0, delta), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", normalized, LocalTZ)
	if err != nil {
		return time.Time{}, fmt.Errorf(timeRangeHint, token)
	}
	return parsed, nil
}

func weekBounds(today time.Time, offsetWeeks int) (time.Time, time.Time) {
	// Go's Weekday starts on Sunday; convert to Monday-based.
	weekday := (int(today.Weekday()) + 6) % 7
	start := today.AddDate(0, 0, -weekday+7*offsetWeeks)
	return start, start.AddDate(0, 0, 6)
}

func monthBounds(day time.Time, offsetMonths int) (time.Time, time.Time) {
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, LocalTZ).AddDate(0, offsetMonths, 0)
	last := first.AddDate(0, 1, -1)
	return first, last
}
