package schedule

import (
	"fmt"
	"regexp"
	"strings"
)

// qqPattern matches a plausible QQ number: 5-11 digits without a leading zero.
var qqPattern = regexp.MustCompile(`^[1-9][0-9]{4,10}$`)

// NormalizeQQ validates and trims a QQ number; an empty value clears it.
func NormalizeQQ(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !qqPattern.MatchString(value) {
		return "", fmt.Errorf("QQ 号应为 5-11 位数字。")
	}
	return value, nil
}

// SetMemberQQ binds (or clears) a member's QQ number with the revision lock.
func (s *Service) SetMemberQQ(scopeID, userID, qq, actor string) (string, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return "", fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	normalized, err := NormalizeQQ(qq)
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		member, found, err := s.store.GetMember(scopeID, userID)
		if err != nil {
			return "", err
		}
		if !found || member == nil {
			return "", fmt.Errorf("找不到该成员的课表，请先导入课表。")
		}
		if member.QQ == normalized {
			return normalized, nil
		}
		updated := *member
		updated.QQ = normalized
		updated.LastModifiedAt = NowISO()
		updated.LastModifiedBy = actor
		expected := member.Revision
		if err := s.store.PutMember(scopeID, userID, &updated, &expected); err != nil {
			if err == ErrConflict && attempt == 0 {
				continue
			}
			return "", err
		}
		return normalized, nil
	}
	return "", fmt.Errorf("课表刚被其他操作更新，请重试。")
}

// MemberQQBindings returns user_id -> QQ for members with a binding.
func (s *Service) MemberQQBindings(scopeID string) (map[string]string, error) {
	members, err := s.store.GetScopeMembers(scopeID)
	if err != nil {
		return nil, err
	}
	bindings := make(map[string]string)
	for userID, member := range members {
		if member != nil && member.QQ != "" {
			bindings[userID] = member.QQ
		}
	}
	return bindings, nil
}
