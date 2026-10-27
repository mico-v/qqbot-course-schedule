package schedule

import (
	"fmt"
	"strings"
)

// GroupKey identifies one group conversation.
type GroupKey struct {
	GroupOpenID string
}

// Validate reports whether the group coordinate is usable.
func (k GroupKey) Validate() error {
	if strings.TrimSpace(k.GroupOpenID) == "" {
		return ValidationErrorf("group_openid 不能为空")
	}
	return nil
}

// ScopeID returns the legacy wire representation of a group coordinate.
func (k GroupKey) ScopeID() string {
	return "group:" + k.GroupOpenID
}

// GroupMemberKey identifies one member inside one group.
//
// MemberOpenID is a platform-issued group-local identifier. It is not a
// nickname and two groups may legitimately return the same literal value.
type GroupMemberKey struct {
	GroupOpenID  string
	MemberOpenID string
}

// Validate reports whether the member coordinate is usable.
func (k GroupMemberKey) Validate() error {
	if strings.TrimSpace(k.GroupOpenID) == "" {
		return ValidationErrorf("group_openid 不能为空")
	}
	if strings.TrimSpace(k.MemberOpenID) == "" {
		return ValidationErrorf("member_openid 不能为空")
	}
	return nil
}

// Group returns the containing group coordinate.
func (k GroupMemberKey) Group() GroupKey {
	return GroupKey{GroupOpenID: k.GroupOpenID}
}

// Revision is a schedule revision used for optimistic concurrency control.
// Zero means create-only; positive values require an exact match.
type Revision int64

// LegacyScopeRef is accepted only at HTTP and migration boundaries.
type LegacyScopeRef struct {
	Kind string
	ID   string
}

// ParseLegacyScopeRef parses a v6 scope identifier without treating it as a
// new domain identity.
func ParseLegacyScopeRef(scopeID string) (LegacyScopeRef, error) {
	kind, id, ok := strings.Cut(strings.TrimSpace(scopeID), ":")
	if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(id) == "" {
		return LegacyScopeRef{}, ValidationErrorf("无效的 scope_id")
	}
	return LegacyScopeRef{Kind: kind, ID: id}, nil
}

// GroupMemberKeyFromLegacy converts a group legacy coordinate. Non-group
// scopes are rejected so compatibility data cannot enter new group use cases.
func GroupMemberKeyFromLegacy(scopeID, memberOpenID string) (GroupMemberKey, error) {
	ref, err := ParseLegacyScopeRef(scopeID)
	if err != nil {
		return GroupMemberKey{}, err
	}
	if ref.Kind != "group" {
		return GroupMemberKey{}, ValidationErrorf("仅支持群聊 scope")
	}
	key := GroupMemberKey{GroupOpenID: ref.ID, MemberOpenID: strings.TrimSpace(memberOpenID)}
	if err := key.Validate(); err != nil {
		return GroupMemberKey{}, err
	}
	return key, nil
}

// GroupMemberKeyFromInbound builds the only valid group member coordinate.
func GroupMemberKeyFromInbound(groupOpenID, memberOpenID string) (GroupMemberKey, error) {
	key := GroupMemberKey{
		GroupOpenID:  strings.TrimSpace(groupOpenID),
		MemberOpenID: strings.TrimSpace(memberOpenID),
	}
	if err := key.Validate(); err != nil {
		return GroupMemberKey{}, fmt.Errorf("无效的群成员坐标: %w", err)
	}
	return key, nil
}
