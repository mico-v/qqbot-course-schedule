package schedule

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// StorageTarget identifies where one member's personal schedule is persisted.
// ScopeID and UserID are storage coordinates; LocalUserID is the identifier
// shown in the current group or private chat.
type StorageTarget struct {
	ScopeID     string
	UserID      string
	LocalUserID string
}

// memberScopeStorage is an optional storage capability used to locate legacy
// per-group rows without adding another required method to every test store.
type memberScopeStorage interface {
	ListMemberScopes(userID string) ([]string, error)
}

// ResolveStorageTarget returns the write target for one member. In global mode
// an existing row in the current conversation always wins so old group data
// keeps working without migration. Otherwise the union OpenID target is used.
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

	current, found, err := s.store.GetMember(displayScopeID, localUserID)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("读取本群课表失败：%w", err)
	}
	if found && current != nil {
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

// VisibleMember resolves the schedule shown for one local member. In global
// mode the current conversation's own schedule has priority, then the shared
// global schedule, then the most recently updated schedule from another group.
func (s *Service) VisibleMember(displayScopeID, localUserID, knownGlobalUserOpenID string) (*Member, bool, error) {
	displayScopeID = strings.TrimSpace(displayScopeID)
	localUserID = strings.TrimSpace(localUserID)
	if displayScopeID == "" || localUserID == "" {
		return nil, false, fmt.Errorf("无法识别要查看的成员，请稍后重试。")
	}
	settings, err := s.BotSettings()
	if err != nil {
		return nil, false, fmt.Errorf("读取课表存储设置失败：%w", err)
	}
	if settings.StorageMode != StorageModeGlobal {
		member, found, err := s.store.GetMember(displayScopeID, localUserID)
		return member, found, err
	}

	resolver, err := s.newGlobalScheduleResolver(displayScopeID)
	if err != nil {
		return nil, false, err
	}
	return resolver.resolve(localUserID, knownGlobalUserOpenID)
}

// VisibleMembers returns the schedules visible in one conversation. Group
// storage keeps the old scope behavior; global storage projects local
// identities onto shared personal schedules and also includes legacy group
// rows that predate the identity map.
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
// local OpenIDs used by one group or private chat. Existing rows in that same
// conversation take priority, and only this scope's all-member overrides are
// merged so another group's markers never leak into the view.
func (s *Service) SharedUserMembers(displayScopeID string, identities map[string]string) (map[string]*Member, error) {
	displayScopeID = strings.TrimSpace(displayScopeID)
	if displayScopeID == "" {
		return nil, fmt.Errorf("无法识别当前会话。")
	}
	resolver, err := s.newGlobalScheduleResolver(displayScopeID)
	if err != nil {
		return nil, err
	}

	currentMembers, err := s.store.GetScopeMembers(displayScopeID)
	if err != nil {
		return nil, err
	}
	localIDs := make(map[string]bool, len(currentMembers)+len(identities))
	for localID := range currentMembers {
		localIDs[localID] = true
	}
	for localID := range identities {
		if strings.TrimSpace(localID) != "" {
			localIDs[localID] = true
		}
	}

	members := make(map[string]*Member)
	for localID := range localIDs {
		knownGlobalUserOpenID := strings.TrimSpace(identities[localID])
		member, found, err := resolver.resolve(localID, knownGlobalUserOpenID)
		if err != nil {
			return nil, err
		}
		if found && member != nil {
			members[localID] = member
		}
	}
	return members, nil
}

type globalScheduleResolver struct {
	service        *Service
	displayScopeID string
	identities     map[string]map[string]string
}

func (s *Service) newGlobalScheduleResolver(displayScopeID string) (*globalScheduleResolver, error) {
	entries, err := s.store.ListKV(KVScopeGlobal, KVNamespaceIdentity)
	if err != nil {
		return nil, fmt.Errorf("读取全局用户身份失败：%w", err)
	}
	identities := make(map[string]map[string]string, len(entries))
	for _, entry := range entries {
		var mapping map[string]string
		if err := json.Unmarshal(entry.Value, &mapping); err != nil {
			continue
		}
		identities[entry.Key] = mapping
	}
	return &globalScheduleResolver{
		service:        s,
		displayScopeID: displayScopeID,
		identities:     identities,
	}, nil
}

func (r *globalScheduleResolver) resolve(localUserID, knownGlobalUserOpenID string) (*Member, bool, error) {
	localUserID = strings.TrimSpace(localUserID)
	if localUserID == "" {
		return nil, false, nil
	}
	globalUserOpenID := strings.TrimSpace(knownGlobalUserOpenID)
	if globalUserOpenID == "" {
		globalUserOpenID = strings.TrimSpace(r.identities[r.displayScopeID][localUserID])
	}

	current, currentFound, err := r.service.store.GetMember(r.displayScopeID, localUserID)
	if err != nil {
		return nil, false, fmt.Errorf("读取本群课表失败：%w", err)
	}
	if currentFound && hasCourses(current) {
		return r.project(localUserID, globalUserOpenID, current)
	}

	var globalMember *Member
	globalFound := false
	if globalUserOpenID != "" {
		globalMember, globalFound, err = r.service.store.GetMember(ScopeGlobalUser(globalUserOpenID), globalUserOpenID)
		if err != nil {
			return nil, false, fmt.Errorf("读取全局课表失败：%w", err)
		}
		if globalFound && hasCourses(globalMember) {
			return r.project(localUserID, globalUserOpenID, globalMember)
		}
	}

	fallback, fallbackFound, err := r.latestOtherGroupMember(localUserID, globalUserOpenID)
	if err != nil {
		return nil, false, err
	}
	if fallbackFound {
		return r.project(localUserID, globalUserOpenID, fallback)
	}
	if currentFound {
		return r.project(localUserID, globalUserOpenID, current)
	}
	if globalFound {
		return r.project(localUserID, globalUserOpenID, globalMember)
	}
	return nil, false, nil
}

func (r *globalScheduleResolver) latestOtherGroupMember(localUserID, globalUserOpenID string) (*Member, bool, error) {
	type candidate struct {
		scopeID string
		userID  string
		member  *Member
	}
	candidates := make(map[string]candidate)
	addCandidate := func(scopeID, userID string) error {
		scopeID = strings.TrimSpace(scopeID)
		userID = strings.TrimSpace(userID)
		if scopeID == "" || userID == "" || scopeID == r.displayScopeID {
			return nil
		}
		kind, _ := ParseScope(scopeID)
		if kind != "group" && kind != "private" {
			return nil
		}
		if mapped := strings.TrimSpace(r.identities[scopeID][userID]); mapped != "" &&
			globalUserOpenID != "" && mapped != globalUserOpenID {
			return nil
		}
		key := scopeID + "\x00" + userID
		if _, exists := candidates[key]; exists {
			return nil
		}
		member, found, err := r.service.store.GetMember(scopeID, userID)
		if err != nil {
			return fmt.Errorf("读取其他群课表失败：%w", err)
		}
		if found && hasCourses(member) {
			candidates[key] = candidate{scopeID: scopeID, userID: userID, member: member}
		}
		return nil
	}

	if globalUserOpenID != "" {
		for scopeID, mapping := range r.identities {
			for userID, mappedGlobalID := range mapping {
				if strings.TrimSpace(mappedGlobalID) == globalUserOpenID {
					if err := addCandidate(scopeID, userID); err != nil {
						return nil, false, err
					}
				}
			}
		}
	}
	if storage, ok := r.service.store.(memberScopeStorage); ok {
		scopeIDs, err := storage.ListMemberScopes(localUserID)
		if err != nil {
			return nil, false, fmt.Errorf("读取其他群课表位置失败：%w", err)
		}
		for _, scopeID := range scopeIDs {
			if err := addCandidate(scopeID, localUserID); err != nil {
				return nil, false, err
			}
		}
	}

	ordered := make([]candidate, 0, len(candidates))
	for _, item := range candidates {
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := memberUpdatedAt(ordered[i].member), memberUpdatedAt(ordered[j].member)
		if !left.Equal(right) {
			return left.After(right)
		}
		if ordered[i].scopeID != ordered[j].scopeID {
			return ordered[i].scopeID < ordered[j].scopeID
		}
		return ordered[i].userID < ordered[j].userID
	})
	if len(ordered) == 0 {
		return nil, false, nil
	}
	return ordered[0].member, true, nil
}

