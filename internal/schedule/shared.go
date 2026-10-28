package schedule

import (
	"context"
	"fmt"
	"strings"
)

// StorageTarget identifies one exact persisted member schedule.
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

// ResolveStorageTarget chooses the exact writable schedule target for the
// current storage mode. A non-empty current-group schedule always wins.
func (s *Service) ResolveStorageTarget(displayScopeID, localUserID, knownUnionOpenID string) (StorageTarget, error) {
	return s.ResolveStorageTargetContext(
		context.Background(), displayScopeID, localUserID, knownUnionOpenID,
	)
}

// ResolveStorageTargetContext is the cancellable storage target resolver.
func (s *Service) ResolveStorageTargetContext(
	ctx context.Context,
	displayScopeID, localUserID, knownUnionOpenID string,
) (StorageTarget, error) {
	key, err := GroupMemberKeyFromLegacy(displayScopeID, localUserID)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("无法识别要操作的成员: %w", err)
	}
	settings, err := s.BotSettingsContext(ctx)
	if err != nil {
		return StorageTarget{}, fmt.Errorf("读取课表存储设置失败: %w", err)
	}
	currentTarget := NewStorageTarget(key)
	if settings.StorageMode != StorageModeGlobal {
		return currentTarget, nil
	}

	current, found, err := s.schedules.GetGroupMemberSchedule(ctx, key)
	if err != nil {
		return StorageTarget{}, err
	}
	if found && current != nil && len(current.Events) > 0 {
		return currentTarget, nil
	}
	unionOpenID := strings.TrimSpace(knownUnionOpenID)
	if unionOpenID == "" {
		profile, profileFound, profileErr := s.members.GetGroupMember(ctx, key)
		if profileErr != nil {
			return StorageTarget{}, profileErr
		}
		if profileFound {
			unionOpenID = strings.TrimSpace(profile.UnionOpenID)
		}
	}
	_, target, found, err := s.global.FindGlobalSchedule(
		ctx, key.MemberOpenID, unionOpenID, &key,
	)
	if err != nil {
		return StorageTarget{}, err
	}
	if found {
		target.LocalUserID = key.MemberOpenID
		return target, nil
	}
	return currentTarget, nil
}

// VisibleMember resolves the schedule shown for one local group member.
func (s *Service) VisibleMember(displayScopeID, localUserID, knownUnionOpenID string) (*Member, bool, error) {
	return s.VisibleMemberContext(
		context.Background(), displayScopeID, localUserID, knownUnionOpenID,
	)
}

// VisibleMemberContext resolves one schedule for the active storage mode.
func (s *Service) VisibleMemberContext(
	ctx context.Context,
	displayScopeID, localUserID, knownUnionOpenID string,
) (*Member, bool, error) {
	key, err := GroupMemberKeyFromLegacy(displayScopeID, localUserID)
	if err != nil {
		return nil, false, err
	}
	settings, err := s.BotSettingsContext(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("读取课表存储设置失败: %w", err)
	}
	current, found, err := s.schedules.GetGroupMemberSchedule(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if settings.StorageMode != StorageModeGlobal {
		return current, found, nil
	}
	if found && current != nil && len(current.Events) > 0 {
		return current, true, nil
	}

	unionOpenID := strings.TrimSpace(knownUnionOpenID)
	if unionOpenID == "" {
		profile, profileFound, profileErr := s.members.GetGroupMember(ctx, key)
		if profileErr != nil {
			return nil, false, profileErr
		}
		if profileFound {
			unionOpenID = strings.TrimSpace(profile.UnionOpenID)
		}
	}
	globalMember, _, globalFound, err := s.global.FindGlobalSchedule(
		ctx, key.MemberOpenID, unionOpenID, &key,
	)
	if err != nil {
		return nil, false, err
	}
	if globalFound && globalMember != nil {
		projected := *globalMember
		projected.UserID = key.MemberOpenID
		if found && current != nil {
			projected.Name = firstNonEmpty(current.Name, projected.Name)
			projected.QQ = firstNonEmpty(current.QQ, projected.QQ)
			projected.DayOverrides = current.DayOverrides
		}
		if projected.DayOverrides == nil {
			projected.DayOverrides, err = s.currentGroupOverrides(ctx, key)
			if err != nil {
				return nil, false, err
			}
		}
		return &projected, true, nil
	}
	return current, found, nil
}

// VisibleMembers returns schedules visible in one group under the active mode.
func (s *Service) VisibleMembers(displayScopeID string) (map[string]*Member, error) {
	return s.VisibleMembersContext(context.Background(), displayScopeID)
}

// VisibleMembersContext is the cancellable member-set resolver.
func (s *Service) VisibleMembersContext(ctx context.Context, displayScopeID string) (map[string]*Member, error) {
	ref, err := ParseLegacyScopeRef(strings.TrimSpace(displayScopeID))
	if err != nil {
		return nil, err
	}
	if ref.Kind != "group" {
		return nil, ValidationErrorf("仅支持群聊 scope")
	}
	settings, err := s.BotSettingsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取课表存储设置失败: %w", err)
	}
	if settings.StorageMode != StorageModeGlobal {
		return s.listMembersContext(ctx, displayScopeID)
	}
	return s.sharedUserMembersContext(ctx, displayScopeID, nil)
}

