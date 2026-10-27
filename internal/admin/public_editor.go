package admin

import (
	"context"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// PublicPageSchedule resolves one edit token and loads only that member.
func (s *Service) PublicPageSchedule(token string, now time.Time) (*PublicPageSchedule, bool, error) {
	return s.PublicPageScheduleContext(context.Background(), token, now)
}

// PublicPageScheduleContext resolves a public token with request cancellation.
func (s *Service) PublicPageScheduleContext(ctx context.Context, token string, now time.Time) (*PublicPageSchedule, bool, error) {
	link, err := s.domain.ResolveScheduleEditLinkContext(ctx, token, now)
	if err != nil {
		return nil, false, err
	}
	page, found, err := s.PageScheduleAtContext(ctx, link.ScopeID, link.UserID)
	if err != nil || !found {
		return nil, found, err
	}
	return &PublicPageSchedule{
		Name:      page.Name,
		QQ:        page.QQ,
		Revision:  page.Revision,
		ExpiresAt: link.ExpiresAt.In(schedule.LocalTZ).Format(time.RFC3339),
		Events:    page.Events,
	}, true, nil
}

// SavePublicSchedule validates one edit token and saves only the bound member.
func (s *Service) SavePublicSchedule(token string, payload PublicSavePayload, now time.Time) (*PublicSaveResult, error) {
	return s.SavePublicScheduleContext(context.Background(), token, payload, now)
}

// SavePublicScheduleContext saves a token-scoped member with cancellation.
func (s *Service) SavePublicScheduleContext(ctx context.Context, token string, payload PublicSavePayload, now time.Time) (*PublicSaveResult, error) {
	link, err := s.domain.ResolveScheduleEditLinkContext(ctx, token, now)
	if err != nil {
		return nil, err
	}
	result, err := s.SavePageScheduleAtContext(ctx, SavePagePayload{
		ScopeID:  link.ScopeID,
		UserID:   link.UserID,
		Revision: payload.Revision,
		Name:     payload.Name,
		QQ:       payload.QQ,
		Events:   payload.Events,
	}, "edit-link")
	if err != nil {
		return nil, err
	}
	return &PublicSaveResult{
		Name:       result.Name,
		Revision:   result.Revision,
		EventCount: result.EventCount,
	}, nil
}
