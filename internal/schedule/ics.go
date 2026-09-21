package schedule

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	rrulelib "github.com/teambition/rrule-go"
)

// icsProperty is one unfolded iCalendar property line.
type icsProperty struct {
	Name   string
	Params map[string]string
	Value  string
	Raw    string
}

// unfoldICS splits an iCalendar text into unfolded logical lines.
func unfoldICS(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	rawLines := strings.Split(content, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// findValueSeparator returns the index of the ':' that separates name/params
// from the value, ignoring colons inside quoted parameter values.
func findValueSeparator(line string) int {
	quoted := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			quoted = !quoted
		case ':':
			if !quoted {
				return i
			}
		}
	}
	return -1
}

func parsePropertyLine(line string) (icsProperty, bool) {
	sep := findValueSeparator(line)
	if sep < 0 {
		return icsProperty{}, false
	}
	head, value := line[:sep], line[sep+1:]
	parts := splitUnquoted(head, ';')
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return icsProperty{}, false
	}
	prop := icsProperty{
		Name:   strings.ToUpper(strings.TrimSpace(parts[0])),
		Params: make(map[string]string, len(parts)-1),
		Value:  value,
		Raw:    line,
	}
	for _, part := range parts[1:] {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		prop.Params[strings.ToUpper(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(val), `"`)
	}
	return prop, true
}

func splitUnquoted(value string, separator byte) []string {
	var parts []string
	var current strings.Builder
	quoted := false
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"':
			quoted = !quoted
			current.WriteByte(value[i])
		case separator:
			if quoted {
				current.WriteByte(value[i])
				continue
			}
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(value[i])
		}
	}
	parts = append(parts, current.String())
	return parts
}

// icsComponent is a BEGIN/END block from an iCalendar text.
type icsComponent struct {
	Name  string
	Lines []string
}

// extractComponents returns every component block in document order, including
// nested ones (a VEVENT usually contains a VALARM).
func extractComponents(content string) []icsComponent {
	lines := unfoldICS(content)
	var components []icsComponent
	var stack []*icsComponent
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "BEGIN:"):
			name := strings.ToUpper(strings.TrimSpace(trimmed[len("BEGIN:"):]))
			stack = append(stack, &icsComponent{Name: name, Lines: []string{line}})
		case strings.HasPrefix(upper, "END:"):
			if len(stack) == 0 {
				continue
			}
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			current.Lines = append(current.Lines, line)
			components = append(components, *current)
			// Keep nested components inside their parent's raw text so a
			// VEVENT's RAW_ICAL still contains its VALARM block.
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Lines = append(parent.Lines, current.Lines...)
			}
		default:
			if len(stack) > 0 {
				stack[len(stack)-1].Lines = append(stack[len(stack)-1].Lines, line)
			}
		}
	}
	return components
}

func componentRaw(component icsComponent) string {
	return strings.Join(component.Lines, "\r\n") + "\r\n"
}

// unescapeText decodes iCalendar TEXT escapes.
func unescapeText(value string) string {
	replacer := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return replacer.Replace(value)
}

// escapeText encodes a value as iCalendar TEXT.
func escapeText(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return replacer.Replace(value)
}

