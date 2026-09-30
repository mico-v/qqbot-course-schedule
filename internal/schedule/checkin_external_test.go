package schedule_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func checkinAt(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, schedule.LocalTZ)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func TestCheckinOncePerDay(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	day := checkinAt(t, "2026-09-17 09:00")

	first, err := service.Checkin(scope, "U1", "小明", day, 7)
	if err != nil {
		t.Fatalf("Checkin: %v", err)
	}
	if first.Already || first.Record.Points != 7 || first.Total != 7 || first.Days != 1 {
		t.Fatalf("first check-in = %+v", first)
	}
	if first.Record.Day != "2026-09-17" {
		t.Fatalf("day = %q, want 2026-09-17", first.Record.Day)
	}

	second, err := service.Checkin(scope, "U1", "小明", day.Add(3*time.Hour), 3)
	if err != nil {
		t.Fatalf("second Checkin: %v", err)
	}
	if !second.Already || second.Record.Points != 7 || second.Total != 7 || second.Days != 1 {
		t.Fatalf("second check-in = %+v, want the stored 7-point record", second)
	}

	next, err := service.Checkin(scope, "U1", "小明", day.AddDate(0, 0, 1), 10)
	if err != nil {
		t.Fatalf("next-day Checkin: %v", err)
	}
	if next.Already || next.Total != 17 || next.Days != 2 {
		t.Fatalf("next-day check-in = %+v", next)
	}

	// Another member's points stay separate.
	other, err := service.Checkin(scope, "U2", "小红", day, 1)
	if err != nil {
		t.Fatalf("other Checkin: %v", err)
	}
	if other.Total != 1 || other.Days != 1 {
		t.Fatalf("other check-in = %+v", other)
	}
}

func TestCheckinRejectsBadPointsAndScope(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	day := checkinAt(t, "2026-09-17 09:00")

	for _, points := range []int{0, -1, schedule.CheckinMaxPoints + 1} {
		if _, err := service.Checkin(scope, "U1", "小明", day, points); err == nil {
			t.Fatalf("points=%d was accepted", points)
		}
	}
	if _, err := service.Checkin("", "U1", "小明", day, 5); err == nil {
		t.Fatal("empty scope was accepted")
	}
	if _, err := service.Checkin(scope, "", "小明", day, 5); err == nil {
		t.Fatal("empty user was accepted")
	}
}

func TestCheckinStatusAndBoard(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	day := checkinAt(t, "2026-09-17 09:00")

	awards := []int{5, 9, 2}
	for index, points := range awards {
		if _, err := service.Checkin(scope, "U1", "小明", day.AddDate(0, 0, index), points); err != nil {
			t.Fatalf("Checkin day %d: %v", index, err)
		}
	}
	if _, err := service.Checkin(scope, "U2", "小红", day.AddDate(0, 0, 2), 10); err != nil {
		t.Fatalf("Checkin U2: %v", err)
	}

	status, err := service.CheckinStatus(scope, "U1")
	if err != nil {
		t.Fatalf("CheckinStatus: %v", err)
	}
	if status.Total != 16 || status.Days != 3 {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Records) != 3 || status.Records[0].Day != "2026-09-19" {
		t.Fatalf("records = %+v, want newest day first", status.Records)
	}

	empty, err := service.CheckinStatus(scope, "U9")
	if err != nil {
		t.Fatalf("empty CheckinStatus: %v", err)
	}
	if empty.Total != 0 || empty.Days != 0 || len(empty.Records) != 0 {
		t.Fatalf("empty status = %+v", empty)
	}

	board, err := service.CheckinBoard(scope)
	if err != nil {
		t.Fatalf("CheckinBoard: %v", err)
	}
	if len(board.Totals) != 2 || board.Totals[0].UserID != "U1" || board.Totals[0].Points != 16 {
		t.Fatalf("board totals = %+v", board.Totals)
	}
	if board.Totals[1].UserID != "U2" || board.Totals[1].Points != 10 || board.Totals[1].Days != 1 {
		t.Fatalf("board totals = %+v", board.Totals)
	}
	if len(board.Records) != 4 || board.Records[0].Day != "2026-09-19" {
		t.Fatalf("board records = %+v", board.Records)
	}
	if board.Totals[0].Name != "小明" {
		t.Fatalf("board name = %q", board.Totals[0].Name)
	}

	if _, err := service.CheckinBoard("  "); err == nil {
		t.Fatal("empty scope board was accepted")
	}
}

func TestCheckinScopesAreIsolated(t *testing.T) {
	service, storeHandle := newService(t)
	day := checkinAt(t, "2026-09-17 09:00")
	if _, err := service.Checkin(schedule.ScopeGroup("G1"), "U1", "小明", day, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Checkin(schedule.ScopeGroup("G2"), "U1", "小明", day, 6); err != nil {
		t.Fatal(err)
	}
	status, err := service.CheckinStatus(schedule.ScopeGroup("G1"), "U1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Total != 4 {
		t.Fatalf("scope G1 total = %d, want 4", status.Total)
	}

	// Deleting the record clears the day and frees a re-check-in.
	deleted, err := storeHandle.DeleteCheckinRecord(schedule.ScopeGroup("G1"), "U1", "2026-09-17")
	if err != nil || !deleted {
		t.Fatalf("DeleteCheckinRecord = %v, %v", deleted, err)
	}
	if _, err := storeHandle.DeleteCheckinRecord(schedule.ScopeGroup("G1"), "U1", "2026-09-17"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	again, err := service.Checkin(schedule.ScopeGroup("G1"), "U1", "小明", day, 3)
	if err != nil {
		t.Fatal(err)
	}
	if again.Already || again.Total != 3 {
		t.Fatalf("re-check-in = %+v", again)
	}
}

func TestCheckinDayUsesLocalTZ(t *testing.T) {
	moment := time.Date(2026, 9, 17, 23, 30, 0, 0, time.UTC)
	if got := schedule.CheckinDay(moment); got != "2026-09-18" {
		t.Fatalf("CheckinDay = %q, want 2026-09-18", got)
	}
	if !strings.Contains(schedule.CheckinDay(time.Now()), "-") {
		t.Fatal("CheckinDay should format a date")
	}
}
