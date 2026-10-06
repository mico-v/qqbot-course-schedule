package schedule_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestScheduleEditLinkRoundTripAndExpiry(t *testing.T) {
	service, storeHandle := newService(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, schedule.LocalTZ)
	token, expiresAt, err := service.CreateScheduleEditLink("group:G1", "U1", 30*time.Minute, now)
	if err != nil {
		t.Fatalf("CreateScheduleEditLink: %v", err)
	}
	if token == "" || !expiresAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("token=%q expiresAt=%v", token, expiresAt)
	}

	entries, err := storeHandle.ListKV("global", schedule.KVNamespaceEditLinks)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Key == token {
		t.Fatalf("stored token entries = %+v; raw token must not be persisted", entries)
	}

	link, err := service.ResolveScheduleEditLink(token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ResolveScheduleEditLink: %v", err)
	}
	if link.ScopeID != "group:G1" || link.UserID != "U1" || !link.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("link = %+v", link)
	}

	if _, err := service.ResolveScheduleEditLink(token, expiresAt); !errors.Is(err, schedule.ErrScheduleEditLinkExpired) {
		t.Fatalf("expired token err = %v", err)
	}
	if _, err := service.ResolveScheduleEditLink(token, now); !errors.Is(err, schedule.ErrScheduleEditLinkInvalid) {
		t.Fatalf("deleted expired token err = %v", err)
	}
	if _, err := service.ResolveScheduleEditLink("not-a-token", now); !errors.Is(err, schedule.ErrScheduleEditLinkInvalid) {
		t.Fatalf("malformed token err = %v", err)
	}
}

func TestScheduleEditLinkRejectsMissingIdentity(t *testing.T) {
	service, _ := newService(t)
	if _, _, err := service.CreateScheduleEditLink("", "U1", time.Hour, time.Now()); err == nil {
		t.Fatal("empty scope should fail")
	}
	if _, _, err := service.CreateScheduleEditLink("group:G1", "U1", 0, time.Now()); err == nil {
		t.Fatal("zero TTL should fail")
	}
}
