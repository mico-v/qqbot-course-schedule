package admin

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// ScheduleStorage is the narrow schedule port used by admin use cases.
type ScheduleStorage interface {
	GetMember(scopeID, userID string) (*schedule.Member, bool, error)
	GetScopeMembers(scopeID string) (map[string]*schedule.Member, error)
	PutMember(scopeID, userID string, member *schedule.Member, expectedRevision *int64) error
}

// DirectoryStorage lists groups, members and schedule summaries.
type DirectoryStorage interface {
	ListScopeSummaries() ([]ScopeSummary, error)
	ListKnownGroups() ([]string, error)
	ListGroupMembers(context.Context, schedule.GroupKey) ([]schedule.GroupMember, error)
}

// OverrideStorage owns holiday/shift markers.
type OverrideStorage interface {
	ListDayOverrides(scopeID string) ([]schedule.DayOverrideRow, error)
	DeleteScopeDayOverrides(scopeID string) error
	SetDayOverride(scopeID, userID, day string, override schedule.DayOverride, createdBy, createdAt string) error
	DeleteDayOverride(scopeID, userID, day string) (bool, error)
}

// Storage is the composition-root aggregate accepted by NewService.
type Storage interface {
	ScheduleStorage
	DirectoryStorage
	OverrideStorage
}

// Service is the admin application layer. It composes the schedule service for
// domain operations and uses its own storage port for admin-only queries.
type Service struct {
	domain    *schedule.Service
	schedules ScheduleStorage
	directory DirectoryStorage
	overrides OverrideStorage
}

// NewService wires the admin layer to the schedule service and admin storage.
func NewService(domain *schedule.Service, storage Storage) *Service {
	return &Service{
		domain:    domain,
		schedules: storage,
		directory: storage,
		overrides: storage,
	}
}

// ScopeSummaries lists every scope that has saved schedules, plus scopes whose
// members only interacted with the bot (so their first schedule can be created).
func (s *Service) ScopeSummaries() ([]ScopeSummary, error) {
	rawSummaries, err := s.directory.ListScopeSummaries()
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	summaries := make(map[string]ScopeSummary)
	for _, summary := range rawSummaries {
		if !isDisplayScope(summary.ScopeID) {
			continue
		}
		known[summary.ScopeID] = true
		summaries[summary.ScopeID] = summary
	}
	knownGroups, err := s.directory.ListKnownGroups()
	if err != nil {
		return nil, err
	}
	for _, groupOpenID := range knownGroups {
		scopeID := schedule.ScopeGroup(groupOpenID)
		if isDisplayScope(scopeID) {
			known[scopeID] = true
		}
	}
	result := make([]ScopeSummary, 0, len(known))
	for scopeID := range known {
		members := summaries[scopeID].Members
		result = append(result, ScopeSummary{ScopeID: scopeID, Members: members})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ScopeID < result[j].ScopeID })
	return result, nil
}

func isDisplayScope(scopeID string) bool {
	kind, _ := schedule.ParseScope(scopeID)
	return kind == "group"
}

func memberSummaries(members map[string]*schedule.Member) []ScopeMemberSummary {
	ids := make([]string, 0, len(members))
	for userID := range members {
		ids = append(ids, userID)
	}
	sort.Strings(ids)
	summaries := make([]ScopeMemberSummary, 0, len(ids))
	for _, userID := range ids {
		member := members[userID]
		if member == nil {
			continue
		}
		summaries = append(summaries, ScopeMemberSummary{
			UserID:     userID,
			Name:       firstNonEmpty(member.Name, userID),
			EventCount: len(member.Events),
			Revision:   member.Revision,
		})
	}
	return summaries
}

