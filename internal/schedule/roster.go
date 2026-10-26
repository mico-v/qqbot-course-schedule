package schedule

import (
	"fmt"
	"strings"
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

// SaveGroupRoster replaces the cached member list for one group and remembers
// every union OpenID included in the snapshot.
func (s *Service) SaveGroupRoster(scopeID string, members []GroupRosterMember) error {
	scopeID = strings.TrimSpace(scopeID)
	kind, targetID := ParseScope(scopeID)
	if kind != "group" || strings.TrimSpace(targetID) == "" {
		return fmt.Errorf("无效的群聊作用域。")
	}

	deduplicated := make([]GroupRosterMember, 0, min(len(members), rosterMemberLimit))
	identities := make(map[string]string)
	seen := make(map[string]bool)
	for _, member := range members {
		member.UserID = strings.TrimSpace(member.UserID)
		member.Name = strings.TrimSpace(member.Name)
		member.UnionOpenID = strings.TrimSpace(member.UnionOpenID)
		if member.UserID == "" || seen[member.UserID] {
			continue
		}
		if len(deduplicated) >= rosterMemberLimit {
			break
		}
		seen[member.UserID] = true
		if member.Name == "" {
			member.Name = member.UserID
		}
		deduplicated = append(deduplicated, member)
		if member.UnionOpenID != "" {
			identities[member.UserID] = member.UnionOpenID
		}
	}

	roster := GroupRoster{Members: deduplicated, UpdatedAt: NowISO()}
	if err := s.store.SetKV(KVScopeGlobal, KVNamespaceRoster, scopeID, roster); err != nil {
		return fmt.Errorf("保存群成员快照失败：%w", err)
	}
	if err := s.recordIdentities(scopeID, identities); err != nil {
		return fmt.Errorf("保存群成员身份失败：%w", err)
	}
	return nil
}

// GroupRoster returns the latest cached member snapshot for one group.
func (s *Service) GroupRoster(scopeID string) (GroupRoster, bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return GroupRoster{}, false, nil
	}
	var roster GroupRoster
	found, err := s.store.GetKV(KVScopeGlobal, KVNamespaceRoster, scopeID, &roster)
	if err != nil {
		return GroupRoster{}, false, fmt.Errorf("读取群成员快照失败：%w", err)
	}
	return roster, found, nil
}
