package schedule

import (
	"context"
	"strings"
	"time"
)

// RecordSeenMember upserts a member observed through a group message. Seen
// state is durable member data, not a separate KV namespace.
func (s *Service) RecordSeenMember(scopeID, userID, name string) error {
	return s.RecordSeenMemberContext(context.Background(), scopeID, userID, name)
}

// RecordSeenMemberContext records a member observation with cancellation.
func (s *Service) RecordSeenMemberContext(ctx context.Context, scopeID, userID, name string) error {
	key, err := GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return err
	}
	return s.members.RecordGroupMemberSeen(ctx, GroupMemberObservation{
		Key:         key,
		DisplayName: strings.TrimSpace(name),
		ObservedAt:  time.Now().UTC(),
	})
}
