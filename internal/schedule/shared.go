package schedule

import (
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

// memberImportUnionStorage locates per-group imports that recorded a stable
// union OpenID. It stays optional so small test stores need not implement it.
type memberImportUnionStorage interface {
	ListMembersByImportUnionOpenID(unionOpenID string) ([]MemberLocation, error)
}

// memberLocalStorage locates schedule rows by their local OpenID. The platform
// describes member_openid as group-scoped, but production has returned the same
// value across groups. Global storage therefore uses it only as a fallback when
// no union OpenID is available.
type memberLocalStorage interface {
	ListMemberScopes(userID string) ([]string, error)
}

type memberCandidate struct {
	location MemberLocation
	member   *Member
}

// ResolveStorageTarget returns the write target for one member. In global mode
// any known union OpenID always maps to the canonical personal scope. A local
// OpenID fallback keeps legacy per-group imports editable when the platform
// does not return a union OpenID.
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
		if found {
			globalUserOpenID = resolved
		}
	}

	current, found, err := s.store.GetMember(displayScopeID, localUserID)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("读取本群课表失败：%w", err)
	}
	if globalUserOpenID == "" && found && current != nil && current.ImportUnionOpenID != "" &&
		(current.ImportUserOpenID == "" || current.ImportUserOpenID == localUserID) {
		globalUserOpenID = strings.TrimSpace(current.ImportUnionOpenID)
	}
	if globalUserOpenID != "" {
		return StorageTarget{
			ScopeID:     ScopeGlobalUser(globalUserOpenID),
			UserID:      globalUserOpenID,
			LocalUserID: localUserID,
		}, nil
	}

	candidates, err := s.localFallbackCandidates(localUserID)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("读取历史课表失败：%w", err)
	}
	if found && current != nil && memberMatchesLocalSource(localUserID, current) {
		candidates = append(candidates, memberCandidate{
			location: MemberLocation{ScopeID: displayScopeID, UserID: localUserID},
			member:   current,
		})
	}
	if len(candidates) > 0 {
		sortMemberCandidates(candidates)
		latest := candidates[0].location
		return StorageTarget{
			ScopeID:     latest.ScopeID,
			UserID:      latest.UserID,
			LocalUserID: localUserID,
		}, nil
	}

	known, err := s.memberKnownInScope(displayScopeID, localUserID)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("读取群成员身份失败：%w", err)
	}
	if known {
		return StorageTarget{
			ScopeID:     displayScopeID,
			UserID:      localUserID,
			LocalUserID: localUserID,
		}, nil
	}
	return StorageTarget{}, fmt.Errorf("找不到该成员的课表记录或群成员身份，请让对方先在本群发送一条消息。")
}

// VisibleMember resolves the latest imported schedule shown for one local
// member. A union OpenID wins when available; the local OpenID is a fallback
// for deployments that return the same value across groups.
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
// storage keeps the old scope behavior; global storage projects the scoped
// roster or locally observed identities onto shared personal schedules.
func (s *Service) VisibleMembers(displayScopeID string) (map[string]*Member, error) {
	settings, err := s.BotSettings()
	if err != nil {
		return nil, fmt.Errorf("读取课表存储设置失败：%w", err)
	}
	if settings.StorageMode != StorageModeGlobal {
		return s.store.GetScopeMembers(displayScopeID)
	}
	return s.SharedUserMembers(displayScopeID, nil)
}

