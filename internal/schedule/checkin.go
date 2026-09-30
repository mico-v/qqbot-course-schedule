package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Check-in bounds and limits.
const (
	// CheckinMinPoints and CheckinMaxPoints bound one day's random award.
	CheckinMinPoints = 1
	CheckinMaxPoints = 10
	// CheckinRecentLimit is how many records /积分 lists.
	CheckinRecentLimit = 10
	// MaxCheckinRecordsPerScope caps the stored records in one scope.
	MaxCheckinRecordsPerScope = 20000
	// MaxCheckinBoardRecords caps the records the admin board returns.
	MaxCheckinBoardRecords = 1000
)

// CheckinRecord is one member's check-in on one LocalTZ day.
type CheckinRecord struct {
	ScopeID   string `json:"scope_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name,omitempty"`
	Day       string `json:"day"`
	Points    int    `json:"points"`
	CreatedAt string `json:"created_at"`
}

// CheckinTotal aggregates one member's points inside a scope.
type CheckinTotal struct {
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"`
	Points int    `json:"points"`
	Days   int    `json:"days"`
}

// CheckinResult is the outcome of one check-in attempt.
type CheckinResult struct {
	Record CheckinRecord `json:"record"`
	Total  int           `json:"total"`
	Days   int           `json:"days"`
	// Already is true when the member had checked in that day before.
	Already bool `json:"already"`
}

// CheckinStatus is one member's points summary for /积分.
type CheckinStatus struct {
	Total   int             `json:"total"`
	Days    int             `json:"days"`
	Records []CheckinRecord `json:"records"`
}

// CheckinBoard is the admin view of one scope's check-ins.
type CheckinBoard struct {
	ScopeID string          `json:"scope_id"`
	Totals  []CheckinTotal  `json:"totals"`
	Records []CheckinRecord `json:"records"`
}

// CheckinDay formats the LocalTZ day key of a moment.
func CheckinDay(at time.Time) string { return at.In(LocalTZ).Format("2006-01-02") }

// Checkin records points for one member on one day. points must be within
// [CheckinMinPoints, CheckinMaxPoints]; the caller rolls the random value so
// tests can pin it. A second call on the same day returns the stored record
// without adding points again.
func (s *Service) Checkin(scopeID, userID, name string, at time.Time, points int) (*CheckinResult, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return nil, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	if points < CheckinMinPoints || points > CheckinMaxPoints {
		return nil, fmt.Errorf("签到积分必须在 %d-%d 之间。", CheckinMinPoints, CheckinMaxPoints)
	}
	day := CheckinDay(at)
	existing, found, err := s.store.GetCheckinRecord(scopeID, userID, day)
	if err != nil {
		return nil, err
	}
	if found && existing != nil {
		total, days, err := s.checkinTotal(scopeID, userID)
		if err != nil {
			return nil, err
		}
		return &CheckinResult{Record: *existing, Total: total, Days: days, Already: true}, nil
	}
	count, err := s.store.CountCheckinRecords(scopeID)
	if err != nil {
		return nil, err
	}
	if count >= MaxCheckinRecordsPerScope {
		return nil, fmt.Errorf("本会话签到记录已达上限 %d 条，请联系管理员清理。", MaxCheckinRecordsPerScope)
	}
	record := CheckinRecord{
		ScopeID:   scopeID,
		UserID:    userID,
		Name:      strings.TrimSpace(name),
		Day:       day,
		Points:    points,
		CreatedAt: NowISO(),
	}
	inserted, err := s.store.InsertCheckinRecord(record)
	if err != nil {
		return nil, err
	}
	if !inserted {
		// A concurrent call won the day; report its record instead of doubling.
		existing, found, err := s.store.GetCheckinRecord(scopeID, userID, day)
		if err != nil {
			return nil, err
		}
		if !found || existing == nil {
			return nil, fmt.Errorf("签到记录写入冲突，请稍后重试。")
		}
		total, days, err := s.checkinTotal(scopeID, userID)
		if err != nil {
			return nil, err
		}
		return &CheckinResult{Record: *existing, Total: total, Days: days, Already: true}, nil
	}
	total, days, err := s.checkinTotal(scopeID, userID)
	if err != nil {
		return nil, err
	}
	return &CheckinResult{Record: record, Total: total, Days: days}, nil
}

// CheckinStatus returns one member's totals and most recent records.
func (s *Service) CheckinStatus(scopeID, userID string) (*CheckinStatus, error) {
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeID == "" || userID == "" {
		return nil, fmt.Errorf("scope_id 和 user_id 不能为空。")
	}
	records, err := s.store.ListCheckinRecords(scopeID, userID, CheckinRecentLimit)
	if err != nil {
		return nil, err
	}
	total, days, err := s.checkinTotal(scopeID, userID)
	if err != nil {
		return nil, err
	}
	return &CheckinStatus{Total: total, Days: days, Records: records}, nil
}

// CheckinBoard lists one scope's per-member totals and newest records.
func (s *Service) CheckinBoard(scopeID string) (*CheckinBoard, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, fmt.Errorf("scope_id 不能为空。")
	}
	all, err := s.store.ListCheckinRecords(scopeID, "", 0)
	if err != nil {
		return nil, err
	}
	records := all
	if len(records) > MaxCheckinBoardRecords {
		records = records[:MaxCheckinBoardRecords]
	}
	totals := make(map[string]*CheckinTotal)
	for _, record := range all {
		total, ok := totals[record.UserID]
		if !ok {
			total = &CheckinTotal{UserID: record.UserID}
			totals[record.UserID] = total
		}
		total.Points += record.Points
		total.Days++
		if total.Name == "" {
			total.Name = record.Name
		}
	}
	board := &CheckinBoard{ScopeID: scopeID, Records: records}
	for _, total := range totals {
		board.Totals = append(board.Totals, *total)
	}
	sort.Slice(board.Totals, func(i, j int) bool {
		if board.Totals[i].Points != board.Totals[j].Points {
			return board.Totals[i].Points > board.Totals[j].Points
		}
		return board.Totals[i].UserID < board.Totals[j].UserID
	})
	return board, nil
}

// checkinTotal sums one member's records across all days.
func (s *Service) checkinTotal(scopeID, userID string) (total, days int, err error) {
	records, err := s.store.ListCheckinRecords(scopeID, userID, 0)
	if err != nil {
		return 0, 0, err
	}
	for _, record := range records {
		total += record.Points
	}
	return total, len(records), nil
}
