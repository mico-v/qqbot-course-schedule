package schedule_test

import (
	"errors"
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
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })
	return schedule.NewService(storeHandle), storeHandle
}

func TestPageScheduleAndSave(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminICS, "schedule.ics", "U1"); err != nil {
		t.Fatalf("SaveICS: %v", err)
	}

	page, found, err := service.PageSchedule(scope, "U1")
	if err != nil || !found {
		t.Fatalf("PageSchedule found=%v err=%v", found, err)
	}
	if page.Name != "小明" || page.Revision != 1 || len(page.Events) != 1 {
		t.Fatalf("page = %+v", page)
	}
	event := page.Events[0]
	if event.Course != "高等数学" || event.Start != "2026-09-01T08:00" || event.End != "2026-09-01T09:30" {
		t.Fatalf("event = %+v", event)
	}
	if event.UID != "math-1" || event.RRule == "" {
		t.Fatalf("event uid/rrule = %+v", event)
	}

	// Save with the revision we read: name + course change, uid preserved.
	name := "数学课代表"
	revision := page.Revision
	result, err := service.SavePageSchedule(schedule.SavePagePayload{
		ScopeID:  scope,
		UserID:   "U1",
		Revision: &revision,
		Name:     &name,
		Events: []schedule.WebEventInput{{
			ID: event.ID, UID: event.UID, Course: "高等数学A",
			Start: event.Start, End: event.End, Location: "教一102", RRule: event.RRule,
		}},
	}, "webui")
	if err != nil {
		t.Fatalf("SavePageSchedule: %v", err)
	}
	if result.Revision != 2 || result.EventCount != 1 || result.Name != "数学课代表" {
		t.Fatalf("result = %+v", result)
	}
	saved, _, _ := service.PageSchedule(scope, "U1")
	if saved.Events[0].Course != "高等数学A" || saved.Events[0].UID != "math-1" {
		t.Fatalf("saved event = %+v", saved.Events[0])
	}
	if saved.Name != "数学课代表" {
		t.Fatalf("saved name = %q", saved.Name)
	}

	// A stale revision must conflict.
	stale := int64(1)
	_, err = service.SavePageSchedule(schedule.SavePagePayload{
		ScopeID: scope, UserID: "U1", Revision: &stale,
		Events: []schedule.WebEventInput{{Course: "冲突", Start: "2026-09-02 08:00", End: "2026-09-02 09:00"}},
	}, "webui")
	if !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("stale save err = %v, want ErrConflict", err)
	}
}

func TestSavePageScheduleValidation(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminICS, "", "U1"); err != nil {
		t.Fatal(err)
	}
	revision := int64(1)

	cases := []struct {
		name    string
		payload schedule.SavePagePayload
	}{
		{"missing scope", schedule.SavePagePayload{UserID: "U1", Revision: &revision}},
		{"missing revision", schedule.SavePagePayload{ScopeID: scope, UserID: "U1"}},
		{"unknown member", schedule.SavePagePayload{ScopeID: scope, UserID: "U9", Revision: &revision}},
		{"no course", schedule.SavePagePayload{ScopeID: scope, UserID: "U1", Revision: &revision,
			Events: []schedule.WebEventInput{{Start: "2026-09-02 08:00", End: "2026-09-02 09:00"}}}},
		{"bad time", schedule.SavePagePayload{ScopeID: scope, UserID: "U1", Revision: &revision,
			Events: []schedule.WebEventInput{{Course: "课", Start: "2026-09-02 09:00", End: "2026-09-02 08:00"}}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := service.SavePageSchedule(testCase.payload, "webui"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCreateMemberSchedulesAndPending(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminICS, "", "U1"); err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateMemberSchedules(scope, []schedule.NewMember{
		{UserID: "U2", Name: "小红"},
		{UserID: "U1", Name: "小明"}, // already exists -> skipped
	}, "webui")
	if err != nil {
		t.Fatalf("CreateMemberSchedules: %v", err)
	}
	if len(created) != 1 || created[0].UserID != "U2" {
		t.Fatalf("created = %+v", created)
	}
	if _, err := service.CreateMemberSchedules(scope, []schedule.NewMember{{UserID: "U1"}}, "webui"); err == nil {
		t.Fatal("creating an existing member should fail when none were created")
	}

	if err := service.RecordSeenMember(scope, "U3", "小刚"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordSeenMember(scope, "U4", "小李"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordSeenMember(scope, "U2", "小红"); err != nil {
		t.Fatal(err)
	}
	pending, err := service.PendingMembers(scope)
	if err != nil {
		t.Fatalf("PendingMembers: %v", err)
	}
	if len(pending) != 2 || pending[0].UserID != "U3" || pending[1].UserID != "U4" {
		t.Fatalf("pending = %+v", pending)
	}

	summaries, err := service.ScopeSummaries()
	if err != nil {
		t.Fatalf("ScopeSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ScopeID != scope || len(summaries[0].Members) != 2 {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestScopeSummariesIncludesSeenOnlyScope(t *testing.T) {
	service, _ := newService(t)
	scope := schedule.ScopeGroup("GSEEN")
	if err := service.RecordSeenMember(scope, "U9", "小九"); err != nil {
		t.Fatal(err)
	}
	summaries, err := service.ScopeSummaries()
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, summary := range summaries {
		if summary.ScopeID == scope {
			seen++
			if len(summary.Members) != 0 {
				t.Fatalf("seen-only scope has members: %+v", summary.Members)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("seen-only scope count = %d, summaries = %+v", seen, summaries)
	}
	pending, err := service.PendingMembers(scope)
	if err != nil || len(pending) != 1 || pending[0].UserID != "U9" {
		t.Fatalf("pending = %+v err=%v", pending, err)
	}

	// A scope with a saved schedule must not be duplicated by the seen list.
	withSchedule := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(withSchedule, "U1", "小明", adminICS, "s.ics", "U1"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordSeenMember(withSchedule, "U1", "小明"); err != nil {
		t.Fatal(err)
	}
	summaries, err = service.ScopeSummaries()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, summary := range summaries {
		if summary.ScopeID == withSchedule {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("scope %s appears %d times", withSchedule, count)
	}
}