// SharedUserMembers loads global personal schedules and projects them onto the
// local OpenIDs used by one group or private chat. A non-empty cached roster
// is authoritative; otherwise current rows, identity mappings and seen members
// form the fallback member set. extraLocalIDs always stays visible so a newly
// active sender is not hidden by a stale roster snapshot.
func (s *Service) SharedUserMembers(displayScopeID string, extraLocalIDs map[string]string) (map[string]*Member, error) {
	displayScopeID = strings.TrimSpace(displayScopeID)
	if displayScopeID == "" {
		return nil, fmt.Errorf("无法识别当前会话。")
	}
	resolver, err := s.newGlobalScheduleResolver(displayScopeID)
	if err != nil {
		return nil, err
	}

	localIDs := resolver.localMemberIDs(extraLocalIDs)

	members := make(map[string]*Member)
	for localID := range localIDs {
		knownGlobalUserOpenID := strings.TrimSpace(extraLocalIDs[localID])
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
	currentMembers map[string]*Member
	identities     map[string]string
	rosterMembers  map[string]bool
	rosterUnions   map[string]string
	rosterFound    bool
	seenMembers    map[string]string
}

func (s *Service) newGlobalScheduleResolver(displayScopeID string) (*globalScheduleResolver, error) {
	currentMembers, err := s.store.GetScopeMembers(displayScopeID)
	if err != nil {
		return nil, fmt.Errorf("读取本群课表失败：%w", err)
	}
	identities, err := s.ScopeIdentities(displayScopeID)
	if err != nil {
		return nil, fmt.Errorf("读取全局用户身份失败：%w", err)
	}
	rosterMembers := make(map[string]bool)
	rosterUnions := make(map[string]string)
	rosterFound := false
	kind, _ := ParseScope(displayScopeID)
	if kind == "group" {
		roster, found, err := s.GroupRoster(displayScopeID)
		if err != nil {
			return nil, err
		}
		if found {
			rosterFound = len(roster.Members) > 0
			for _, member := range roster.Members {
				if member.UserID == "" {
					continue
				}
				rosterMembers[member.UserID] = true
				if member.UnionOpenID != "" {
					rosterUnions[member.UserID] = member.UnionOpenID
				}
			}
		}
	}
	var seenMembers map[string]string
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceSeen, displayScopeID, &seenMembers); err != nil {
		return nil, fmt.Errorf("读取群成员列表失败：%w", err)
	}
	return &globalScheduleResolver{
		service:        s,
		displayScopeID: displayScopeID,
		currentMembers: currentMembers,
		identities:     identities,
		rosterMembers:  rosterMembers,
		rosterUnions:   rosterUnions,
		rosterFound:    rosterFound,
		seenMembers:    seenMembers,
	}, nil
}

func (r *globalScheduleResolver) localMemberIDs(extra map[string]string) map[string]bool {
	localIDs := make(map[string]bool)
	if r.rosterFound {
		for localID := range r.rosterMembers {
			localIDs[localID] = true
		}
		for localID := range extra {
			if strings.TrimSpace(localID) != "" {
				localIDs[localID] = true
			}
		}
		return localIDs
	}
	for localID := range r.currentMembers {
		if strings.TrimSpace(localID) != "" {
			localIDs[localID] = true
		}
	}
	for localID := range r.identities {
		localIDs[localID] = true
	}
	for localID := range r.rosterUnions {
		localIDs[localID] = true
	}
	for localID := range r.seenMembers {
		if strings.TrimSpace(localID) != "" {
			localIDs[localID] = true
		}
	}
	for localID := range extra {
		if strings.TrimSpace(localID) != "" {
			localIDs[localID] = true
		}
	}
	return localIDs
}

func (r *globalScheduleResolver) resolve(localUserID, knownGlobalUserOpenID string) (*Member, bool, error) {
	localUserID = strings.TrimSpace(localUserID)
	if localUserID == "" {
		return nil, false, nil
	}
	globalUserOpenID := strings.TrimSpace(knownGlobalUserOpenID)
	if globalUserOpenID == "" {
		globalUserOpenID = strings.TrimSpace(r.identities[localUserID])
	}
	if globalUserOpenID == "" {
		globalUserOpenID = strings.TrimSpace(r.rosterUnions[localUserID])
	}

	unionCandidates := make(map[string]memberCandidate)
	add := func(location MemberLocation, member *Member) {
		if member == nil {
			return
		}
		key := location.ScopeID + "\x00" + location.UserID
		unionCandidates[key] = memberCandidate{location: location, member: member}
	}

	current, currentFound, err := r.service.store.GetMember(r.displayScopeID, localUserID)
	if err != nil {
		return nil, false, fmt.Errorf("读取本群课表失败：%w", err)
	}
	if currentFound && memberBelongsToDisplayScope(r.displayScopeID, localUserID, current) &&
		memberMatchesUnionSource(globalUserOpenID, current) {
		add(MemberLocation{ScopeID: r.displayScopeID, UserID: localUserID}, current)
	}

	if globalUserOpenID != "" {
		globalMember, globalFound, err := r.service.store.GetMember(ScopeGlobalUser(globalUserOpenID), globalUserOpenID)
		if err != nil {
			return nil, false, fmt.Errorf("读取全局课表失败：%w", err)
		}
		if globalFound {
			add(MemberLocation{ScopeID: ScopeGlobalUser(globalUserOpenID), UserID: globalUserOpenID}, globalMember)
		}
	}

	if globalUserOpenID != "" {
		if storage, ok := r.service.store.(memberImportUnionStorage); ok {
			locations, err := storage.ListMembersByImportUnionOpenID(globalUserOpenID)
			if err != nil {
				return nil, false, fmt.Errorf("读取成员导入来源失败：%w", err)
			}
			for _, location := range locations {
				member, found, err := r.service.store.GetMember(location.ScopeID, location.UserID)
				if err != nil {
					return nil, false, fmt.Errorf("读取历史课表失败：%w", err)
				}
				if found && member.ImportUnionOpenID == globalUserOpenID {
					add(location, member)
				}
			}
		}
	}

	if hasCandidateCourses(unionCandidates) {
		item, _ := latestMemberCandidate(unionCandidates)
		return r.project(localUserID, globalUserOpenID, item.member)
	}

	// A union schedule is authoritative whenever one exists with courses. Only
	// fall back to rows matched by the local OpenID when no usable union
	// schedule is found, for example while migrating old per-group data.
	localCandidates, err := r.service.localFallbackCandidates(localUserID)
	if err != nil {
		return nil, false, fmt.Errorf("读取历史课表失败：%w", err)
	}
	for _, item := range localCandidates {
		add(item.location, item.member)
	}
	item, found := latestMemberCandidate(unionCandidates)
	if !found {
		return nil, false, nil
	}
	return r.project(localUserID, globalUserOpenID, item.member)
}

