package schedule_test

import (
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

const adminICS = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:math-1
SUMMARY:高等数学
DTSTART;TZID=Asia/Shanghai:20260901T080000
DTEND;TZID=Asia/Shanghai:20260901T093000
LOCATION:教一101
RRULE:FREQ=WEEKLY;BYDAY=TU
END:VEVENT
END:VCALENDAR
`

func newService(t *testing.T) (*schedule.Service, *store.Store) {
	t.Helper()
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "schedule.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })
	return schedule.NewService(storeHandle), storeHandle
}
