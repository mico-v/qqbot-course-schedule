package schedule

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// DayRow is one member's card row for a selected day.
type DayRow struct {
	UserID          string
	Name            string
	Status          string
	StatusKey       string
	Course          string
	Location        string
	TimeText        string
	Duration        string
	DurationMinutes int
	CountdownLabel  string
	Countdown       string
	Progress        float64
	CourseCount     int
	OverrideKind    string
	OverrideSource  string
	OverrideNote    string
	SortPriority    int
	SortTime        float64
}

// DailyMemberRows builds one status row per member on the selected day.
func DailyMemberRows(members map[string]*Member, target time.Time, now time.Time, memberIDs []string) []DayRow {
	current := now.In(LocalTZ)
	target = beginningOfDay(target)
	isToday := target.Equal(beginningOfDay(current))
	startBound := target
	endBound := target.Add(24 * time.Hour)

	selected := memberIDs
	if selected == nil {
		selected = make([]string, 0, len(members))
		for userID := range members {
			selected = append(selected, userID)
		}
		sort.Strings(selected)
	}

	result := make([]DayRow, 0, len(selected))
	for _, userID := range selected {
		member, ok := members[userID]
		if !ok || member == nil {
			continue
		}
		occurrences := ExpandMemberOccurrences(member, startBound, endBound)
		sort.SliceStable(occurrences, func(i, j int) bool { return occurrences[i].Start.Before(occurrences[j].Start) })

		override := member.DayOverrides[target.Format("2006-01-02")]
		holiday := override.Kind == DayOverrideHoliday
		overrideSource := ""
		shiftNote := ""
		if override.Kind == DayOverrideShift && override.SourceDay != "" {
			overrideSource = override.SourceDay
			if source, err := time.ParseInLocation("2006-01-02", override.SourceDay, LocalTZ); err == nil {
				shiftNote = "调休 · 按 " + source.Format("01-02") + " 的课表"
			}
		}

		var active, upcoming *Occurrence
		for index := range occurrences {
			occurrence := &occurrences[index]
			if holiday {
				break
			}
			if active == nil && isToday && !occurrence.Start.After(current) && current.Before(occurrence.End) {
				active = occurrence
				continue
			}
			if upcoming == nil && ((isToday && occurrence.Start.After(current)) || target.After(beginningOfDay(current))) {
				upcoming = occurrence
			}
		}

		row := DayRow{
			UserID:         userID,
			Name:           displayName(member.Name, userID),
			OverrideKind:   override.Kind,
			OverrideSource: overrideSource,
			OverrideNote:   shiftNote,
		}

		var featured *Occurrence
		switch {
		case holiday:
			row.StatusKey = "holiday"
			row.Status = pickText(isToday, "今日休假", "当天休假")
			row.CountdownLabel = "假期状态"
			row.Countdown = "当天课程全部取消"
			row.SortPriority = 3
			row.SortTime = math.Inf(1)
		case active != nil:
			featured = active
			row.StatusKey = "active"
			row.Status = "正在上课"
			row.CountdownLabel = "距下课"
			row.Countdown = FormatRemaining(active.End.Sub(current))
			row.Progress = clamp01(current.Sub(active.Start).Seconds() / math.Max(1, active.End.Sub(active.Start).Seconds()))
			row.SortPriority = 0
			row.SortTime = float64(active.Start.UnixNano()) / 1e9
		case upcoming != nil:
			featured = upcoming
			row.StatusKey = "upcoming"
			row.Status = "下一节即将上课"
			row.CountdownLabel = "距上课"
			row.Countdown = FormatRemaining(upcoming.Start.Sub(current))
			row.SortPriority = 1
			row.SortTime = float64(upcoming.Start.UnixNano()) / 1e9
		case len(occurrences) > 0 && !target.After(beginningOfDay(current)):
			featured = &occurrences[len(occurrences)-1]
			row.StatusKey = "finished"
			row.Status = pickText(isToday, "今日课程已结束", "当天课程已结束")
			row.CountdownLabel = "课程状态"
			row.Countdown = pickText(isToday, "今天的课程都上完啦", "这一天的课程都上完啦")
			row.Progress = 1
			row.SortPriority = 2
			row.SortTime = -float64(featured.End.UnixNano()) / 1e9
		case len(occurrences) > 0:
			featured = &occurrences[0]
			row.StatusKey = "upcoming"
			row.Status = "下一节即将上课"
			row.CountdownLabel = "距上课"
			row.Countdown = FormatRemaining(featured.Start.Sub(current))
			row.SortPriority = 1
			row.SortTime = float64(featured.Start.UnixNano()) / 1e9
		default:
			row.StatusKey = "none"
			row.Status = pickText(isToday, "今日无课", "当天无课")
			row.CountdownLabel = "课程状态"
			row.Countdown = "休息日，安排得明明白白"
			row.SortPriority = 3
			row.SortTime = math.Inf(1)
		}

		switch {
		case holiday:
			row.Course = "休假 · 无课程安排"
			row.Duration = "—"
			row.TimeText = "当天课程已全部取消"
		case featured != nil:
			row.Course = firstNonEmpty(featured.Event["SUMMARY"], "未命名课程")
			row.Location = strings.TrimSpace(featured.Event["LOCATION"])
			row.DurationMinutes = maxInt(1, int(math.Round(featured.End.Sub(featured.Start).Minutes())))
			row.Duration = FormatDurationMinutes(row.DurationMinutes)
			row.TimeText = fmt.Sprintf("%s - %s", featured.Start.Format("15:04"), featured.End.Format("15:04"))
			if shiftNote != "" {
				row.TimeText = shiftNote + "   ·   " + row.TimeText
			}
			row.CourseCount = len(occurrences)
		default:
			row.Course = "暂无课程安排"
			row.Duration = "—"
			row.TimeText = pickText(isToday, "今天没有安排课程", "当天没有安排课程")
		}

		result = append(result, row)
	}

	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.SortPriority != right.SortPriority {
			return left.SortPriority < right.SortPriority
		}
		if left.SortTime != right.SortTime {
			return left.SortTime < right.SortTime
		}
		if leftName, rightName := strings.ToLower(left.Name), strings.ToLower(right.Name); leftName != rightName {
			return leftName < rightName
		}
		return left.UserID < right.UserID
	})
	return result
}