func (s *Service) localFallbackCandidates(localUserID string) ([]memberCandidate, error) {
	storage, ok := s.store.(memberLocalStorage)
	if !ok {
		return nil, nil
	}
	scopeIDs, err := storage.ListMemberScopes(localUserID)
	if err != nil {
		return nil, err
	}
	seen := make(map[MemberLocation]bool)
	candidates := make([]memberCandidate, 0, len(scopeIDs))
	for _, scopeID := range scopeIDs {
		kind, _ := ParseScope(scopeID)
		if kind != "group" && kind != "private" {
			continue
		}
		location := MemberLocation{ScopeID: scopeID, UserID: localUserID}
		if seen[location] {
			continue
		}
		member, found, err := s.store.GetMember(scopeID, localUserID)
		if err != nil {
			return nil, err
		}
		if !found || !memberMatchesLocalSource(localUserID, member) {
			continue
		}
		seen[location] = true
		candidates = append(candidates, memberCandidate{location: location, member: member})
	}
	return candidates, nil
}

func (s *Service) memberKnownInScope(scopeID, userID string) (bool, error) {
	var seen map[string]string
	found, err := s.store.GetKV(KVScopeGlobal, KVNamespaceSeen, scopeID, &seen)
	if err != nil {
		return false, err
	}
	if found {
		if _, ok := seen[userID]; ok {
			return true, nil
		}
	}

	kind, _ := ParseScope(scopeID)
	if kind != "group" {
		return false, nil
	}
	roster, found, err := s.GroupRoster(scopeID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	for _, member := range roster.Members {
		if member.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func memberMatchesLocalSource(localUserID string, member *Member) bool {
	if member == nil || strings.TrimSpace(member.ImportUnionOpenID) != "" {
		return false
	}
	importUserID := strings.TrimSpace(member.ImportUserOpenID)
	return importUserID == "" || importUserID == strings.TrimSpace(localUserID)
}

func memberMatchesUnionSource(globalUserOpenID string, member *Member) bool {
	globalUserOpenID = strings.TrimSpace(globalUserOpenID)
	if globalUserOpenID == "" || member == nil {
		return false
	}
	return strings.TrimSpace(member.ImportUnionOpenID) == globalUserOpenID
}

func hasCandidateCourses(candidates map[string]memberCandidate) bool {
	for _, candidate := range candidates {
		if hasCourses(candidate.member) {
			return true
		}
	}
	return false
}

func latestMemberCandidate(candidates map[string]memberCandidate) (memberCandidate, bool) {
	ordered := make([]memberCandidate, 0, len(candidates))
	for _, item := range candidates {
		ordered = append(ordered, item)
	}
	sortMemberCandidates(ordered)
	if len(ordered) == 0 {
		return memberCandidate{}, false
	}
	for _, item := range ordered {
		if hasCourses(item.member) {
			return item, true
		}
	}
	return ordered[0], true
}

func sortMemberCandidates(candidates []memberCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left := memberUpdatedAt(candidates[i].member)
		right := memberUpdatedAt(candidates[j].member)
		if !left.Equal(right) {
			return left.After(right)
		}
		if candidates[i].location.ScopeID != candidates[j].location.ScopeID {
			return candidates[i].location.ScopeID < candidates[j].location.ScopeID
		}
		return candidates[i].location.UserID < candidates[j].location.UserID
	})
}

func memberBelongsToDisplayScope(displayScopeID, localUserID string, member *Member) bool {
	if member == nil {
		return false
	}
	kind, targetID := ParseScope(displayScopeID)
	switch kind {
	case "group":
		return member.ImportGroupOpenID == targetID &&
			(member.ImportUserOpenID == "" || member.ImportUserOpenID == localUserID)
	case "private":
		return member.ImportGroupOpenID == "" &&
			(member.ImportUserOpenID == "" || member.ImportUserOpenID == localUserID)
	default:
		return true
	}
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
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value)); err == nil {
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
