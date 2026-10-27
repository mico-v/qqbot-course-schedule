package schedule_test

import (
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestGroupMemberIdentityIncludesGroup(t *testing.T) {
	first := schedule.GroupMemberKey{GroupOpenID: "G1", MemberOpenID: "M1"}
	second := schedule.GroupMemberKey{GroupOpenID: "G2", MemberOpenID: "M1"}

	if first == second {
		t.Fatal("same member_openid in different groups must produce different identities")
	}
	if first.Group() != (schedule.GroupKey{GroupOpenID: "G1"}) {
		t.Fatalf("group = %+v", first.Group())
	}
}

func TestGroupMemberKeyFromLegacyRejectsNonGroupScope(t *testing.T) {
	_, err := schedule.GroupMemberKeyFromLegacy("private:U1", "U1")
	if schedule.ErrorKindOf(err) != schedule.ErrorValidation {
		t.Fatalf("kind = %q, want validation", schedule.ErrorKindOf(err))
	}
}

func TestGroupMemberKeyValidation(t *testing.T) {
	testCases := []struct {
		name string
		key  schedule.GroupMemberKey
		ok   bool
	}{
		{name: "valid", key: schedule.GroupMemberKey{GroupOpenID: "G1", MemberOpenID: "M1"}, ok: true},
		{name: "missing group", key: schedule.GroupMemberKey{MemberOpenID: "M1"}},
		{name: "missing member", key: schedule.GroupMemberKey{GroupOpenID: "G1"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.key.Validate()
			if testCase.ok && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !testCase.ok && schedule.ErrorKindOf(err) != schedule.ErrorValidation {
				t.Fatalf("kind = %q, want validation", schedule.ErrorKindOf(err))
			}
		})
	}
}
