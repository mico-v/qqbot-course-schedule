// Package schedule holds the course schedule domain logic: iCalendar parsing,
// recurrence expansion, Chinese date parsing, day cards and the service layer.
// It has no dependency on the bot framework or the web layer.
package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Day override kinds.
const (
	DayOverrideHoliday = "holiday"
	DayOverrideShift   = "shift"
	// DayOverrideAll marks an override that applies to every member.
	DayOverrideAll = "*"
)

// Limits inherited from the AstrBot plugin.
const (
	MaxICSBytes             = 2 << 20
	MaxEventsPerFile        = 120
	MaxMembersPerCreate     = 200
	MaxDayOverridesPerScope = 1000
	MaxDayOverrideRangeDays = 180
	MaxCourseNameLength     = 200
	MaxDescriptionLength    = 2000
	MaxRRuleLength          = 500
	MaxMemberNameLength     = 200
)

// ErrConflict is returned when an optimistic-lock write loses a race.
var ErrConflict = errors.New("课表已被其他操作更新")

// LocalTZ is the single timezone used for parsing, display and day boundaries.
var LocalTZ = mustZone("Asia/Shanghai")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// time/tzdata is embedded by the binary; this is a programming error.
		panic(fmt.Sprintf("加载时区 %s 失败: %v", name, err))
	}
	return loc
}

// Event is the canonical event form: UPPERCASE iCalendar keys plus RAW_ICAL.
type Event map[string]string

// DayOverride is one holiday/shift marker for a member on one day.
type DayOverride struct {
	Kind      string `json:"kind"`
	SourceDay string `json:"source_day"`
}

// ImportSource identifies where one personal schedule was imported from.
// UserOpenID is the platform's local member_openid in groups (or user_openid
// in private chat). Production has returned the same member_openid across
// groups, so it is also a global-lookup fallback, not a QQ number. UnionOpenID
// is the optional cross-application identity and wins when present.
type ImportSource struct {
	GroupOpenID string
	UserOpenID  string
	UnionOpenID string
}

// MemberLocation is one persisted member coordinate.
type MemberLocation struct {
	ScopeID string
	UserID  string
}

// Member is one person's schedule inside a scope.
type Member struct {
	UserID string
	Name   string
	// QQ is an optional self-reported QQ number used for avatar lookups.
	QQ                string
	Events            []Event
	ICS               string
	Schedule          string
	Source            string
	EventCount        int
	UpdatedAt         string
	ScheduleUpdatedAt string
	LastModifiedAt    string
	LastModifiedBy    string
	SourceFile        string
	UploaderID        string
	ImportGroupOpenID string
	ImportUserOpenID  string
	ImportUnionOpenID string
	Revision          int64
	// DayOverrides is attached on read, merged from scope-wide and member rows.
	DayOverrides map[string]DayOverride
}

// ServiceStorage is the explicit repository set used by domain services.
type ServiceStorage interface {
	GroupMemberRepository
	GroupScheduleRepository
	DayOverrideRepository
	SettingsStore
	EditLinkStore
	StatsRepository
}

// KVEntry is one namespaced key/value pair.
type KVEntry struct {
	Key   string
	Value []byte
}

// DayOverrideRow is one stored marker.
type DayOverrideRow struct {
	ScopeID   string `json:"scope_id"`
	UserID    string `json:"user_id"`
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	SourceDay string `json:"source_day"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// ScopeGroup builds the group scope id.
func ScopeGroup(groupOpenID string) string { return "group:" + groupOpenID }

// ScopePrivate builds the private-chat scope id.
func ScopePrivate(userOpenID string) string { return "private:" + userOpenID }

// ScopeGlobalUser builds the shared personal-schedule scope id.
func ScopeGlobalUser(globalUserOpenID string) string { return "global-user:" + globalUserOpenID }

// ParseScope splits a scope id into its kind and id.
func ParseScope(scopeID string) (kind, targetID string) {
	if kind, targetID, ok := strings.Cut(scopeID, ":"); ok {
		return kind, targetID
	}
	return "other", scopeID
}

// NowISO returns the UTC bookkeeping timestamp.
func NowISO() string { return time.Now().UTC().Format(time.RFC3339Nano) }
