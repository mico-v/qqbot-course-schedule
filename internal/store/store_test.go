package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestPutMemberRevisionLifecycle(t *testing.T) {
	store := newTestStore(t)
	member := &schedule.Member{UserID: "U1", Name: "小明", Source: "manual"}

	zero := int64(0)
	if err := store.PutMember("group:G1", "U1", member, &zero); err != nil {
		t.Fatalf("insert: %v", err)
	}
	loaded, found, err := store.GetMember("group:G1", "U1")
	if err != nil || !found {
		t.Fatalf("GetMember found=%v err=%v", found, err)
	}
	if loaded.Revision != 1 || loaded.Name != "小明" {
		t.Fatalf("loaded = %+v", loaded)
	}

	// Insert-only must conflict when the row already exists.
	if err := store.PutMember("group:G1", "U1", member, &zero); !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	// CAS with a stale revision must conflict.
	stale := int64(0)
	if err := store.PutMember("group:G1", "U1", member, &stale); !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("expected stale conflict, got %v", err)
	}

	// Correct revision updates.
	member.Name = "小红"
	current := int64(1)
	if err := store.PutMember("group:G1", "U1", member, &current); err != nil {
		t.Fatalf("update: %v", err)
	}
	loaded, _, _ = store.GetMember("group:G1", "U1")
	if loaded.Revision != 2 || loaded.Name != "小红" {
		t.Fatalf("loaded after update = %+v", loaded)
	}
}

func TestMemberEventsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	event, err := schedule.MakeEvent("高数", "2026-09-01 08:00", "2026-09-01 09:30", "教一", "备注", "FREQ=WEEKLY;BYDAY=TU", "")
	if err != nil {
		t.Fatal(err)
	}
	member := &schedule.Member{UserID: "U1", Name: "小明", Events: []schedule.Event{event}, Source: "ics"}
	if err := store.PutMember("group:G1", "U1", member, nil); err != nil {
		t.Fatalf("PutMember: %v", err)
	}
	loaded, _, err := store.GetMember("group:G1", "U1")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 || loaded.Events[0]["SUMMARY"] != "高数" || loaded.Events[0]["RRULE"] == "" {
		t.Fatalf("events = %+v", loaded.Events)
	}
}

func TestDayOverrideMergeMemberWins(t *testing.T) {
	store := newTestStore(t)
	if err := store.PutMember("group:G1", "U1", &schedule.Member{UserID: "U1", Name: "小明"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDayOverride("group:G1", schedule.DayOverrideAll, "2026-10-01", schedule.DayOverride{Kind: schedule.DayOverrideHoliday}, "admin", schedule.NowISO()); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDayOverride("group:G1", "U1", "2026-10-01", schedule.DayOverride{Kind: schedule.DayOverrideShift, SourceDay: "2026-10-08"}, "admin", schedule.NowISO()); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := store.GetMember("group:G1", "U1")
	if err != nil {
		t.Fatal(err)
	}
	override := loaded.DayOverrides["2026-10-01"]
	if override.Kind != schedule.DayOverrideShift || override.SourceDay != "2026-10-08" {
		t.Fatalf("override = %+v (member must win)", override)
	}

	// Scope-wide marker applies to a member without its own marker.
	if err := store.PutMember("group:G1", "U2", &schedule.Member{UserID: "U2", Name: "小红"}, nil); err != nil {
		t.Fatal(err)
	}
	members, err := store.GetScopeMembers("group:G1")
	if err != nil {
		t.Fatal(err)
	}
	if members["U2"].DayOverrides["2026-10-01"].Kind != schedule.DayOverrideHoliday {
		t.Fatalf("U2 override = %+v", members["U2"].DayOverrides)
	}
}

func TestDeleteDayOverride(t *testing.T) {
	store := newTestStore(t)
	if err := store.SetDayOverride("group:G1", "U1", "2026-10-01", schedule.DayOverride{Kind: schedule.DayOverrideHoliday}, "admin", ""); err != nil {
		t.Fatal(err)
	}
	removed, err := store.DeleteDayOverride("group:G1", "U1", "2026-10-01")
	if err != nil || !removed {
		t.Fatalf("delete = %v %v", removed, err)
	}
	removed, _ = store.DeleteDayOverride("group:G1", "U1", "2026-10-01")
	if removed {
		t.Fatal("second delete should report false")
	}
}

func TestKV(t *testing.T) {
	store := newTestStore(t)
	type value struct {
		Panel string `json:"panel"`
	}
	if err := store.SetKV("global", "panel", "c2c", value{Panel: "p1"}); err != nil {
		t.Fatal(err)
	}
	var loaded value
	found, err := store.GetKV("global", "panel", "c2c", &loaded)
	if err != nil || !found || loaded.Panel != "p1" {
		t.Fatalf("GetKV = %+v %v %v", loaded, found, err)
	}
	if err := store.DeleteKV("global", "panel", "c2c"); err != nil {
		t.Fatal(err)
	}
	found, _ = store.GetKV("global", "panel", "c2c", &loaded)
	if found {
		t.Fatal("value should be gone")
	}
}
