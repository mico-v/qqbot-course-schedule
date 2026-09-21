package schedule

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Roll controls how a year-less date is resolved.
type Roll int

const (
	// RollForward always points ahead (used when marking future days).
	RollForward Roll = iota
	// RollNearest picks the closest occurrence in either direction.
	RollNearest
)

var relativeDays = map[string]int{
	"今天": 0, "今日": 0, "today": 0, "now": 0,
	"明天": 1, "明日": 1, "tomorrow": 1,
	"后天": 2, "後天": 2, "大后天": 3,
	"昨天": -1, "昨日": -1, "yesterday": -1,
	"前天": -2,
}

var dayDeltaText = map[int]string{
	0: "今天", 1: "明天", 2: "后天", 3: "大后天", -1: "昨天", -2: "前天",
}

// DayHint is the shared "how to write a date" help text.
const DayHint = "支持 2026-09-17、9.17、9月17日、今天、明天、后天、昨天 等写法。"

var fillerTokens = map[string]bool{
	"上": true, "按": true, "补": true, "调": true, "换": true,
	"改成": true, "改到": true, "换成": true, "替换": true, "标记": true,
	"设成": true, "设为": true, "假期": true, "放假": true, "休假": true,
	"调休": true, "的": true, "的课": true, "课": true, "课程": true,
	"到": true, "至": true, "从": true, "把": true, "给": true, "为": true,
	"在": true, "和": true, "与": true, "→": true, "->": true, "=>": true,
	"=": true, "--": true,
}

var (
	dateTextRe         = regexp.MustCompile(`^(\d{4})[-/.年](\d{1,2})[-/.月](\d{1,2})日?$`)
	monthDayRe         = regexp.MustCompile(`^(\d{1,2})[-/.月](\d{1,2})日?$`)
	rangeSplitRe       = regexp.MustCompile(`\s*(?:\.\.|~|～|—|至|到)\s*`)
	rangeJoinRe        = regexp.MustCompile(`\s*(\.\.|~|～|—)\s*`)
	tokenSplitRe       = regexp.MustCompile(`[\s,，、;；]+`)
	leadingParticleRe  = regexp.MustCompile(`^(?:上|按|补|调|到|从|把|给|为|在|换|改)+`)
	trailingParticleRe = regexp.MustCompile(`(?:的(?:课|课程|课程表)?|课|课程)+$`)
)

const yearSearchSpan = 4

func dayAt(year int, month time.Month, day int) (time.Time, bool) {
	value := time.Date(year, month, day, 0, 0, 0, 0, LocalTZ)
	if value.Year() != year || value.Month() != month || value.Day() != day {
		return time.Time{}, false
	}
	return value, true
}

