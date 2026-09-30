package admin

import (
	"fmt"
	"strings"
)

// WebCheckinRecord is one check-in row in the admin page.
type WebCheckinRecord struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Day       string `json:"day"`
	Points    int    `json:"points"`
	CreatedAt string `json:"created_at,omitempty"`
}

// WebCheckinTotal is one member's aggregated points in the admin page.
type WebCheckinTotal struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Points int    `json:"points"`
	Days   int    `json:"days"`
}

// WebCheckinBoard is the admin page payload for one scope.
type WebCheckinBoard struct {
	ScopeID string             `json:"scope_id"`
	Totals  []WebCheckinTotal  `json:"totals"`
	Records []WebCheckinRecord `json:"records"`
}

// WebCheckinBoard lists one scope's check-in records with display names.
func (s *Service) WebCheckinBoard(scopeID string) (*WebCheckinBoard, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, fmt.Errorf("scope_id 不能为空。")
	}
	board, err := s.CheckinBoard(scopeID)
	if err != nil {
		return nil, err
	}
	members, err := s.storage.GetScopeMembers(scopeID)
	if err != nil {
		return nil, err
	}
	display := func(userID, fallback string) string {
		if member, ok := members[userID]; ok && member != nil && strings.TrimSpace(member.Name) != "" {
			return member.Name
		}
		return firstNonEmpty(fallback, userID)
	}
	page := &WebCheckinBoard{
		ScopeID: scopeID,
		Totals:  make([]WebCheckinTotal, 0, len(board.Totals)),
		Records: make([]WebCheckinRecord, 0, len(board.Records)),
	}
	for _, total := range board.Totals {
		page.Totals = append(page.Totals, WebCheckinTotal{
			UserID: total.UserID,
			Name:   display(total.UserID, total.Name),
			Points: total.Points,
			Days:   total.Days,
		})
	}
	for _, record := range board.Records {
		page.Records = append(page.Records, WebCheckinRecord{
			UserID:    record.UserID,
			Name:      display(record.UserID, record.Name),
			Day:       record.Day,
			Points:    record.Points,
			CreatedAt: record.CreatedAt,
		})
	}
	return page, nil
}

// DeleteWebCheckin removes one member's check-in on one day.
func (s *Service) DeleteWebCheckin(scopeID, userID, day string) (bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return false, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	target, err := parseWebDay(day, "签到日期")
	if err != nil {
		return false, err
	}
	return s.storage.DeleteCheckinRecord(scopeID, userID, target.Format("2006-01-02"))
}
