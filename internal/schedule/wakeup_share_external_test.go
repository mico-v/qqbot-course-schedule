package schedule_test

import (
	"testing"
)

func TestSaveWakeUpShare(t *testing.T) {
	service, storeHandle := newService(t)

	const shareData = `{"courseLen":50,"id":1,"name":"默认"}
[{"endTime":"08:45","node":1,"startTime":"08:00"}]
{"maxWeek":20,"nodes":1,"startDate":"2026-8-31","tableName":"26秋"}
{"courseName":"高等数学","room":"A101","day":1,"startNode":1,"step":1,"startWeek":1,"endWeek":4}`

	result, err := service.SaveWakeUpShare("group:g1", "u1", "小明", shareData, "wakeup", "u1")
	if err != nil {
		t.Fatalf("SaveWakeUpShare: %v", err)
	}
	if !result.Created || result.EventCount != 1 {
		t.Fatalf("result = %+v", result)
	}

	member, found, err := storeHandle.GetMember("group:g1", "u1")
	if err != nil || !found {
		t.Fatalf("GetMember found=%v err=%v", found, err)
	}
	if member.Source != "wakeup" {
		t.Errorf("Source = %q, want wakeup", member.Source)
	}
	if member.ICS == "" || member.Schedule == "" {
		t.Errorf("ICS/Schedule not populated: %+v", member)
	}

	// A second import replaces the schedule and reports an update.
	result, err = service.SaveWakeUpShare("group:g1", "u1", "小明", shareData, "wakeup", "u1")
	if err != nil {
		t.Fatalf("second SaveWakeUpShare: %v", err)
	}
	if result.Created {
		t.Error("second import should update, not create")
	}
}

func TestSaveWakeUpShareRejectsGarbage(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.SaveWakeUpShare("group:g1", "u1", "小明", "not a share payload", "wakeup", "u1"); err == nil {
		t.Fatal("garbage share data should fail")
	}
}
