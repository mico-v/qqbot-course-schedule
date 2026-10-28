package schedule

import (
	"context"
	"time"
)

// GroupMemberRepository owns group member profiles, roster and seen state.
type GroupMemberRepository interface {
	GetGroupMember(ctx context.Context, key GroupMemberKey) (GroupMember, bool, error)
	ListGroupMembers(ctx context.Context, group GroupKey) ([]GroupMember, error)
	RecordGroupMemberSeen(ctx context.Context, observation GroupMemberObservation) error
	ReplaceGroupRoster(ctx context.Context, group GroupKey, members []GroupMemberObservation) error
}

// GroupScheduleRepository owns member schedules and revision semantics.
type GroupScheduleRepository interface {
	GetGroupMemberSchedule(ctx context.Context, key GroupMemberKey) (*Member, bool, error)
	ListGroupSchedules(ctx context.Context, group GroupKey) (map[string]*Member, error)
	PutGroupMemberSchedule(
		ctx context.Context,
		key GroupMemberKey,
		member *Member,
		expected Revision,
	) (Revision, error)
}

// GlobalScheduleRepository provides explicit cross-group lookup and schedule
// deletion for global mode. It never changes the group-member identity key.
type GlobalScheduleRepository interface {
	FindGlobalSchedule(
		ctx context.Context,
		memberOpenID string,
		unionOpenID string,
		exclude *GroupMemberKey,
	) (*Member, StorageTarget, bool, error)
	DeleteMemberSchedules(
		ctx context.Context,
		target StorageTarget,
		expected Revision,
		currentKey GroupMemberKey,
		unionOpenID string,
		global bool,
	) (int, error)
}

// DayOverrideRepository owns group-wide and member-specific day markers.
type DayOverrideRepository interface {
	ListGroupDayOverrides(ctx context.Context, group GroupKey) ([]DayOverrideRow, error)
	SetGroupDayOverride(ctx context.Context, row DayOverrideRow) error
	DeleteGroupDayOverride(ctx context.Context, group GroupKey, memberOpenID, day string) (bool, error)
	DeleteAllGroupDayOverrides(ctx context.Context, group GroupKey) error
}

// SettingsStore persists the single bot settings aggregate.
type SettingsStore interface {
	LoadBotSettings(ctx context.Context) (Settings, bool, error)
	SaveBotSettings(ctx context.Context, settings Settings) error
}

// EditLinkRecord is the durable, hashed edit-link representation.
type EditLinkRecord struct {
	TokenHash    string
	GroupOpenID  string
	MemberOpenID string
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// EditLinkStore persists hashed schedule edit tokens.
type EditLinkStore interface {
	SaveEditLink(ctx context.Context, record EditLinkRecord) error
	GetEditLink(ctx context.Context, tokenHash string) (EditLinkRecord, bool, error)
	DeleteEditLink(ctx context.Context, tokenHash string) error
}

// StatsRepository persists append-only message timing records.
type StatsRepository interface {
	InsertMessageStats(ctx context.Context, record MessageStats) error
	ListMessageStats(ctx context.Context, since time.Time, group *GroupKey) ([]MessageStats, error)
	PruneMessageStats(ctx context.Context, before time.Time) (int, error)
}

// LegacyRepository exposes v6-only data to read/export compatibility paths.
// Group use cases must not depend on this interface.
type LegacyRepository interface {
	ListLegacyScopes(ctx context.Context) ([]string, error)
	GetLegacyMember(ctx context.Context, scopeID, userID string) (*Member, bool, error)
}
