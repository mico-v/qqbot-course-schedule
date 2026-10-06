package schedule

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxWakeUpNodes bounds course step expansion so a hostile payload cannot make
// the parser walk an unbounded number of periods.
const maxWakeUpNodes = 60

// WakeUpNode is one period in the WakeUp timetable (作息时间).
type WakeUpNode struct {
	Node      int    `json:"node"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// WakeUpTable is the schedule-metadata line of a share payload.
type WakeUpTable struct {
	StartDate string `json:"startDate"`
	TableName string `json:"tableName"`
	School    string `json:"school"`
	MaxWeek   int    `json:"maxWeek"`
	Nodes     int    `json:"nodes"`
	// SundayFirst is the app's "周日为每周第一天" display preference. It only
	// reorders columns in the app; stored course days stay 1..7 = Mon..Sun.
	SundayFirst bool `json:"sundayFirst"`
}

// WakeUpCourse is one course row of a share payload.
type WakeUpCourse struct {
	CourseName string `json:"courseName"`
	Name       string `json:"name"`
	Teacher    string `json:"teacher"`
	Room       string `json:"room"`
	Day        int    `json:"day"`
	StartNode  int    `json:"startNode"`
	Step       int    `json:"step"`
	StartWeek  int    `json:"startWeek"`
	EndWeek    int    `json:"endWeek"`
}

// WakeUpShare is a decoded WakeUp share payload (the "shareData" text).
type WakeUpShare struct {
	Nodes   []WakeUpNode
	Table   WakeUpTable
	Courses []WakeUpCourse
}

// ParseWakeUpShare parses the newline-separated lines of a share payload:
// timetable, nodes array, table metadata, then one JSON object per course.
func ParseWakeUpShare(shareData string) (*WakeUpShare, error) {
	share := &WakeUpShare{}
	for _, raw := range strings.Split(shareData, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			var nodes []WakeUpNode
			if err := json.Unmarshal([]byte(line), &nodes); err == nil && len(nodes) > 0 {
				share.Nodes = nodes
				continue
			}
		}
		var probe map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			continue
		}
		switch {
		case probe["startDate"] != nil || probe["maxWeek"] != nil:
			_ = json.Unmarshal([]byte(line), &share.Table)
		case probe["courseName"] != nil || probe["startNode"] != nil:
			var course WakeUpCourse
			if err := json.Unmarshal([]byte(line), &course); err == nil {
				share.Courses = append(share.Courses, course)
			}
		}
	}
	if len(share.Nodes) == 0 {
		return nil, fmt.Errorf("WakeUp 分享数据缺少作息时间")
	}
	return share, nil
}

// Events converts the share payload into canonical course events.
//
// WakeUp numbers weekdays 1..7 as Monday..Sunday and stores the term start as
// the Monday of week 1, so the first meeting of a course is
// startDate + (startWeek-1)*7 + (day-1) days. Recurrence is one weekly RRULE
// with COUNT equal to the number of weeks the course spans.
func (s *WakeUpShare) Events() ([]Event, error) {
	if strings.TrimSpace(s.Table.StartDate) == "" {
		return nil, fmt.Errorf("WakeUp 分享数据缺少开学日期")
	}
	startDate, err := parseWakeUpDate(s.Table.StartDate)
	if err != nil {
		return nil, err
	}
	nodes := make(map[int]WakeUpNode, len(s.Nodes))
	for _, node := range s.Nodes {
		nodes[node.Node] = node
	}

	var events []Event
	for _, course := range s.Courses {
		name := firstNonEmpty(strings.TrimSpace(course.CourseName), strings.TrimSpace(course.Name))
		if name == "" || course.Day < 1 || course.Day > 7 {
			continue
		}
		start, ok := nodes[course.StartNode]
		if !ok {
			continue
		}
		// Walk the covered nodes and use the last one that exists, so a step
		// past the timetable edge (or a gap) degrades to the previous period
		// instead of overflowing.
		step := course.Step
		if step < 1 {
			step = 1
		}
		if step > maxWakeUpNodes {
			step = maxWakeUpNodes
		}
		end := start
		for nodeIndex := course.StartNode + 1; nodeIndex < course.StartNode+step; nodeIndex++ {
			if candidate, exists := nodes[nodeIndex]; exists {
				end = candidate
			}
		}
		startWeek := max(course.StartWeek, 1)
		endWeek := course.EndWeek
		if endWeek < startWeek {
			endWeek = startWeek
		}
		if s.Table.MaxWeek > 0 {
			if startWeek > s.Table.MaxWeek {
				continue
			}
			if endWeek > s.Table.MaxWeek {
				endWeek = s.Table.MaxWeek
			}
		}
		firstDay := startDate.AddDate(0, 0, (startWeek-1)*7+(course.Day-1))
		dtstart, startOK := wakeUpDateTime(firstDay, start.StartTime)
		dtend, endOK := wakeUpDateTime(firstDay, end.EndTime)
		if !startOK || !endOK {
			continue
		}
		event := Event{
			"UID":     wakeUpEventUID(name, course, firstDay),
			"SUMMARY": name,
			"DTSTART": dtstart.Format("20060102T150405"),
			"DTEND":   dtend.Format("20060102T150405"),
			"DTSTAMP": time.Now().UTC().Format("20060102T150405Z"),
			"RRULE":   fmt.Sprintf("FREQ=WEEKLY;COUNT=%d;WKST=MO", endWeek-startWeek+1),
		}
		if room := strings.TrimSpace(course.Room); room != "" {
			event["LOCATION"] = room
		}
		if teacher := strings.TrimSpace(course.Teacher); teacher != "" {
			event["DESCRIPTION"] = teacher
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("WakeUp 分享数据中没有可用的课程")
	}
	SortEvents(events)
	return events, nil
}

// ParseWakeUpEventsAndSchedule parses a share payload into events and the
// human-readable schedule text, mirroring ParseICSEventsAndSchedule.
func ParseWakeUpEventsAndSchedule(shareData string) ([]Event, string, error) {
	share, err := ParseWakeUpShare(shareData)
	if err != nil {
		return nil, "", err
	}
	events, err := share.Events()
	if err != nil {
		return nil, "", err
	}
	return events, FormatICSSchedule(events), nil
}

func wakeUpEventUID(name string, course WakeUpCourse, day time.Time) string {
	return fmt.Sprintf("wakeup-%s-%d-%d-%d-%d",
		strings.ReplaceAll(name, " ", "_"),
		course.Day, course.StartNode, course.StartWeek, day.Unix())
}

func parseWakeUpDate(value string) (time.Time, error) {
	for _, layout := range []string{"2006-1-2", "2006-01-02", "2006/1/2", "2006/01/02"} {
		if parsed, err := time.ParseInLocation(layout, value, LocalTZ); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析 WakeUp 开学日期 %q", value)
}

func wakeUpDateTime(day time.Time, clock string) (time.Time, bool) {
	hour, minute, ok := parseWakeUpClock(clock)
	if !ok {
		return time.Time{}, false
	}
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, LocalTZ), true
}

func parseWakeUpClock(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 {
		return 0, 0, false
	}
	hour, errHour := strconv.Atoi(strings.TrimSpace(parts[0]))
	minute, errMinute := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errHour != nil || errMinute != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}
