package schedule

import (
	"context"
	"strings"
	"time"
)

// RecordIdentity stores union_openid as an optional group member attribute.
// It never creates a cross-group identity or redirects schedule storage.
func (s *Service) RecordIdentity(scopeID, userID, unionOpenID string) error {
	return s.RecordIdentityContext(context.Background(), scopeID, userID, unionOpenID)
}

// RecordIdentityContext stores the optional union attribute with cancellation.
func (s *Service) RecordIdentityContext(ctx context.Context, scopeID, userID, unionOpenID string) error {
	unionOpenID = strings.TrimSpace(unionOpenID)
	if unionOpenID == "" {
		return nil
	}
	key, err := GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return err
	}
	return s.members.RecordGroupMemberSeen(ctx, GroupMemberObservation{
		Key:         key,
		UnionOpenID: unionOpenID,
		ObservedAt:  time.Now().UTC(),
	})
}

// ResolveIdentity returns the union_openid attribute for one exact group
// member. A missing union value is expected and is not an error.
func (s *Service) ResolveIdentity(scopeID, userID string) (string, bool, error) {
	key, err := GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return "", false, err
	}
	member, found, err := s.members.GetGroupMember(context.Background(), key)
	if err != nil || !found {
		return "", false, err
	}
	unionOpenID := strings.TrimSpace(member.UnionOpenID)
	return unionOpenID, unionOpenID != "", nil
}

// ScopeIdentities returns the known union attributes inside one group.
func (s *Service) ScopeIdentities(scopeID string) (map[string]string, error) {
	ref, err := ParseLegacyScopeRef(scopeID)
	if err != nil {
		return nil, err
	}
	if ref.Kind != "group" {
		return nil, ValidationErrorf("仅支持群聊 scope")
	}
	members, err := s.members.ListGroupMembers(
		context.Background(),
		GroupKey{GroupOpenID: ref.ID},
	)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, member := range members {
		if unionOpenID := strings.TrimSpace(member.UnionOpenID); unionOpenID != "" {
			result[member.Key.MemberOpenID] = unionOpenID
		}
	}
	return result, nil
}
