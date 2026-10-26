package schedule

import "strings"

const identityMemberLimit = 500

// RecordIdentity remembers the union OpenID behind a local OpenID when the
// platform returns one. Local OpenIDs remain a compatibility fallback because
// production has returned the same member_openid across groups.
func (s *Service) RecordIdentity(scopeID, userID, globalUserOpenID string) error {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	globalUserOpenID = strings.TrimSpace(globalUserOpenID)
	if scopeID == "" || userID == "" || globalUserOpenID == "" {
		return nil
	}
	return s.recordIdentities(scopeID, map[string]string{userID: globalUserOpenID})
}

func (s *Service) recordIdentities(scopeID string, values map[string]string) error {
	var identities map[string]string
	if _, err := s.store.GetKV(KVScopeGlobal, KVNamespaceIdentity, scopeID, &identities); err != nil {
		return err
	}
	if identities == nil {
		identities = make(map[string]string)
	}
	changed := false
	for userID, globalUserOpenID := range values {
		userID = strings.TrimSpace(userID)
		globalUserOpenID = strings.TrimSpace(globalUserOpenID)
		if userID == "" || globalUserOpenID == "" {
			continue
		}
		if existing, ok := identities[userID]; ok && existing == globalUserOpenID {
			continue
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
		changed = true
	}
	if !changed {
		return nil
	}
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
