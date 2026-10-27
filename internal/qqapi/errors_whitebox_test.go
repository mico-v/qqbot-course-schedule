package qqapi

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorCode(t *testing.T) {
	apiErr := &APIError{Status: 400, Code: 40034105, Message: "无权限"}
	if got := ErrorCode(apiErr); got != 40034105 {
		t.Fatalf("ErrorCode = %d", got)
	}
	if got := ErrorCode(fmt.Errorf("send: %w", apiErr)); got != 40034105 {
		t.Fatalf("wrapped ErrorCode = %d", got)
	}
	if got := ErrorCode(errors.New("plain")); got != 0 {
		t.Fatalf("plain ErrorCode = %d", got)
	}
}