// ParseICSTime parses an iCalendar date/time value in the given timezone.
func ParseICSTime(value, tzid string) (time.Time, bool) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return time.Time{}, false
	}
	zone := LocalTZ
	if tzid != "" {
		if loaded, err := time.LoadLocation(tzid); err == nil {
			zone = loaded
		} else if loaded, err := time.LoadLocation(strings.TrimPrefix(tzid, "/")); err == nil {
			// Some exporters write TZID=/Asia/Shanghai with a leading slash.
			zone = loaded
		}
	}
	if len(raw) == 8 && isDigits(raw) {
		parsed, err := time.ParseInLocation("20060102", raw, zone)
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	}
	if strings.HasSuffix(raw, "Z") {
		for _, layout := range []string{"20060102T150405Z", "20060102T1504Z"} {
			if parsed, err := time.Parse(layout, raw); err == nil {
				return parsed, true
			}
		}
		return time.Time{}, false
	}
	for _, layout := range []string{"20060102T150405", "20060102T1504"} {
		if parsed, err := time.ParseInLocation(layout, raw, zone); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func isDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

// FormatICSDateTime renders a stored value for display in local time.
func FormatICSDateTime(value, tzid string) string {
	parsed, ok := ParseICSTime(value, tzid)
	if !ok {
		return strings.TrimSpace(value)
	}
	return parsed.In(LocalTZ).Format("2006-01-02 15:04")
}

// FormatRRULE renders an RRULE as short Chinese text.
func FormatRRULE(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := map[string]string{}
	for _, field := range strings.Split(value, ";") {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		parts[strings.ToUpper(strings.TrimSpace(key))] = strings.TrimSpace(val)
	}
	text := map[string]string{
		"DAILY": "每天", "WEEKLY": "每周", "MONTHLY": "每月", "YEARLY": "每年",
	}[parts["FREQ"]]
	if text == "" {
		text = parts["FREQ"]
	}
	if byday := parts["BYDAY"]; byday != "" {
		text += " " + byday
	}
	if count := parts["COUNT"]; count != "" {
		text += fmt.Sprintf(" 共 %s 次", count)
	}
	return strings.TrimSpace(text)
}

// FormatICSSchedule renders events as the numbered text kept in the member row.
func FormatICSSchedule(events []Event) string {
	if len(events) == 0 {
		return "未解析到课程事件。"
	}
	lines := make([]string, 0, len(events))
	for index, event := range events {
		start := FormatICSDateTime(event["DTSTART"], event["DTSTART_TZID"])
		end := FormatICSDateTime(event["DTEND"], event["DTEND_TZID"])
		line := fmt.Sprintf("%d. %s", index+1, firstNonEmpty(event["SUMMARY"], "未命名课程"))
		if start != "" || end != "" {
			line += " | " + start
			if start != "" && end != "" {
				line += " - "
			}
			line += end
		}
		if rrule := FormatRRULE(event["RRULE"]); rrule != "" {
			line += " | " + rrule
		}
		if location := strings.TrimSpace(event["LOCATION"]); location != "" {
			line += " | " + location
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// ParseICSEvents parses every VEVENT of an iCalendar text.
func ParseICSEvents(content string) ([]Event, error) {
	raw := content
	content = DecodeICSBytes([]byte(raw))
	if !strings.Contains(strings.ToUpper(content), "BEGIN:VCALENDAR") {
		// Sniff the original bytes for binary formats, then the decoded text.
		hint := sniffICSContent(raw)
		if strings.HasPrefix(hint, "（文件开头") && content != raw {
			hint = sniffICSContent(content)
		}
		return nil, fmt.Errorf("不是有效的 iCalendar：缺少 VCALENDAR%s", hint)
	}
	components := extractComponents(content)
	var events []Event
	for _, component := range components {
		if component.Name != "VEVENT" {
			continue
		}
		events = append(events, componentToEvent(component))
	}
	if len(events) > MaxEventsPerFile {
		return nil, fmt.Errorf("VEVENT 数量超过上限 %d", MaxEventsPerFile)
	}
	sortEvents(events)
	return events, nil
}

func componentToEvent(component icsComponent) Event {
	event := Event{"RAW_ICAL": componentRaw(component)}
	depth := 0
	for _, line := range component.Lines {
		prop, ok := parsePropertyLine(line)
		if !ok {
			continue
		}
		if prop.Name == "BEGIN" && !strings.EqualFold(strings.TrimSpace(prop.Value), "VEVENT") {
			depth++
			continue
		}
		if prop.Name == "END" && depth > 0 {
			depth--
			continue
		}
		if depth > 0 {
			// Nested component (VALARM): keep it in RAW_ICAL, do not let its
			// DESCRIPTION/SUMMARY shadow the event's own properties.
			continue
		}
		switch prop.Name {
		case "SUMMARY", "LOCATION", "DESCRIPTION", "UID":
			if value := strings.TrimSpace(unescapeText(prop.Value)); value != "" {
				event[prop.Name] = value
			}
		case "DTSTART", "DTEND", "DTSTAMP":
			value := strings.TrimSpace(prop.Value)
			if value == "" {
				continue
			}
			event[prop.Name] = value
			if tzid := prop.Params["TZID"]; tzid != "" {
				event[prop.Name+"_TZID"] = tzid
			}
		case "RRULE":
			if value := strings.TrimSpace(prop.Value); value != "" {
				event["RRULE"] = value
			}
		}
	}
	return event
}

func sortEvents(events []Event) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0; j-- {
			left, right := events[j-1], events[j]
			if left["DTSTART"] < right["DTSTART"] ||
				(left["DTSTART"] == right["DTSTART"] && left["DTEND"] <= right["DTEND"]) {
				break
			}
			events[j-1], events[j] = events[j], events[j-1]
		}
	}
}

// ParseICSEventsAndSchedule parses an ICS file and builds the schedule text.
func ParseICSEventsAndSchedule(content string) ([]Event, string, error) {
	events, err := ParseICSEvents(content)
	if err != nil {
		return nil, "", err
	}
	return events, FormatICSSchedule(events), nil
}

// NormalizeDateTime converts ISO/iCalendar input to local iCalendar text.
func NormalizeDateTime(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", fmt.Errorf("时间不能为空")
	}
	if parsed, ok := ParseICSTime(raw, ""); ok {
		return parsed.In(LocalTZ).Format("20060102T150405"), nil
	}
	layouts := []string{
		"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02 15:04:05",
		"2006-01-02T15:04:05", "2006/01/02 15:04", "2006-01-02", time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, LocalTZ); err == nil {
			return parsed.Format("20060102T150405"), nil
		}
	}
	return "", fmt.Errorf("无法解析时间“%s”，请使用 YYYY-MM-DD HH:MM 或 iCalendar 格式。", raw)
}

