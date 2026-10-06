package schedule

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const scheduleEditTokenBytes = 32

var (
	// ErrScheduleEditLinkInvalid means the token is malformed or unknown.
	ErrScheduleEditLinkInvalid = errors.New("课表修改链接无效。")
	// ErrScheduleEditLinkExpired means the token was valid but has expired.
	ErrScheduleEditLinkExpired = errors.New("课表修改链接已过期，请重新发送 /修改课程表。")
)

// ScheduleEditLink is the member scope carried by one valid edit token.
type ScheduleEditLink struct {
	ScopeID   string
	UserID    string
	ExpiresAt time.Time
}

type storedScheduleEditLink struct {
	ScopeID   string `json:"scope_id"`
	UserID    string `json:"user_id"`
	ExpiresAt string `json:"expires_at"`
}

// CreateScheduleEditLink creates a random bearer token for one member's public
// schedule editor. Only the token digest is persisted.
func (s *Service) CreateScheduleEditLink(scopeID, userID string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return "", time.Time{}, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	if ttl <= 0 {
		return "", time.Time{}, fmt.Errorf("课表修改链接有效期必须大于 0。")
	}
	if now.IsZero() {
		now = time.Now()
	}

	raw := make([]byte, scheduleEditTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("生成课表修改链接失败: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := now.Add(ttl).UTC()
	record := storedScheduleEditLink{
		ScopeID:   scopeID,
		UserID:    userID,
		ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}
	if err := s.store.SetKV(KVScopeGlobal, KVNamespaceEditLinks, scheduleEditTokenKey(token), record); err != nil {
		return "", time.Time{}, fmt.Errorf("保存课表修改链接失败: %w", err)
	}
	return token, expiresAt, nil
}

// ResolveScheduleEditLink validates one token and returns the member it grants
// access to. Expired tokens are removed when they are encountered.
func (s *Service) ResolveScheduleEditLink(token string, now time.Time) (*ScheduleEditLink, error) {
	token = strings.TrimSpace(token)
	if !validScheduleEditToken(token) {
		return nil, ErrScheduleEditLinkInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}

	var record storedScheduleEditLink
	found, err := s.store.GetKV(
		KVScopeGlobal,
		KVNamespaceEditLinks,
		scheduleEditTokenKey(token),
		&record,
	)
	if err != nil {
		return nil, err
	}
	if !found || strings.TrimSpace(record.ScopeID) == "" || strings.TrimSpace(record.UserID) == "" {
		return nil, ErrScheduleEditLinkInvalid
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		return nil, ErrScheduleEditLinkInvalid
	}
	if !now.UTC().Before(expiresAt) {
		_ = s.store.DeleteKV(KVScopeGlobal, KVNamespaceEditLinks, scheduleEditTokenKey(token))
		return nil, ErrScheduleEditLinkExpired
	}
	return &ScheduleEditLink{
		ScopeID:   record.ScopeID,
		UserID:    record.UserID,
		ExpiresAt: expiresAt,
	}, nil
}

func validScheduleEditToken(token string) bool {
	if token == "" {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(raw) == scheduleEditTokenBytes
}

func scheduleEditTokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
