package schedule

import (
	"fmt"
	"strings"
)

// StorageTarget identifies where one member's personal schedule is persisted.
// ScopeID and UserID are storage coordinates; LocalUserID is the identifier
// shown in the current group or private chat.
type StorageTarget struct {
	ScopeID     string
	UserID      string
	LocalUserID string
}

// ResolveStorageTarget maps a displayed member to the configured personal
// schedule storage. knownGlobalUserOpenID is used when the current message
// already carries the union OpenID; otherwise the identity map is consulted.
func (s *Service) ResolveStorageTarget(displayScopeID, localUserID, knownGlobalUserOpenID string) (StorageTarget, error) {
	displayScopeID = strings.TrimSpace(displayScopeID)
	localUserID = strings.TrimSpace(localUserID)
	if displayScopeID == "" || localUserID == "" {
		return StorageTarget{}, fmt.Errorf("无法识别要操作的成员，请稍后重试。")
	}
	settings, err := s.BotSettings()
	if err != nil {
		return StorageTarget{}, fmt.Errorf("读取课表存储设置失败：%w", err)
	}
	if settings.StorageMode != StorageModeGlobal {
		return StorageTarget{
			ScopeID:     displayScopeID,
			UserID:      localUserID,
			LocalUserID: localUserID,
		}, nil
	}

	globalUserOpenID := strings.TrimSpace(knownGlobalUserOpenID)
	if globalUserOpenID == "" {
		resolved, found, err := s.ResolveIdentity(displayScopeID, localUserID)
		if err != nil {
			return StorageTarget{}, fmt.Errorf("读取全局用户身份失败：%w", err)
		}
		if !found {
			return StorageTarget{}, fmt.Errorf("找不到该成员的全局用户身份，请让对方先在本群发送一条消息。")
		}
		globalUserOpenID = resolved
	}
	return StorageTarget{
		ScopeID:     ScopeGlobalUser(globalUserOpenID),
		UserID:      globalUserOpenID,
		LocalUserID: localUserID,
	}, nil
}

// VisibleMembers returns the schedules visible in one conversation. Group
// storage keeps the old scope behavior; global storage projects only known
// local identities onto their shared personal schedules.
func (s *Service) VisibleMembers(displayScopeID string) (map[string]*Member, error) {
	settings, err := s.BotSettings()
	if err != nil {
		return nil, fmt.Errorf("读取课表存储设置失败：%w", err)
	}
	if settings.StorageMode != StorageModeGlobal {
		return s.store.GetScopeMembers(displayScopeID)
	}
	identities, err := s.ScopeIdentities(displayScopeID)
	if err != nil {
		return nil, err
	}
	return s.SharedUserMembers(displayScopeID, identities)
}

// SharedUserMembers loads global personal schedules and projects them onto the
// local OpenIDs used by one group or private chat. Only this scope's
// all-member overrides are merged, so switching to global storage does not
// apply another group's personal markers.
func (s *Service) SharedUserMembers(displayScopeID string, identities map[string]string) (map[string]*Member, error) {
	rows, err := s.store.ListDayOverrides(displayScopeID)
	if err != nil {
		return nil, err
	}
	groupWideRaw := make(map[string]DayOverride)
	for _, row := range rows {
		if row.UserID == DayOverrideAll {
			groupWideRaw[row.Day] = DayOverride{Kind: row.Kind, SourceDay: row.SourceDay}
		}
	}

	members := make(map[string]*Member)
	for localID, globalUserOpenID := range identities {
		globalScopeID := ScopeGlobalUser(globalUserOpenID)
		member, found, err := s.store.GetMember(globalScopeID, globalUserOpenID)
		if err != nil {
			return nil, err
		}
		if !found || member == nil {
			continue
		}
		projected := *member
		projected.UserID = localID
		projected.DayOverrides = mergeOverrides(groupWideRaw, member.DayOverrides)
		members[localID] = &projected
	}
	return members, nil
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
