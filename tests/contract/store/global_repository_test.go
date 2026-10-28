package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func TestGlobalScheduleFindAndDeleteContract(t *testing.T) {
	ctx := context.Background()
	handle, err := store.Open(filepath.Join(t.TempDir(), "global-contract.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handle.Close() })
	event, _ := schedule.MakeEvent(
		"高数", "2026-10-28 08:00", "2026-10-28 09:30",
		"", "", "", "event",
	)
	for _, seed := range []struct {
		group string
		union string
		name  string
	}{
		{"CURRENT", "UNION-A", "current"},
		{"SOURCE-A", "UNION-A", "source-a"},
		{"SOURCE-B", "UNION-B", "source-b"},
	} {
		if _, err := handle.PutGroupMemberSchedule(ctx, schedule.GroupMemberKey{
			GroupOpenID: seed.group, MemberOpenID: "M1",
		}, &schedule.Member{
			Name: seed.name, ImportUnionOpenID: seed.union,
			Events: []schedule.Event{event},
		}, 0); err != nil {
			t.Fatal(err)
		}
	}
	exclude := schedule.GroupMemberKey{GroupOpenID: "CURRENT", MemberOpenID: "M1"}
	member, target, found, err := handle.FindGlobalSchedule(
		ctx, "M1", "UNION-A", &exclude,
	)
	if err != nil || !found || target.Key.GroupOpenID != "SOURCE-A" ||
		len(member.Events) != 1 {
		t.Fatalf("FindGlobalSchedule member=%+v target=%+v found=%v err=%v", member, target, found, err)
	}

	deleted, err := handle.DeleteMemberSchedules(ctx, target, 1, exclude, "UNION-A", true)
	if err != nil || deleted < 2 {
		t.Fatalf("DeleteMemberSchedules = %d/%v", deleted, err)
	}
	if _, found, err := handle.GetGroupMemberSchedule(ctx, target.Key); err != nil || found {
		t.Fatalf("global schedule still exists found=%v err=%v", found, err)
	}
	if _, found, err := handle.GetGroupMemberSchedule(ctx, exclude); err != nil || found {
		t.Fatalf("current shadow schedule still exists found=%v err=%v", found, err)
	}

	if deleted, err := handle.DeleteMemberSchedules(
		ctx,
		schedule.NewStorageTarget(exclude),
		9,
		exclude,
		"",
		false,
	); err != nil || deleted != 0 {
		t.Fatalf("deleted missing schedule = %d/%v", deleted, err)
	}
}

func TestGlobalScheduleDeleteRevisionConflict(t *testing.T) {
	ctx := context.Background()
	handle, err := store.Open(filepath.Join(t.TempDir(), "global-conflict.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handle.Close() })
	key := schedule.GroupMemberKey{GroupOpenID: "G1", MemberOpenID: "M1"}
	if _, err := handle.PutGroupMemberSchedule(
		ctx, key, &schedule.Member{Name: "M1"}, 0,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DeleteMemberSchedules(
		ctx, schedule.NewStorageTarget(key), 2, key, "", false,
	); !errors.Is(err, schedule.ErrConflict) {
		t.Fatalf("delete conflict = %v", err)
	}
}
