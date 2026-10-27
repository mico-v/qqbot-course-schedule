package schedule_test

import (
	"errors"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestErrorKindClassification(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want schedule.ErrorKind
	}{
		{"validation", schedule.ValidationErrorf("bad"), schedule.ErrorValidation},
		{"not found", schedule.NotFoundErrorf("missing"), schedule.ErrorNotFound},
		{"conflict sentinel", schedule.ErrConflict, schedule.ErrorConflict},
		{"edit link expired", schedule.ErrScheduleEditLinkExpired, schedule.ErrorGone},
		{"edit link invalid", schedule.ErrScheduleEditLinkInvalid, schedule.ErrorNotFound},
		{"internal", errors.New("database"), schedule.ErrorInternal},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := schedule.ErrorKindOf(testCase.err); got != testCase.want {
				t.Fatalf("ErrorKindOf = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestLegacyScopeBoundaryConversion(t *testing.T) {
	key, err := schedule.GroupMemberKeyFromLegacy(" group:G1 ", " M1 ")
	if err != nil {
		t.Fatal(err)
	}
	if key.GroupOpenID != "G1" || key.MemberOpenID != "M1" {
		t.Fatalf("key = %+v", key)
	}
	if _, err := schedule.ParseLegacyScopeRef("invalid"); schedule.ErrorKindOf(err) != schedule.ErrorValidation {
		t.Fatalf("invalid scope kind = %q", schedule.ErrorKindOf(err))
	}
}
