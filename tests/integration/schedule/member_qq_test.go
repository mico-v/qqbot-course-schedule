package schedule_test

import (
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func TestNormalizeQQ(t *testing.T) {
	valid := []string{"12345", "1234567890", "12345678901"}
	for _, value := range valid {
		got, err := schedule.NormalizeQQ(" " + value + " ")
		if err != nil || got != value {
			t.Fatalf("NormalizeQQ(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"1234", "012345", "123456789012", "abcde", "123 456"} {
		if _, err := schedule.NormalizeQQ(value); err == nil {
			t.Fatalf("NormalizeQQ(%q) should fail", value)
		}
	}
	if got, err := schedule.NormalizeQQ("  "); err != nil || got != "" {
		t.Fatalf("empty = %q, %v", got, err)
	}
}
