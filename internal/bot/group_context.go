package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// Scope returns the schedule scope for a group message.
func (e *Env) Scope(in *Inbound) string {
	if in == nil {
		return ""
	}
	if in.Origin != OriginGroup || strings.TrimSpace(in.GroupOpenID) == "" {
		return ""
	}
	return schedule.ScopeGroup(in.GroupOpenID)
}

// ScheduleTarget resolves one exact group member coordinate.
func (e *Env) ScheduleTarget(_ context.Context, in *Inbound, localUserID string) (schedule.StorageTarget, error) {
	if in == nil || in.Origin != OriginGroup {
		return schedule.StorageTarget{}, schedule.ValidationErrorf("仅支持群聊课表")
	}
	key, err := schedule.GroupMemberKeyFromInbound(in.GroupOpenID, localUserID)
	if err != nil {
		return schedule.StorageTarget{}, err
	}
	return schedule.NewStorageTarget(key), nil
}

// ImportTarget resolves the exact group member that receives an import.
func (e *Env) ImportTarget(ctx context.Context, in *Inbound, localUserID string) (schedule.StorageTarget, error) {
	return e.ScheduleTarget(ctx, in, localUserID)
}

// ImportSource captures optional source metadata for one import.
func (e *Env) ImportSource(ctx context.Context, in *Inbound, target schedule.StorageTarget) schedule.ImportSource {
	source := schedule.ImportSource{}
	if in != nil {
		source.GroupOpenID = in.GroupOpenID
		source.UserOpenID = target.Key.MemberOpenID
		source.UnionOpenID = strings.TrimSpace(in.GlobalUserOpenID)
	}
	if source.UnionOpenID == "" {
		unionOpenID, found, err := e.Service.ResolveIdentity(target.ScopeID, target.UserID)
		if err == nil && found {
			source.UnionOpenID = unionOpenID
		}
	}
	return source
}

// ScheduleMembers returns schedules isolated to the current group.
func (e *Env) ScheduleMembers(ctx context.Context, in *Inbound) (map[string]*schedule.Member, error) {
	scopeID := e.Scope(in)
	if scopeID == "" {
		return nil, schedule.ValidationErrorf("仅支持群聊课表")
	}
	e.syncGroupRoster(ctx, in)
	return e.Service.ScopeMembersContext(ctx, scopeID)
}

func (e *Env) syncGroupRoster(ctx context.Context, in *Inbound) {
	if e == nil || e.Client == nil || in == nil || in.Origin != OriginGroup ||
		strings.TrimSpace(in.GroupOpenID) == "" {
		return
	}
	now := e.now()
	e.rosterMu.Lock()
	if e.rosterAttempts == nil {
		e.rosterAttempts = make(map[string]time.Time)
	}
	if last := e.rosterAttempts[in.GroupOpenID]; !last.IsZero() && now.Sub(last) < groupRosterSyncInterval {
		e.rosterMu.Unlock()
		return
	}
	e.rosterAttempts[in.GroupOpenID] = now
	e.rosterMu.Unlock()

	members, err := e.Client.ListGroupMembers(ctx, in.GroupOpenID)
	if err != nil {
		slog.Warn("group member roster sync failed; using cached members",
			"group", shortID(in.GroupOpenID),
			"permission_denied", qqapi.ErrorCode(err) == 11253,
			"err", err,
		)
		return
	}
	roster := make([]schedule.GroupRosterMember, 0, len(members))
	for _, member := range members {
		if member.Bot || strings.TrimSpace(member.MemberOpenID) == "" {
			continue
		}
		roster = append(roster, schedule.GroupRosterMember{
			UserID:      member.MemberOpenID,
			Name:        member.Username,
			UnionOpenID: member.UnionOpenID,
		})
	}
	if err := e.Service.SaveGroupRosterContext(ctx, e.Scope(in), roster); err != nil {
		slog.Warn("failed to cache group member roster",
			"group", shortID(in.GroupOpenID),
			"err", err,
		)
		return
	}
	slog.Info("group member roster synced",
		"group", shortID(in.GroupOpenID),
		"members", len(roster),
	)
}

// OverrideTargets resolves local IDs to exact group member coordinates.
func (e *Env) OverrideTargets(ctx context.Context, in *Inbound, localUserIDs []string) ([]schedule.DayOverrideTarget, error) {
	scopeID := e.Scope(in)
	targets := make([]schedule.DayOverrideTarget, 0, len(localUserIDs))
	for _, localUserID := range localUserIDs {
		localUserID = strings.TrimSpace(localUserID)
		if localUserID == "" {
			continue
		}
		if localUserID == schedule.DayOverrideAll {
			targets = append(targets, schedule.DayOverrideTarget{
				ScopeID: scopeID, UserID: schedule.DayOverrideAll,
				DisplayID: schedule.DayOverrideAll,
			})
			continue
		}
		target, err := e.ScheduleTarget(ctx, in, localUserID)
		if err != nil {
			return nil, err
		}
		targets = append(targets, schedule.DayOverrideTarget{
			ScopeID: target.ScopeID, UserID: target.UserID, DisplayID: target.LocalUserID,
		})
	}
	return targets, nil
}