// MakeEvent builds a canonical event from user input.
func MakeEvent(course, startTime, endTime, location, description, rrule, uid string) (Event, error) {
	summary := strings.TrimSpace(course)
	if summary == "" {
		return nil, fmt.Errorf("课程名称不能为空")
	}
	start, err := NormalizeDateTime(startTime)
	if err != nil {
		return nil, err
	}
	end, err := NormalizeDateTime(endTime)
	if err != nil {
		return nil, err
	}
	startAt, startOK := ParseICSTime(start, "")
	endAt, endOK := ParseICSTime(end, "")
	if !startOK || !endOK || !endAt.After(startAt) {
		return nil, fmt.Errorf("结束时间必须晚于开始时间")
	}
	if uid == "" {
		uid = randomUID()
	}
	event := Event{
		"UID":     uid,
		"SUMMARY": summary,
		"DTSTART": start,
		"DTEND":   end,
		"DTSTAMP": time.Now().UTC().Format("20060102T150405Z"),
	}
	if location = strings.TrimSpace(location); location != "" {
		event["LOCATION"] = location
	}
	if description = strings.TrimSpace(description); description != "" {
		event["DESCRIPTION"] = description
	}
	if rrule = strings.TrimSpace(rrule); rrule != "" {
		if _, err := rrulelib.StrToRRule(rrule); err != nil {
			return nil, fmt.Errorf("重复规则无法解析：%s", rrule)
		}
		if !strings.Contains(strings.ToUpper(rrule), "FREQ=") {
			return nil, fmt.Errorf("重复规则必须包含 FREQ，例如 FREQ=WEEKLY;BYDAY=MO")
		}
		event["RRULE"] = rrule
	}
	return event, nil
}

func randomUID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d@qqbot-course-schedule", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf) + "@qqbot-course-schedule"
}

// knownEventProperties are rebuilt from canonical fields when serializing.
var knownEventProperties = map[string]bool{
	"UID": true, "DTSTAMP": true, "DTSTART": true, "DTEND": true,
	"RRULE": true, "SUMMARY": true, "LOCATION": true, "DESCRIPTION": true,
	"BEGIN": true, "END": true,
}

// SerializeScheduleICS rebuilds a calendar, keeping non-VEVENT components from
// the base text and unknown properties from each event's RAW_ICAL.
func SerializeScheduleICS(events []Event, baseICS, calendarName string) string {
	var lines []string
	lines = append(lines, "BEGIN:VCALENDAR", "VERSION:2.0",
		"PRODID:-//qqbot-course-schedule//CN", "CALSCALE:GREGORIAN")
	if calendarName != "" {
		lines = append(lines, "X-WR-CALNAME:"+escapeText(calendarName))
	}
	if baseICS != "" {
		for _, component := range extractComponents(baseICS) {
			if component.Name == "VCALENDAR" || component.Name == "VEVENT" {
				continue
			}
			lines = append(lines, component.Lines...)
		}
	}
	for _, event := range events {
		lines = append(lines, eventComponentLines(event)...)
	}
	lines = append(lines, "END:VCALENDAR")

	var out strings.Builder
	for _, line := range lines {
		for _, folded := range foldICalLine(line) {
			out.WriteString(folded)
			out.WriteString("\r\n")
		}
	}
	return out.String()
}

func eventComponentLines(event Event) []string {
	lines := []string{"BEGIN:VEVENT"}
	raw := unfoldICS(event["RAW_ICAL"])
	for index := 0; index < len(raw); index++ {
		line := raw[index]
		prop, ok := parsePropertyLine(line)
		if !ok {
			continue
		}
		if prop.Name == "BEGIN" {
			value := strings.ToUpper(strings.TrimSpace(prop.Value))
			if value == "VEVENT" {
				continue
			}
			// Preserve nested components (VALARM and friends) verbatim.
			end := "END:" + value
			lines = append(lines, line)
			for index++; index < len(raw); index++ {
				lines = append(lines, raw[index])
				if strings.EqualFold(strings.TrimSpace(raw[index]), end) {
					break
				}
			}
			continue
		}
		if prop.Name == "END" || knownEventProperties[prop.Name] {
			continue
		}
		lines = append(lines, line)
	}
	uid := strings.TrimSpace(event["UID"])
	if uid == "" {
		uid = randomUID()
	}
	lines = append(lines, "UID:"+escapeText(uid))
	dtstamp := strings.TrimSpace(event["DTSTAMP"])
	if dtstamp == "" {
		dtstamp = time.Now().UTC().Format("20060102T150405Z")
	}
	lines = append(lines, "DTSTAMP:"+dtstamp)
	lines = append(lines, datetimePropertyLines("DTSTART", event)...)
	lines = append(lines, datetimePropertyLines("DTEND", event)...)
	if rrule := strings.TrimSpace(event["RRULE"]); rrule != "" {
		lines = append(lines, "RRULE:"+rrule)
	}
	for _, name := range []string{"SUMMARY", "LOCATION", "DESCRIPTION"} {
		if value := strings.TrimSpace(event[name]); value != "" {
			lines = append(lines, name+":"+escapeText(value))
		}
	}
	lines = append(lines, "END:VEVENT")
	return lines
}

