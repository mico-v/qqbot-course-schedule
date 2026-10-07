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
	// TimeTable links a node to the作息表 (time table) it belongs to.
	TimeTable int `json:"timeTable"`
}

// WakeUpTable is the schedule-metadata line of a share payload.
type WakeUpTable struct {
	ID        int    `json:"id"`
	TimeTable int    `json:"timeTable"`
	StartDate string `json:"startDate"`
	TableName string `json:"tableName"`
	School    string `json:"school"`
	MaxWeek   int    `json:"maxWeek"`
	Nodes     int    `json:"nodes"`
	// SundayFirst is the app's "周日为每周第一天" display preference. It only
	// reorders columns in the app; stored course days stay 1..7 = Mon..Sun.
	SundayFirst bool `json:"sundayFirst"`
}

// WakeUpCourse is one course occurrence row of a share payload.
type WakeUpCourse struct {
	// ID references the course definition (name) in the same table.
	ID         int    `json:"id"`
	TableID    int    `json:"tableId"`
	CourseName string `json:"courseName"`
	Name       string `json:"name"`
	Teacher    string `json:"teacher"`
	Room       string `json:"room"`
	Day        int    `json:"day"`
	StartNode  int    `json:"startNode"`
	Step       int    `json:"step"`
	StartWeek  int    `json:"startWeek"`
	EndWeek    int    `json:"endWeek"`
	// Type is the app's isOdd flag: 0 = every week, 1 = odd weeks, 2 = even weeks.
	Type int `json:"type"`
	// OwnTime means the row overrides the timetable with StartTime/EndTime.
	OwnTime   bool   `json:"ownTime"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// WakeUpShare is a decoded WakeUp share payload (the "shareData" text).
//
// The real payload is a sequence of JSON lines: a作息表 object plus a node
// array per time table, then a课程表 object, a course-definition array and a
// course-occurrence array per table. Older/hand-written payloads use one flat
// course object per line, which is still accepted.
type WakeUpShare struct {
	Nodes   []WakeUpNode
	Table   WakeUpTable
	Courses []WakeUpCourse

	// nodesByTable groups periods by作息表 id; tables maps课程表 id to metadata.
	nodesByTable map[int][]WakeUpNode
	tables       map[int]WakeUpTable
}

// wakeUpCourseDef is a course-definition row: it carries the human-readable
// name that course occurrences reference by table-scoped id.
type wakeUpCourseDef struct {
	ID         int    `json:"id"`
	TableID    int    `json:"tableId"`
	CourseName string `json:"courseName"`
}

// ParseWakeUpShare parses the newline-separated lines of a share payload.
func ParseWakeUpShare(shareData string) (*WakeUpShare, error) {
	share := &WakeUpShare{
		nodesByTable: map[int][]WakeUpNode{},
		tables:       map[int]WakeUpTable{},
	}
	defs := map[int]map[int]string{}
	var nodes []WakeUpNode
	var courses []WakeUpCourse
	tableSet := false

	for _, raw := range strings.Split(shareData, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			switch classifyWakeUpArray(line) {
			case "nodes":
				var parsed []WakeUpNode
				if err := json.Unmarshal([]byte(line), &parsed); err == nil {
					nodes = append(nodes, parsed...)
				}
			case "defs":
				var parsed []wakeUpCourseDef
				if err := json.Unmarshal([]byte(line), &parsed); err == nil {
					for _, def := range parsed {
						if defs[def.TableID] == nil {
							defs[def.TableID] = map[int]string{}
						}
						defs[def.TableID][def.ID] = def.CourseName
					}
				}
			case "courses":
				var parsed []WakeUpCourse
				if err := json.Unmarshal([]byte(line), &parsed); err == nil {
					courses = append(courses, parsed...)
				}
			}
			continue
		}
		var probe map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			continue
		}
		switch {
		case probe["startDate"] != nil || probe["maxWeek"] != nil:
			var table WakeUpTable
			if err := json.Unmarshal([]byte(line), &table); err != nil {
				continue
			}
			share.tables[table.ID] = table
			if !tableSet {
				share.Table = table
				tableSet = true
			}
		// Legacy flat payloads put the name and the row in a single object.
		case probe["courseName"] != nil || probe["startNode"] != nil:
			var course WakeUpCourse
			if err := json.Unmarshal([]byte(line), &course); err == nil {
				courses = append(courses, course)
			}
		}
	}

	share.Nodes = nodes
	for _, node := range nodes {
		share.nodesByTable[node.TimeTable] = append(share.nodesByTable[node.TimeTable], node)
	}
	for i := range courses {
		course := &courses[i]
		if strings.TrimSpace(course.CourseName) == "" && strings.TrimSpace(course.Name) == "" {
			if names := defs[course.TableID]; names != nil {
				course.CourseName = names[course.ID]
			}
		}
	}
	share.Courses = courses

	if len(share.Nodes) == 0 {
		return nil, fmt.Errorf("WakeUp 分享数据缺少作息时间")
	}
	return share, nil
}

// classifyWakeUpArray inspects the first element of a JSON array line to tell
// the three arrays in a real payload apart.
func classifyWakeUpArray(line string) string {
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(line), &items); err != nil || len(items) == 0 {
		return ""
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(items[0], &probe); err != nil {
		return ""
	}
	switch {
	case probe["node"] != nil:
		return "nodes"
	case probe["startNode"] != nil || probe["day"] != nil:
		return "courses"
	case probe["courseName"] != nil:
		return "defs"
	}
	return ""
}

// tableForCourse returns the课程表 metadata a course row belongs to, falling
// back to the primary table when the payload does not carry table ids.
func (s *WakeUpShare) tableForCourse(course WakeUpCourse) WakeUpTable {
	if table, ok := s.tables[course.TableID]; ok {
		return table
	}
	return s.Table
}

// nodesForCourse resolves the作息表 periods for a course. A payload without
// per-table grouping (legacy tests, hand-written data) falls back to the flat
// node list.
func (s *WakeUpShare) nodesForCourse(course WakeUpCourse) []WakeUpNode {
	if table, ok := s.tables[course.TableID]; ok {
		if nodes := s.nodesByTable[table.TimeTable]; len(nodes) > 0 {
			return nodes
		}
	}
	return s.Nodes
}

// Events converts the share payload into canonical course events.
//
// WakeUp numbers weekdays 1..7 as Monday..Sunday and stores the term start as
// the Monday of week 1, so the first meeting of a course is
// startDate + (firstWeek-1)*7 + (day-1) days. Recurrence is one weekly RRULE
// with COUNT equal to the number of weeks the course spans; odd/even courses
// use INTERVAL=2 from their first matching week.
func (s *WakeUpShare) Events() ([]Event, error) {
	var events []Event
	for _, course := range s.Courses {
		name := firstNonEmpty(strings.TrimSpace(course.CourseName), strings.TrimSpace(course.Name))
		if name == "" || course.Day < 1 || course.Day > 7 {
			continue
		}
		table := s.tableForCourse(course)
		if strings.TrimSpace(table.StartDate) == "" {
			continue
		}
		startDate, err := parseWakeUpDate(table.StartDate)
		if err != nil {
			return nil, err
		}

		startTime, endTime, ok := s.courseTimes(course)
		if !ok {
			continue
		}

		startWeek := max(course.StartWeek, 1)
		endWeek := course.EndWeek
		if endWeek < startWeek {
			endWeek = startWeek
		}
		if table.MaxWeek > 0 {
			if startWeek > table.MaxWeek {
				continue
			}
			if endWeek > table.MaxWeek {
				endWeek = table.MaxWeek
			}
		}
		interval := 1
		switch course.Type {
		case 1: // 单周
			if startWeek%2 == 0 {
				startWeek++
			}
			interval = 2
		case 2: // 双周
			if startWeek%2 == 1 {
				startWeek++
			}
			interval = 2
		}
		if startWeek > endWeek {
			continue
		}
		count := endWeek - startWeek + 1
		if interval == 2 {
			count = (endWeek-startWeek)/2 + 1
		}

		firstDay := startDate.AddDate(0, 0, (startWeek-1)*7+(course.Day-1))
		dtstart, startOK := wakeUpDateTime(firstDay, startTime)
		dtend, endOK := wakeUpDateTime(firstDay, endTime)
		if !startOK || !endOK {
			continue
		}
		rrule := fmt.Sprintf("FREQ=WEEKLY;COUNT=%d;WKST=MO", count)
		if interval == 2 {
			rrule = fmt.Sprintf("FREQ=WEEKLY;INTERVAL=2;COUNT=%d;WKST=MO", count)
		}
		event := Event{
			"UID":     wakeUpEventUID(name, course, firstDay),
			"SUMMARY": name,
			"DTSTART": dtstart.Format("20060102T150405"),
			"DTEND":   dtend.Format("20060102T150405"),
			"DTSTAMP": time.Now().UTC().Format("20060102T150405Z"),
			"RRULE":   rrule,
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

// courseTimes returns the start/end clock for a course, using the custom times
// of an ownTime row or walking the covered periods otherwise.
func (s *WakeUpShare) courseTimes(course WakeUpCourse) (string, string, bool) {
	if course.OwnTime && strings.TrimSpace(course.StartTime) != "" && strings.TrimSpace(course.EndTime) != "" {
		return course.StartTime, course.EndTime, true
	}
	nodes := make(map[int]WakeUpNode)
	for _, node := range s.nodesForCourse(course) {
		nodes[node.Node] = node
	}
	start, ok := nodes[course.StartNode]
	if !ok {
		return "", "", false
	}
	// Walk the covered nodes and use the last one that exists, so a step past
	// the timetable edge (or a gap) degrades to the previous period instead of
	// overflowing.
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
	return start.StartTime, end.EndTime, true
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
	return fmt.Sprintf("wakeup-%s-%d-%d-%d-%d-%d",
		strings.ReplaceAll(name, " ", "_"),
		course.Day, course.StartNode, course.StartWeek, course.Type, day.Unix())
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
