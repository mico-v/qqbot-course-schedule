package schedule

import "strings"

const identityMemberLimit = 500

// RecordIdentity remembers the union OpenID behind a session-local OpenID.
// Group member OpenIDs are only stable inside one group, so this mapping is
// required before a personal schedule can be shared globally.
func (s *Service) RecordIdentity(scopeID, userID, globalUserOpenID string) error {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	globalUserOpenID = strings.TrimSpace(globalUserOpenID)
	if scopeID == "" || userID == "" || globalUserOpenID == "" {
		return nil
	}
	var identities map[string]string
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceIdentity, scopeID, &identities); err != nil {
		return err
	}
	if identities == nil {
		identities = make(map[string]string)
	}
	if existing, ok := identities[userID]; ok && existing == globalUserOpenID {
		return nil
	}
	if len(identities) >= identityMemberLimit {
		if _, exists := identities[userID]; !exists {
			for key := range identities {
				delete(identities, key)
				break
			}
		}
	}
	identities[userID] = globalUserOpenID
	return s.store.SetKV(KVScopeGlobal, KVNamespaceIdentity, scopeID, identities)
}

// ResolveIdentity returns the global union OpenID recorded for a local OpenID.
func (s *Service) ResolveIdentity(scopeID, userID string) (string, bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return "", false, nil
	}
	var identities map[string]string
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceIdentity, scopeID, &identities); err != nil {
		return "", false, err
	}
	globalUserOpenID := strings.TrimSpace(identities[userID])
	return globalUserOpenID, globalUserOpenID != "", nil
}

// ScopeIdentities returns all known local-to-global OpenID mappings in a scope.
func (s *Service) ScopeIdentities(scopeID string) (map[string]string, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, nil
	}
	var identities map[string]string
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceIdentity, scopeID, &identities); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(identities))
	for localID, globalID := range identities {
		localID = strings.TrimSpace(localID)
		globalID = strings.TrimSpace(globalID)
		if localID != "" && globalID != "" {
			result[localID] = globalID
		}
	}
	return result, nil
}
