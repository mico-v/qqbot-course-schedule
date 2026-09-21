package schedule

import (
	"sort"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// Occurrence is one concrete class meeting expanded from a stored event.
type Occurrence struct {
	Event       Event
	Start       time.Time
	End         time.Time
	SourceIndex int    // 1-based position in the member's stored event list
	ShiftedFrom string // source day "2006-01-02" when produced by a 调休 marker
}

// EventDatetimes returns the event start and end in local time.
func EventDatetimes(event Event) (time.Time, time.Time, bool) {
	start, ok := ParseICSTime(event["DTSTART"], event["DTSTART_TZID"])
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, endOK := ParseICSTime(event["DTEND"], event["DTEND_TZID"])
	if !endOK {
		duration, hasDuration := EventDurationFromRaw(event)
		if !hasDuration {
			if len(strings.TrimSpace(event["DTSTART"])) == 8 {
				duration = 24 * time.Hour
			} else {
				duration = 90 * time.Minute
			}
		}
		end = start.Add(duration)
	}
	if !end.After(start) {
		end = start.Add(90 * time.Minute)
	}
	return start.In(LocalTZ), end.In(LocalTZ), true
}

// ExpandEventOccurrences expands one event inside [startBound, endBound).
func ExpandEventOccurrences(event Event, startBound, endBound time.Time) []Occurrence {
	start, end, ok := EventDatetimes(event)
	if !ok {
		return nil
	}
	duration := end.Sub(start)

	var starts []time.Time
	if rruleText := strings.TrimSpace(event["RRULE"]); rruleText != "" {
		if rule, err := rrule.StrToRRule(rruleText); err == nil {
			rule.DTStart(start)
			starts = rule.Between(startBound.Add(-duration), endBound, true)
		}
	} else {
		starts = []time.Time{start}
	}
	starts = append(starts, recurrenceDates(event, "RDATE")...)

	excluded := make(map[int64]bool)
	for _, day := range recurrenceDates(event, "EXDATE") {
		excluded[day.UnixNano()] = true
	}

	var result []Occurrence
	seen := make(map[int64]bool)
	for _, candidate := range starts {
		occurrenceStart := candidate.In(LocalTZ)
		key := occurrenceStart.UnixNano()
		if excluded[key] || seen[key] {
			continue
		}
		occurrenceEnd := occurrenceStart.Add(duration)
		if occurrenceStart.Before(endBound) && occurrenceEnd.After(startBound) {
			seen[key] = true
			result = append(result, Occurrence{Event: event, Start: occurrenceStart, End: occurrenceEnd})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Start.Before(result[j].Start) })
	return result
}

// ExpandEventsOnDay expands every event over one local calendar day.
func ExpandEventsOnDay(events []Event, day time.Time) []Occurrence {
	dayStart := beginningOfDay(day)
	dayEnd := dayStart.Add(24 * time.Hour)
	dayText := dayStart.Format("2006-01-02")
	var found []Occurrence
	for index, event := range events {
		for _, occurrence := range ExpandEventOccurrences(event, dayStart, dayEnd) {
			if occurrence.Start.Format("2006-01-02") != dayText {
				continue
			}
			occurrence.SourceIndex = index + 1
			found = append(found, occurrence)
		}
	}
	return found
}

// ExpandIndexedOccurrences expands a member's events inside a window, applying
// holiday and shift markers. Holiday days lose their own occurrences; shift
// days show the source day's classes at the same clock times.
func ExpandIndexedOccurrences(events []Event, overrides map[string]DayOverride, startBound, endBound time.Time) []Occurrence {
	var indexed []Occurrence
	for index, event := range events {
		for _, occurrence := range ExpandEventOccurrences(event, startBound, endBound) {
			if _, overridden := overrides[occurrence.Start.Format("2006-01-02")]; overridden {
				continue
			}
			occurrence.SourceIndex = index + 1
			indexed = append(indexed, occurrence)
		}
	}

	sourceCache := make(map[string][]Occurrence)
	for dayText, rule := range overrides {
		if rule.Kind != DayOverrideShift || rule.SourceDay == "" {
			continue
		}
		targetDay, err := time.ParseInLocation("2006-01-02", dayText, LocalTZ)
		if err != nil {
			continue
		}
		sourceDay, err := time.ParseInLocation("2006-01-02", rule.SourceDay, LocalTZ)
		if err != nil {
			continue
		}
		targetStart := beginningOfDay(targetDay)
		targetEnd := targetStart.Add(24 * time.Hour)
		if !targetStart.Before(endBound) || !targetEnd.After(startBound) {
			continue
		}
		sourceStart := beginningOfDay(sourceDay)
		cached, ok := sourceCache[rule.SourceDay]
		if !ok {
			cached = ExpandEventsOnDay(events, sourceDay)
			sourceCache[rule.SourceDay] = cached
		}
		for _, occurrence := range cached {
			shiftedStart := targetStart.Add(occurrence.Start.Sub(sourceStart))
			shifted := occurrence
			shifted.Start = shiftedStart
			shifted.End = shiftedStart.Add(occurrence.End.Sub(occurrence.Start))
			shifted.ShiftedFrom = rule.SourceDay
			indexed = append(indexed, shifted)
		}
	}

	sort.SliceStable(indexed, func(i, j int) bool {
		if indexed[i].Start.Equal(indexed[j].Start) {
			return indexed[i].End.Before(indexed[j].End)
		}
		return indexed[i].Start.Before(indexed[j].Start)
	})
	return indexed
}

// ExpandMemberOccurrences expands one member's schedule inside a window.
func ExpandMemberOccurrences(member *Member, startBound, endBound time.Time) []Occurrence {
	if member == nil {
		return nil
	}
	return ExpandIndexedOccurrences(member.Events, member.DayOverrides, startBound, endBound)
}

func beginningOfDay(value time.Time) time.Time {
	local := value.In(LocalTZ)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, LocalTZ)
}
