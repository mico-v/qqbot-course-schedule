package schedule_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

const canceledContextICS = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:e1\r\n" +
	"SUMMARY:高数\r\n" +
	"DTSTART:20260901T080000\r\n" +
	"DTEND:20260901T093000\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestCanceledContextStopsScheduleIO(t *testing.T) {
	service, _ := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.SaveICSWithSourceContext(
		ctx,
		"group:G1",
		"M1",
		"小明",
		canceledContextICS,
		"test.ics",
		"M1",
		schedule.ImportSource{},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveICSWithSourceContext err = %v, want context.Canceled", err)
	}
}
