package schedule

import (
	"context"
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

// CreateScheduleEditLink creates a random bearer token for one member's public
// schedule editor. Only the token digest is persisted.
func (s *Service) CreateScheduleEditLink(scopeID, userID string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	return s.CreateScheduleEditLinkContext(context.Background(), scopeID, userID, ttl, now)
}

// CreateScheduleEditLinkContext creates a hashed edit token with cancellation.
func (s *Service) CreateScheduleEditLinkContext(ctx context.Context, scopeID, userID string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	key, err := GroupMemberKeyFromLegacy(scopeID, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	if ttl <= 0 {
		return "", time.Time{}, ValidationErrorf("课表修改链接有效期必须大于 0。")
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
	record := EditLinkRecord{
		TokenHash: scheduleEditTokenKey(token), GroupOpenID: key.GroupOpenID,
		MemberOpenID: key.MemberOpenID, ExpiresAt: expiresAt, CreatedAt: now.UTC(),
	}
	if err := s.editLinks.SaveEditLink(ctx, record); err != nil {
		return "", time.Time{}, fmt.Errorf("保存课表修改链接失败: %w", err)
	}
	return token, expiresAt, nil
}

// ResolveScheduleEditLink validates one token and returns the member it grants
// access to. Expired tokens are removed when they are encountered.
func (s *Service) ResolveScheduleEditLink(token string, now time.Time) (*ScheduleEditLink, error) {
	return s.ResolveScheduleEditLinkContext(context.Background(), token, now)
}

// ResolveScheduleEditLinkContext validates a token with cancellation.
func (s *Service) ResolveScheduleEditLinkContext(ctx context.Context, token string, now time.Time) (*ScheduleEditLink, error) {
	token = strings.TrimSpace(token)
	if !validScheduleEditToken(token) {
		return nil, ErrScheduleEditLinkInvalid
	}
	if now.IsZero() {
		now = time.Now()
	}

	record, found, err := s.editLinks.GetEditLink(
		ctx,
		scheduleEditTokenKey(token),
	)
	if err != nil {
		return nil, err
	}
	if !found || strings.TrimSpace(record.GroupOpenID) == "" || strings.TrimSpace(record.MemberOpenID) == "" {
		return nil, ErrScheduleEditLinkInvalid
	}
	if record.ExpiresAt.IsZero() {
		return nil, ErrScheduleEditLinkInvalid
	}
	if !now.UTC().Before(record.ExpiresAt) {
		_ = s.editLinks.DeleteEditLink(ctx, scheduleEditTokenKey(token))
		return nil, ErrScheduleEditLinkExpired
	}
	return &ScheduleEditLink{
		ScopeID:   ScopeGroup(record.GroupOpenID),
		UserID:    record.MemberOpenID,
		ExpiresAt: record.ExpiresAt,
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
