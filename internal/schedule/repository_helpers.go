package schedule

import (
	"context"
	"strings"
	"time"
)

func (s *Service) getMember(scopeID, memberOpenID string) (*Member, bool, error) {
	return s.getMemberContext(context.Background(), scopeID, memberOpenID)
}

func (s *Service) getMemberContext(ctx context.Context, scopeID, memberOpenID string) (*Member, bool, error) {
	key, err := GroupMemberKeyFromLegacy(scopeID, memberOpenID)
	if err != nil {
		return nil, false, err
	}
	return s.schedules.GetGroupMemberSchedule(ctx, key)
}

func (s *Service) listMembers(scopeID string) (map[string]*Member, error) {
	return s.listMembersContext(context.Background(), scopeID)
}

func (s *Service) listMembersContext(ctx context.Context, scopeID string) (map[string]*Member, error) {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(scopeID))
	if err != nil {
		return nil, err
	}
	if ref.Kind != "group" {
		return nil, ValidationErrorf("仅支持群聊 scope")
	}
	return s.schedules.ListGroupSchedules(
		ctx,
		GroupKey{GroupOpenID: ref.ID},
	)
}

func (s *Service) putMember(scopeID, memberOpenID string, member *Member, expected int64) error {
	return s.putMemberContext(context.Background(), scopeID, memberOpenID, member, expected)
}

func (s *Service) putMemberContext(ctx context.Context, scopeID, memberOpenID string, member *Member, expected int64) error {
	key, err := GroupMemberKeyFromLegacy(scopeID, memberOpenID)
	if err != nil {
		return err
	}
	_, err = s.schedules.PutGroupMemberSchedule(
		ctx,
		key,
		member,
		Revision(expected),
	)
	return err
}

func (s *Service) listDayOverrides(scopeID string) ([]DayOverrideRow, error) {
	return s.listDayOverridesContext(context.Background(), scopeID)
}

func (s *Service) listDayOverridesContext(ctx context.Context, scopeID string) ([]DayOverrideRow, error) {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(scopeID))
	if err != nil {
		return nil, err
	}
	if ref.Kind != "group" {
		return nil, ValidationErrorf("仅支持群聊 scope")
	}
	return s.overrides.ListGroupDayOverrides(
		ctx,
		GroupKey{GroupOpenID: ref.ID},
	)
}

func (s *Service) setDayOverride(scopeID, memberOpenID, day string, override DayOverride, actor, createdAt string) error {
	return s.setDayOverrideContext(context.Background(), scopeID, memberOpenID, day, override, actor, createdAt)
}

func (s *Service) setDayOverrideContext(ctx context.Context, scopeID, memberOpenID, day string, override DayOverride, actor, createdAt string) error {
	return s.overrides.SetGroupDayOverride(ctx, DayOverrideRow{
		ScopeID: scopeID, UserID: memberOpenID, Day: day, Kind: override.Kind,
		SourceDay: override.SourceDay, CreatedBy: actor, CreatedAt: createdAt,
	})
}

func (s *Service) deleteDayOverride(scopeID, memberOpenID, day string) (bool, error) {
	return s.deleteDayOverrideContext(context.Background(), scopeID, memberOpenID, day)
}

func (s *Service) deleteDayOverrideContext(ctx context.Context, scopeID, memberOpenID, day string) (bool, error) {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(scopeID))
	if err != nil {
		return false, err
	}
	if ref.Kind != "group" {
		return false, ValidationErrorf("仅支持群聊 scope")
	}
	return s.overrides.DeleteGroupDayOverride(
		ctx,
		GroupKey{GroupOpenID: ref.ID},
		memberOpenID,
		day,
	)
}

func (s *Service) messageStatsSince(since time.Time, scopeID string) ([]MessageStats, error) {
	return s.messageStatsSinceContext(context.Background(), since, scopeID)
}

func (s *Service) messageStatsSinceContext(ctx context.Context, since time.Time, scopeID string) ([]MessageStats, error) {
	var group *GroupKey
	if strings.TrimSpace(scopeID) != "" {
		ref, err := ParseLegacyScopeRef(scopeID)
		if err != nil {
			return nil, err
		}
		if ref.Kind != "group" {
			return nil, ValidationErrorf("仅支持群聊 scope")
		}
		group = &GroupKey{GroupOpenID: ref.ID}
	}
	return s.stats.ListMessageStats(ctx, since, group)
}
