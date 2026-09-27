package schedule

import "strings"

const seenMemberLimit = 500

// RecordSeenMember remembers a member who interacted with the bot, so the admin
// page can offer empty schedules for people without one.
func (s *Service) RecordSeenMember(scopeID, userID, name string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	var seen map[string]string
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceSeen, scopeID, &seen); err != nil {
		return err
	}
	if seen == nil {
		seen = make(map[string]string)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = userID
	}
	if existing, ok := seen[userID]; ok && existing == name {
		return nil
	}
	if len(seen) >= seenMemberLimit {
		if _, exists := seen[userID]; !exists {
			for key := range seen {
				delete(seen, key)
				break
			}
		}
	}
	seen[userID] = name
	return s.store.SetKV(KVScopeGlobal, KVNamespaceSeen, scopeID, seen)
}
