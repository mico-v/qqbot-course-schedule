package schedule

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// RankMaxRangeDays is the longest window the rank board accepts.
const RankMaxRangeDays = 366

// DefaultRankTopN is how many rows the rank card shows.
const DefaultRankTopN = 20

// RankRow is one member's row on the class-hours leaderboard.
type RankRow struct {
	Rank           int
	UserID         string
	Name           string
	Minutes        int
	ElapsedMinutes int
	HoursText      string
	ElapsedText    string
	CourseCount    int
	CourseNames    int
	AllDayCount    int
	Progress       float64
}

// IsAllDay reports a DATE-only event, which must not count towards class hours.
func IsAllDay(event Event) bool {
	raw := strings.TrimSpace(event["DTSTART"])
	return len(raw) == 8 && isDigits(raw)
}

func matchesKeywords(event Event, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}
	summary := strings.ToLower(strings.TrimSpace(event["SUMMARY"]))
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(summary, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// ClippedOccurrences trims occurrences to the window and drops the ones the
// metric must not count (all-day events by default).
func ClippedOccurrences(occurrences []Occurrence, startBound, endBound time.Time, includeAllDay bool, excludeKeywords []string) []Occurrence {
	var clipped []Occurrence
	for _, occurrence := range occurrences {
		if !includeAllDay && IsAllDay(occurrence.Event) {
			continue
		}
		if matchesKeywords(occurrence.Event, excludeKeywords) {
			continue
		}
		start := occurrence.Start
		if start.Before(startBound) {
			start = startBound
		}
		end := occurrence.End
		if end.After(endBound) {
			end = endBound
		}
		if !end.After(start) {
			continue
		}
		clipped = append(clipped, Occurrence{
			Event:       occurrence.Event,
			Start:       start,
			End:         end,
			SourceIndex: occurrence.SourceIndex,
			ShiftedFrom: occurrence.ShiftedFrom,
		})
	}
	return clipped
}

func totalMinutes(intervals [][2]time.Time, merge bool) int {
	if merge {
		intervals = MergeIntervals(intervals)
	}
	seconds := 0.0
	for _, interval := range intervals {
		seconds += interval[1].Sub(interval[0]).Seconds()
	}
	return int(math.Round(seconds / 60))
}

// BuildRankRows ranks members by how much class time their schedule occupies in
// a window. minutes counts the whole window; elapsedMinutes only the finished
// part. The union metric counts overlapping courses once.
func BuildRankRows(members map[string]*Member, startBound, endBound time.Time, now time.Time) []RankRow {
	current := now.In(LocalTZ)
	var rows []RankRow
	for userID, member := range members {
		if member == nil {
			continue
		}
		raw := ExpandMemberOccurrences(member, startBound, endBound)
		counted := ClippedOccurrences(raw, startBound, endBound, false, nil)
		if len(counted) == 0 {
			continue
		}
		intervals := make([][2]time.Time, 0, len(counted))
		var finished [][2]time.Time
		names := make(map[string]bool)
		for _, occurrence := range counted {
			intervals = append(intervals, [2]time.Time{occurrence.Start, occurrence.End})
			if !occurrence.End.After(current) {
				finished = append(finished, [2]time.Time{occurrence.Start, occurrence.End})
			}
			name := strings.TrimSpace(occurrence.Event["SUMMARY"])
			if name == "" {
				name = "未命名课程"
			}
			names[name] = true
		}
		allDayCount := 0
		for _, occurrence := range raw {
			if IsAllDay(occurrence.Event) {
				allDayCount++
			}
		}
		minutes := totalMinutes(intervals, true)
		elapsed := totalMinutes(finished, true)
		rows = append(rows, RankRow{
			UserID:         userID,
			Name:           firstNonEmpty(member.Name, userID),
			Minutes:        minutes,
			ElapsedMinutes: elapsed,
			HoursText:      FormatDurationMinutes(minutes),
			ElapsedText:    FormatDurationMinutes(elapsed),
			CourseCount:    len(intervals),
			CourseNames:    len(names),
			AllDayCount:    allDayCount,
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Minutes != rows[j].Minutes {
			return rows[i].Minutes > rows[j].Minutes
		}
		if rows[i].CourseCount != rows[j].CourseCount {
			return rows[i].CourseCount > rows[j].CourseCount
		}
		left, right := strings.ToLower(rows[i].Name), strings.ToLower(rows[j].Name)
		if left != right {
			return left < right
		}
		return rows[i].UserID < rows[j].UserID
	})

	leader := 0
	if len(rows) > 0 {
		leader = rows[0].Minutes
	}
	sharedRank := 0
	previousMinutes := -1
	for index := range rows {
		row := &rows[index]
		if row.Minutes <= 0 {
			continue
		}
		if previousMinutes != row.Minutes {
			sharedRank = index + 1
			previousMinutes = row.Minutes
		}
		row.Rank = sharedRank
		if leader > 0 {
			row.Progress = float64(row.Minutes) / float64(leader)
		}
	}
	return rows
}

// RankBoardRows parses a period expression and builds the leaderboard for a scope.
func (s *Service) RankBoardRows(scopeID, period string, now time.Time) ([]RankRow, string, error) {
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, "", err
	}
	if len(members) == 0 {
		return nil, "", nil
	}
	raw := strings.TrimSpace(period)
	if raw == "" {
		raw = "本周"
	}
	window, err := ParseTimeRange(raw, now)
	if err != nil {
		return nil, "", err
	}
	if int(window.End.Sub(window.Start).Hours()/24) > RankMaxRangeDays {
		return nil, "", fmt.Errorf("统计范围最长 %d 天，请缩小范围。", RankMaxRangeDays)
	}
	return BuildRankRows(members, window.Start, window.End, now), window.Label, nil
}
