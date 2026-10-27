package schedule

import "time"

// GroupMember is the durable profile and observation state for one member in
// one group. Identity is always the embedded GroupMemberKey.
type GroupMember struct {
	Key          GroupMemberKey
	DisplayName  string
	QQ           string
	UnionOpenID  string
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	LastRosterAt time.Time
	InRoster     bool
}

// GroupMemberObservation is one seen or roster observation.
type GroupMemberObservation struct {
	Key         GroupMemberKey
	DisplayName string
	UnionOpenID string
	ObservedAt  time.Time
	InRoster    bool
}