// CreateMemberSchedules creates empty schedules, skipping existing members.
func (s *Service) CreateMemberSchedules(scopeID string, inputs []NewMember, actor string) ([]NewMember, error) {
	scopeID = strings.TrimSpace(scopeID)
	kind, _ := schedule.ParseScope(scopeID)
	if kind != "group" && kind != "private" {
		return nil, schedule.ValidationErrorf("无效的 scope_id。")
	}
	if len(inputs) == 0 {
		return nil, schedule.ValidationErrorf("members 不能为空。")
	}
	if len(inputs) > schedule.MaxMembersPerCreate {
		return nil, schedule.ValidationErrorf("一次最多创建 %d 位成员。", schedule.MaxMembersPerCreate)
	}
	var created []NewMember
	for _, input := range inputs {
		userID := strings.TrimSpace(input.UserID)
		if userID == "" {
			continue
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = userID
		}
		if len([]rune(name)) > schedule.MaxMemberNameLength {
			return nil, schedule.ValidationErrorf("成员名称不能超过 %d 个字符。", schedule.MaxMemberNameLength)
		}
		target, err := s.domain.ResolveStorageTarget(scopeID, userID, "")
		if err != nil {
			return nil, err
		}
		member := &schedule.Member{
			UserID:         target.UserID,
			Name:           name,
			Source:         "manual",
			UpdatedAt:      schedule.NowISO(),
			LastModifiedBy: actor,
		}
		zero := int64(0)
		if err := s.schedules.PutMember(target.ScopeID, target.UserID, member, &zero); err != nil {
			if err == schedule.ErrConflict {
				continue
			}
			return nil, err
		}
		created = append(created, NewMember{UserID: userID, Name: name})
	}
	if len(created) == 0 {
		return nil, schedule.ConflictErrorf("所选成员都已经有课表了。")
	}
	return created, nil
}

// WebMemberICS returns one member's ICS content for a direct download.
// content is empty when the member has no events yet.
func (s *Service) WebMemberICS(scopeID, userID string) (name, content string, found bool, err error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return "", "", false, schedule.ValidationErrorf("scope_id 和 user_id 不能为空。")
	}
	member, found, err := s.domain.VisibleMember(scopeID, userID, "")
	if err != nil || !found || member == nil {
		return "", "", false, err
	}
	name = firstNonEmpty(member.Name, userID)
	if len(member.Events) == 0 {
		return name, "", true, nil
	}
	content = strings.TrimSpace(member.ICS)
	if content == "" {
		content = schedule.SerializeScheduleICS(member.Events, "", member.Name)
	}
	return name, content, true, nil
}

// WebMemberICSAt returns ICS content for exact storage coordinates.
func (s *Service) WebMemberICSAt(scopeID, userID string) (name, content string, found bool, err error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return "", "", false, schedule.ValidationErrorf("scope_id 和 user_id 不能为空。")
	}
	member, found, err := s.schedules.GetMember(scopeID, userID)
	if err != nil || !found || member == nil {
		return "", "", false, err
	}
	name = firstNonEmpty(member.Name, userID)
	if len(member.Events) == 0 {
		return name, "", true, nil
	}
	content = strings.TrimSpace(member.ICS)
	if content == "" {
		content = schedule.SerializeScheduleICS(member.Events, "", member.Name)
	}
	return name, content, true, nil
}

// PendingMembers returns observed members who do not have a schedule yet.
func (s *Service) PendingMembers(scopeID string) ([]NewMember, error) {
	ref, err := schedule.ParseLegacyScopeRef(scopeID)
	if err != nil || ref.Kind != "group" {
		return nil, schedule.ValidationErrorf("无效的群聊 scope_id")
	}
	profiles, err := s.directory.ListGroupMembers(
		context.Background(),
		schedule.GroupKey{GroupOpenID: ref.ID},
	)
	if err != nil {
		return nil, err
	}
	members, err := s.domain.VisibleMembers(scopeID)
	if err != nil {
		return nil, err
	}
	var pending []NewMember
	for _, profile := range profiles {
		memberID := profile.Key.MemberOpenID
		if _, exists := members[memberID]; exists {
			continue
		}
		pending = append(pending, NewMember{
			UserID: memberID,
			Name:   firstNonEmpty(profile.DisplayName, memberID),
		})
	}
	sort.Slice(pending, func(i, j int) bool {
		left, right := strings.ToLower(pending[i].Name), strings.ToLower(pending[j].Name)
		if left != right {
			return left < right
		}
		return pending[i].UserID < pending[j].UserID
	})
	return pending, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sameDay(left, right time.Time) bool {
	left = left.In(schedule.LocalTZ)
	right = right.In(schedule.LocalTZ)
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}
