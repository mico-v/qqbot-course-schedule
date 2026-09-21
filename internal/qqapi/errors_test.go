package qqapi

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestErrorClassification(t *testing.T) {
	denied := &APIError{Status: 400, Code: CodeActiveMessageNoPerm, Message: "无权限"}
	if !IsActiveMessageDenied(denied) || IsRateLimited(denied) || IsPassiveExpired(denied) {
		t.Fatal("permission error misclassified")
	}
	limited := &APIError{Status: 429, Code: CodeActiveMessageRateLimited}
	if !IsRateLimited(limited) {
		t.Fatal("rate limit misclassified")
	}
	expired := &APIError{Status: 400, Code: CodePassiveMsgIDExpired}
	if !IsPassiveExpired(expired) {
		t.Fatal("expired msg_id misclassified")
	}
	wrapped := fmt.Errorf("send: %w", denied)
	if !IsActiveMessageDenied(wrapped) {
		t.Fatal("wrapped error should still classify")
	}
	if ErrorCode(errors.New("plain")) != 0 {
		t.Fatal("plain error should have code 0")
	}
}

func TestFriendlyError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&APIError{Code: CodeActiveMessageNoPerm}, "打开「消息推送」"},
		{&APIError{Code: CodeActiveMessageRateLimited}, "频控"},
		{&APIError{Code: CodePassiveMsgIDExpired}, "过期"},
		{&APIError{Code: CodeMediaUploadFailed}, "转存失败"},
		{&APIError{Code: CodeKeyboardLimit}, "按钮"},
	}
	for _, testCase := range cases {
		if got := FriendlyError(testCase.err); !strings.Contains(got, testCase.want) {
			t.Errorf("FriendlyError(%v) = %q, want contains %q", testCase.err, got, testCase.want)
		}
	}
}
