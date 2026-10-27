package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const rosterMemberLimit = 500

// GroupRosterMember is one member snapshot returned by the platform.
type GroupRosterMember struct {
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	UnionOpenID string `json:"union_openid,omitempty"`
}

// GroupRoster is the latest complete member snapshot for one group.
type GroupRoster struct {
	Members   []GroupRosterMember `json:"members"`
	UpdatedAt string              `json:"updated_at"`
}

// SaveGroupRoster atomically replaces one group's roster state in
// group_members.
func (s *Service) SaveGroupRoster(scopeID string, members []GroupRosterMember) error {
	return s.SaveGroupRosterContext(context.Background(), scopeID, members)
}

// SaveGroupRosterContext atomically replaces roster state with cancellation.
func (s *Service) SaveGroupRosterContext(ctx context.Context, scopeID string, members []GroupRosterMember) error {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(scopeID))
	if err != nil || ref.Kind != "group" {
		return ValidationErrorf("无效的群聊作用域")
	}
	now := time.Now().UTC()
	observations := make([]GroupMemberObservation, 0, min(len(members), rosterMemberLimit))
	seen := make(map[string]bool)
	for _, member := range members {
		memberID := strings.TrimSpace(member.UserID)
		if memberID == "" || seen[memberID] {
			continue
		}
		if len(observations) >= rosterMemberLimit {
			break
		}
		seen[memberID] = true
		observations = append(observations, GroupMemberObservation{
			Key: GroupMemberKey{
				GroupOpenID:  ref.ID,
				MemberOpenID: memberID,
			},
			DisplayName: firstNonEmpty(member.Name, memberID),
			UnionOpenID: strings.TrimSpace(member.UnionOpenID),
			ObservedAt:  now,
			InRoster:    true,
		})
	}
	if err := s.members.ReplaceGroupRoster(
		ctx,
		GroupKey{GroupOpenID: ref.ID},
		observations,
	); err != nil {
		return fmt.Errorf("保存群成员快照失败：%w", err)
	}
	return nil
}

// GroupRoster returns members currently marked as part of the roster.
func (s *Service) GroupRoster(scopeID string) (GroupRoster, bool, error) {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(scopeID))
	if err != nil || ref.Kind != "group" {
		return GroupRoster{}, false, ValidationErrorf("无效的群聊作用域")
	}
	members, err := s.members.ListGroupMembers(
		context.Background(),
		GroupKey{GroupOpenID: ref.ID},
	)
	if err != nil {
		return GroupRoster{}, false, fmt.Errorf("读取群成员快照失败：%w", err)
	}
	roster := GroupRoster{}
	var updatedAt time.Time
	for _, member := range members {
		if !member.InRoster {
			continue
		}
		roster.Members = append(roster.Members, GroupRosterMember{
			UserID:      member.Key.MemberOpenID,
			Name:        firstNonEmpty(member.DisplayName, member.Key.MemberOpenID),
			UnionOpenID: member.UnionOpenID,
		})
		if member.LastRosterAt.After(updatedAt) {
			updatedAt = member.LastRosterAt
		}
	}
	if len(roster.Members) == 0 {
		return GroupRoster{}, false, nil
	}
	roster.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return roster, true, nil
}