// foldedStatusKeys are the members with nothing left on the day.
var foldedStatusKeys = map[string]bool{"finished": true, "none": true, "holiday": true}

// SplitFoldedRows splits rows into the cards to show and the members to fold.
func SplitFoldedRows(rows []DayRow) (shown, folded []DayRow) {
	for _, row := range rows {
		if foldedStatusKeys[row.StatusKey] {
			folded = append(folded, row)
		} else {
			shown = append(shown, row)
		}
	}
	return shown, folded
}

// MergeIntervals merges overlapping or touching spans.
func MergeIntervals(intervals [][2]time.Time) [][2]time.Time {
	if len(intervals) == 0 {
		return nil
	}
	sorted := append([][2]time.Time(nil), intervals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i][0].Before(sorted[j][0]) })
	merged := [][2]time.Time{sorted[0]}
	for _, interval := range sorted[1:] {
		last := &merged[len(merged)-1]
		if interval[0].After(last[1]) {
			merged = append(merged, interval)
			continue
		}
		if interval[1].After(last[1]) {
			last[1] = interval[1]
		}
	}
	return merged
}

// FormatDurationMinutes renders "1小时30分钟".
func FormatDurationMinutes(minutes int) string {
	minutes = maxInt(0, minutes)
	hours, remainder := minutes/60, minutes%60
	switch {
	case hours > 0 && remainder > 0:
		return fmt.Sprintf("%d小时%d分钟", hours, remainder)
	case hours > 0:
		return fmt.Sprintf("%d小时", hours)
	default:
		return fmt.Sprintf("%d分钟", minutes)
	}
}

// FormatRemaining renders a countdown ("不到1分钟", "2小时15分钟").
func FormatRemaining(delta time.Duration) string {
	seconds := maxInt(0, int(delta.Seconds()))
	minutes := (seconds + 59) / 60
	if minutes < 1 {
		return "不到1分钟"
	}
	days, minutes := minutes/(24*60), minutes%(24*60)
	if days > 0 {
		hours, remainder := minutes/60, minutes%60
		text := fmt.Sprintf("%d天", days)
		if hours > 0 {
			text += fmt.Sprintf("%d小时", hours)
		}
		if remainder > 0 {
			text += fmt.Sprintf("%d分钟", remainder)
		}
		return text
	}
	return FormatDurationMinutes(minutes)
}

// ScheduleFooter returns the footer line for a day view.
func ScheduleFooter(selected, today time.Time) string {
	selected, today = beginningOfDay(selected), beginningOfDay(today)
	switch {
	case selected.Before(today):
		return "历史课表 · 课程时间以本地时区为准"
	case selected.After(today):
		return "课程安排 · 课程时间以本地时区为准"
	default:
		return "实时状态 · 课程时间以本地时区为准"
	}
}

func displayName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return name
}

func pickText(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
