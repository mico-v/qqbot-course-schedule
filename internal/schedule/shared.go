package schedule

import (
	"context"
	"fmt"
	"strings"
)

// StorageTarget identifies one exact group member schedule. The legacy
// ScopeID/UserID fields remain for the HTTP compatibility boundary; Key is the
// authoritative domain coordinate.
type StorageTarget struct {
	Key         GroupMemberKey
	ScopeID     string
	UserID      string
	LocalUserID string
}

// NewStorageTarget builds a target from an already validated group member key.
func NewStorageTarget(key GroupMemberKey) StorageTarget {
	return StorageTarget{
		Key:         key,
		ScopeID:     key.Group().ScopeID(),
		UserID:      key.MemberOpenID,
		LocalUserID: key.MemberOpenID,
	}
}

// ResolveStorageTarget accepts only a group scope. union_openid has no role in
// coordinate resolution and cannot redirect a write to another group.
func (s *Service) ResolveStorageTarget(displayScopeID, localUserID, _ string) (StorageTarget, error) {
	key, err := GroupMemberKeyFromLegacy(displayScopeID, localUserID)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("无法识别要操作的成员: %w", err)
	}
	return NewStorageTarget(key), nil
}

// VisibleMember resolves one exact group member.
func (s *Service) VisibleMember(displayScopeID, localUserID, _ string) (*Member, bool, error) {
	key, err := GroupMemberKeyFromLegacy(displayScopeID, localUserID)
	if err != nil {
		return nil, false, err
	}
	return s.schedules.GetGroupMemberSchedule(context.Background(), key)
}

// VisibleMembers returns only schedules saved in the requested group.
func (s *Service) VisibleMembers(displayScopeID string) (map[string]*Member, error) {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(displayScopeID))
	if err != nil {
		return nil, err
	}
	if ref.Kind != "group" {
		return nil, ValidationErrorf("仅支持群聊 scope")
	}
	return s.listMembers(displayScopeID)
}

// SharedUserMembers remains as a group-only compatibility wrapper. extra
// union identities are deliberately ignored.
func (s *Service) SharedUserMembers(displayScopeID string, _ map[string]string) (map[string]*Member, error) {
	return s.VisibleMembers(displayScopeID)
}

func (s *Service) memberKnownInScope(scopeID, userID string) (bool, error) {
	_, found, err := s.getMember(scopeID, userID)
	if err != nil || found {
		return found, err
	}
	key, err := GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return false, err
	}
	_, found, err = s.members.GetGroupMember(context.Background(), key)
	return found, err
}

func mergeOverrides(scopeWide, member map[string]DayOverride) map[string]DayOverride {
	merged := make(map[string]DayOverride, len(scopeWide)+len(member))
	for day, override := range scopeWide {
		merged[day] = override
	}
	for day, override := range member {
		merged[day] = override
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}