func yearCandidates(month time.Month, day int, today time.Time) []time.Time {
	var candidates []time.Time
	for offset := -yearSearchSpan; offset <= yearSearchSpan; offset++ {
		if candidate, ok := dayAt(today.Year()+offset, month, day); ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func pickYear(candidates []time.Time, today time.Time, roll Roll) (time.Time, bool) {
	if len(candidates) == 0 {
		return time.Time{}, false
	}
	if roll == RollNearest {
		best := candidates[0]
		bestScore := absInt(int(best.Sub(today).Hours() / 24))
		for _, candidate := range candidates[1:] {
			score := absInt(int(candidate.Sub(today).Hours() / 24))
			if score < bestScore || (score == bestScore && candidate.After(best)) {
				best, bestScore = candidate, score
			}
		}
		return best, true
	}
	var ahead []time.Time
	for _, candidate := range candidates {
		if !candidate.Before(today) {
			ahead = append(ahead, candidate)
		}
	}
	if len(ahead) > 0 {
		sort.Slice(ahead, func(i, j int) bool { return ahead[i].Before(ahead[j]) })
		return ahead[0], true
	}
	latest := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.After(latest) {
			latest = candidate
		}
	}
	return latest, true
}

// ParseDayToken parses one token as a day. found is false when the token is not
// date-shaped at all; err is set when it is date-shaped but invalid.
func ParseDayToken(value string, today time.Time, roll Roll) (day time.Time, found bool, err error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if delta, ok := relativeDays[strings.ToLower(raw)]; ok {
		return beginningOfDay(today).AddDate(0, 0, delta), true, nil
	}
	if matched := dateTextRe.FindStringSubmatch(raw); matched != nil {
		year, month, dayOfMonth := atoi(matched[1]), atoi(matched[2]), atoi(matched[3])
		parsed, ok := dayAt(year, time.Month(month), dayOfMonth)
		if !ok {
			return time.Time{}, false, fmt.Errorf("日期“%s”不存在，请检查后重试。", raw)
		}
		return parsed, true, nil
	}
	if matched := monthDayRe.FindStringSubmatch(raw); matched != nil {
		month, dayOfMonth := atoi(matched[1]), atoi(matched[2])
		parsed, ok := pickYear(yearCandidates(time.Month(month), dayOfMonth, today), beginningOfDay(today), roll)
		if !ok {
			return time.Time{}, false, fmt.Errorf("日期“%s”不存在，请检查后重试。", raw)
		}
		return parsed, true, nil
	}
	return time.Time{}, false, nil
}

func looseDay(value string, today time.Time, roll Roll) (time.Time, bool) {
	if parsed, found, err := ParseDayToken(value, today, roll); err == nil && found {
		return parsed, true
	}
	trimmed := strings.TrimSpace(value)
	cleaned := trailingParticleRe.ReplaceAllString(leadingParticleRe.ReplaceAllString(trimmed, ""), "")
	if cleaned == "" || cleaned == trimmed {
		return time.Time{}, false
	}
	if parsed, found, err := ParseDayToken(cleaned, today, roll); err == nil && found {
		return parsed, true
	}
	return time.Time{}, false
}

func rangeDays(value string, today time.Time, roll Roll) ([]time.Time, bool, error) {
	parts := splitNonEmpty(rangeSplitRe.Split(value, -1))
	if len(parts) != 2 {
		return nil, false, nil
	}
	start, startOK := looseDay(parts[0], today, roll)
	end, endOK := looseDay(parts[1], today, roll)
	if !startOK || !endOK {
		return nil, false, nil
	}
	if end.Before(start) {
		return nil, true, fmt.Errorf("日期范围的结束日期不能早于开始日期。")
	}
	span := int(end.Sub(start).Hours()/24) + 1
	if span > MaxDayOverrideRangeDays {
		return nil, true, fmt.Errorf("一次最多标记 %d 天，请拆分成多个范围后重试。", MaxDayOverrideRangeDays)
	}
	days := make([]time.Time, 0, span)
	for offset := 0; offset < span; offset++ {
		days = append(days, start.AddDate(0, 0, offset))
	}
	return days, true, nil
}

// SplitDayOverrideArgs splits a command tail into days and remaining words.
func SplitDayOverrideArgs(value string, today time.Time, roll Roll) ([]time.Time, []string, error) {
	var days []time.Time
	var rest []string
	joined := rangeJoinRe.ReplaceAllString(value, "$1")
	for _, rawToken := range tokenSplitRe.Split(joined, -1) {
		token := strings.TrimSpace(rawToken)
		if token == "" || fillerTokens[token] {
			continue
		}
		ranged, isRange, err := rangeDays(token, today, roll)
		if err != nil {
			return nil, nil, err
		}
		if isRange {
			days = append(days, ranged...)
			continue
		}
		if parsed, found, err := ParseDayToken(token, today, roll); err != nil {
			return nil, nil, err
		} else if found {
			days = append(days, parsed)
			continue
		}
		if parsed, found := looseDay(token, today, roll); found {
			days = append(days, parsed)
			continue
		}
		rest = append(rest, strings.TrimSpace(strings.TrimLeft(token, "@")))
	}
	var filtered []string
	for _, item := range rest {
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	return days, filtered, nil
}

// SingleDayQuery resolves the /课表 argument to at most one day.
func SingleDayQuery(value string, today time.Time) (day *time.Time, message string) {
	days, rest, err := SplitDayOverrideArgs(value, today, RollNearest)
	if err != nil {
		return nil, err.Error()
	}
	if len(rest) > 0 {
		return nil, fmt.Sprintf("无法识别日期“%s”。%s", strings.Join(rest, " "), DayHint)
	}
	if len(days) > 1 {
		return nil, fmt.Sprintf("一次只能查看一天（当前解析出 %d 天），例如 /课表 9.17 或 /课表 明天。", len(days))
	}
	if len(days) == 0 {
		return nil, ""
	}
	return &days[0], ""
}

// FormatDayList renders one day, a contiguous range, or a short list.
func FormatDayList(days []time.Time) string {
	if len(days) == 0 {
		return ""
	}
	seen := make(map[string]time.Time)
	for _, day := range days {
		seen[day.Format("2006-01-02")] = day
	}
	ordered := make([]time.Time, 0, len(seen))
	for _, day := range seen {
		ordered = append(ordered, day)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })
	if len(ordered) == 1 {
		return ordered[0].Format("2006-01-02")
	}
	first, last := ordered[0], ordered[len(ordered)-1]
	if int(last.Sub(first).Hours()/24)+1 == len(ordered) {
		return fmt.Sprintf("%s 至 %s", first.Format("2006-01-02"), last.Format("2006-01-02"))
	}
	shown := make([]string, 0, 6)
	for index, day := range ordered {
		if index >= 6 {
			break
		}
		shown = append(shown, day.Format("2006-01-02"))
	}
	text := strings.Join(shown, "、")
	if len(ordered) > 6 {
		text += " 等"
	}
	return text
}

// RelativeDayText renders 今天/明天/昨天 for nearby days, N 天后 beyond that.
func RelativeDayText(day, today time.Time) string {
	delta := int(beginningOfDay(day).Sub(beginningOfDay(today)).Hours() / 24)
	if text, ok := dayDeltaText[delta]; ok {
		return text
	}
	if delta > 0 {
		return fmt.Sprintf("%d 天后", delta)
	}
	return fmt.Sprintf("%d 天前", -delta)
}

// DayCountText returns "，共 N 天" for multi-day replies.
func DayCountText(days []time.Time) string {
	seen := make(map[string]bool)
	for _, day := range days {
		seen[day.Format("2006-01-02")] = true
	}
	if len(seen) > 1 {
		return fmt.Sprintf("，共 %d 天", len(seen))
	}
	return ""
}

func splitNonEmpty(values []string) []string {
	var result []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func atoi(value string) int {
	result := 0
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return result
		}
		result = result*10 + int(value[i]-'0')
	}
	return result
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