// SharedUserMembers remains as a compatibility wrapper for global mode.
func (s *Service) SharedUserMembers(displayScopeID string, extra map[string]string) (map[string]*Member, error) {
	return s.SharedUserMembersContext(context.Background(), displayScopeID, extra)
}

// SharedUserMembersContext projects globally resolved schedules onto local IDs.
func (s *Service) SharedUserMembersContext(
	ctx context.Context,
	displayScopeID string,
	extra map[string]string,
) (map[string]*Member, error) {
	return s.sharedUserMembersContext(ctx, displayScopeID, extra)
}

func (s *Service) sharedUserMembersContext(
	ctx context.Context,
	displayScopeID string,
	extra map[string]string,
) (map[string]*Member, error) {
	ref, err := ParseLegacyScopeRef(displayScopeID)
	if err != nil || ref.Kind != "group" {
		return nil, ValidationErrorf("仅支持群聊 scope")
	}
	profiles, err := s.members.ListGroupMembers(ctx, GroupKey{GroupOpenID: ref.ID})
	if err != nil {
		return nil, err
	}
	current, err := s.schedules.ListGroupSchedules(ctx, GroupKey{GroupOpenID: ref.ID})
	if err != nil {
		return nil, err
	}
	memberIDs := make(map[string]string)
	for _, profile := range profiles {
		memberIDs[profile.Key.MemberOpenID] = strings.TrimSpace(profile.UnionOpenID)
	}
	for memberID := range current {
		if _, ok := memberIDs[memberID]; !ok {
			memberIDs[memberID] = ""
		}
	}
	for memberID, unionOpenID := range extra {
		memberID = strings.TrimSpace(memberID)
		if memberID != "" {
			memberIDs[memberID] = strings.TrimSpace(unionOpenID)
		}
	}

	result := make(map[string]*Member)
	for memberID, unionOpenID := range memberIDs {
		member, found, err := s.VisibleMemberContext(
			ctx, displayScopeID, memberID, unionOpenID,
		)
		if err != nil {
			return nil, err
		}
		if found && member != nil {
			result[memberID] = member
		}
	}
	return result, nil
}

func (s *Service) currentGroupOverrides(ctx context.Context, key GroupMemberKey) (map[string]DayOverride, error) {
	rows, err := s.overrides.ListGroupDayOverrides(ctx, key.Group())
	if err != nil {
		return nil, err
	}
	scopeWide := make(map[string]DayOverride)
	member := make(map[string]DayOverride)
	for _, row := range rows {
		override := DayOverride{Kind: row.Kind, SourceDay: row.SourceDay}
		switch row.UserID {
		case DayOverrideAll:
			scopeWide[row.Day] = override
		case key.MemberOpenID:
			member[row.Day] = override
		}
	}
	return mergeOverrides(scopeWide, member), nil
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

// DeleteMemberSchedule removes the schedule selected by the active mode and
// retains the member profile and seen state.
func (s *Service) DeleteMemberSchedule(
	scopeID, localUserID, unionOpenID string,
	expected Revision,
) (bool, error) {
	return s.DeleteMemberScheduleContext(
		context.Background(), scopeID, localUserID, unionOpenID, expected,
	)
}

// DeleteMemberScheduleContext removes one schedule with cancellation.
func (s *Service) DeleteMemberScheduleContext(
	ctx context.Context,
	scopeID, localUserID, unionOpenID string,
	expected Revision,
) (bool, error) {
	target, err := s.ResolveStorageTargetContext(
		ctx, scopeID, localUserID, unionOpenID,
	)
	if err != nil {
		return false, err
	}
	settings, err := s.BotSettingsContext(ctx)
	if err != nil {
		return false, err
	}
	key, err := GroupMemberKeyFromLegacy(scopeID, localUserID)
	if err != nil {
		return false, err
	}
	if unionOpenID == "" {
		if profile, found, profileErr := s.members.GetGroupMember(ctx, key); profileErr != nil {
			return false, profileErr
		} else if found {
			unionOpenID = profile.UnionOpenID
		}
	}
	deleted, err := s.global.DeleteMemberSchedules(
		ctx,
		target,
		expected,
		key,
		unionOpenID,
		settings.StorageMode == StorageModeGlobal,
	)
	return deleted > 0, err
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