func datetimePropertyLines(name string, event Event) []string {
	value := strings.TrimSpace(event[name])
	if value == "" {
		return nil
	}
	tzid := strings.TrimSpace(event[name+"_TZID"])
	if tzid != "" && !strings.HasSuffix(value, "Z") {
		return []string{fmt.Sprintf("%s;TZID=%s:%s", name, tzid, value)}
	}
	return []string{name + ":" + value}
}

// foldICalLine folds a logical line to 75 octets per physical line.
func foldICalLine(line string) []string {
	const limit = 75
	if len(line) <= limit {
		return []string{line}
	}
	var result []string
	remaining := line
	first := true
	for len(remaining) > 0 {
		width := limit
		if !first {
			width = limit - 1
		}
		if len(remaining) <= width {
			if first {
				result = append(result, remaining)
			} else {
				result = append(result, " "+remaining)
			}
			break
		}
		cut := width
		for cut > 0 && !utf8.RuneStart(remaining[cut]) {
			cut--
		}
		if cut == 0 {
			cut = width
		}
		if first {
			result = append(result, remaining[:cut])
			first = false
		} else {
			result = append(result, " "+remaining[:cut])
		}
		remaining = remaining[cut:]
	}
	return result
}

// recurrenceDates reads RDATE/EXDATE datetimes from an event's RAW_ICAL.
func recurrenceDates(event Event, name string) []time.Time {
	raw := event["RAW_ICAL"]
	if raw == "" {
		return nil
	}
	var dates []time.Time
	for _, line := range unfoldICS(raw) {
		prop, ok := parsePropertyLine(line)
		if !ok || prop.Name != name {
			continue
		}
		tzid := prop.Params["TZID"]
		for _, item := range strings.Split(prop.Value, ",") {
			if parsed, ok := ParseICSTime(strings.TrimSpace(item), tzid); ok {
				dates = append(dates, parsed.In(LocalTZ))
			}
		}
	}
	return dates
}

// EventDurationFromRaw reads DURATION from RAW_ICAL, if present.
func EventDurationFromRaw(event Event) (time.Duration, bool) {
	raw := event["RAW_ICAL"]
	if raw == "" {
		return 0, false
	}
	for _, line := range unfoldICS(raw) {
		prop, ok := parsePropertyLine(line)
		if !ok || prop.Name != "DURATION" {
			continue
		}
		if duration, ok := parseICalDuration(strings.TrimSpace(prop.Value)); ok {
			return duration, true
		}
	}
	return 0, false
}

// parseICalDuration understands the subset of RFC 5545 DURATION used by class
// schedules: PnW, PnDTnHnMnS and the T-only form.
func parseICalDuration(value string) (time.Duration, bool) {
	raw := strings.ToUpper(strings.TrimSpace(value))
	if raw == "" || raw[0] != 'P' {
		return 0, false
	}
	raw = raw[1:]
	var total time.Duration
	inTime := false
	number := ""
	apply := func(unit byte) bool {
		if number == "" {
			return false
		}
		amount, err := strconv.Atoi(number)
		if err != nil {
			return false
		}
		number = ""
		switch unit {
		case 'W':
			total += time.Duration(amount) * 7 * 24 * time.Hour
		case 'D':
			total += time.Duration(amount) * 24 * time.Hour
		case 'H':
			total += time.Duration(amount) * time.Hour
		case 'M':
			total += time.Duration(amount) * time.Minute
		case 'S':
			total += time.Duration(amount) * time.Second
		default:
			return false
		}
		return true
	}
	for i := 0; i < len(raw); i++ {
		char := raw[i]
		if char == 'T' {
			inTime = true
			continue
		}
		if char >= '0' && char <= '9' {
			number += string(char)
			continue
		}
		if !inTime && char != 'W' && char != 'D' {
			return 0, false
		}
		if !apply(char) {
			return 0, false
		}
	}
	return total, total > 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