func (r *globalScheduleResolver) project(localUserID, globalUserOpenID string, member *Member) (*Member, bool, error) {
	if member == nil {
		return nil, false, nil
	}
	projected := *member
	projected.UserID = localUserID
	overrides, err := r.personalOverrides(localUserID, globalUserOpenID)
	if err != nil {
		return nil, false, err
	}
	projected.DayOverrides = overrides
	return &projected, true, nil
}

func (r *globalScheduleResolver) personalOverrides(localUserID, globalUserOpenID string) (map[string]DayOverride, error) {
	currentRows, err := r.service.store.ListDayOverrides(r.displayScopeID)
	if err != nil {
		return nil, fmt.Errorf("读取本群休假标记失败：%w", err)
	}
	var globalRows []DayOverrideRow
	if globalUserOpenID != "" {
		globalRows, err = r.service.store.ListDayOverrides(ScopeGlobalUser(globalUserOpenID))
		if err != nil {
			return nil, fmt.Errorf("读取全局休假标记失败：%w", err)
		}
	}

	merged := make(map[string]DayOverride)
	for _, row := range currentRows {
		if row.UserID == DayOverrideAll {
			merged[row.Day] = DayOverride{Kind: row.Kind, SourceDay: row.SourceDay}
		}
	}
	for _, row := range globalRows {
		if row.UserID == globalUserOpenID {
			merged[row.Day] = DayOverride{Kind: row.Kind, SourceDay: row.SourceDay}
		}
	}
	for _, row := range currentRows {
		if row.UserID == localUserID {
			merged[row.Day] = DayOverride{Kind: row.Kind, SourceDay: row.SourceDay}
		}
	}
	if len(merged) == 0 {
		return nil, nil
	}
	return merged, nil
}

func hasCourses(member *Member) bool {
	return member != nil && len(member.Events) > 0
}

func memberUpdatedAt(member *Member) time.Time {
	if member == nil {
		return time.Time{}
	}
	for _, value := range []string{member.ScheduleUpdatedAt, member.UpdatedAt} {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
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
