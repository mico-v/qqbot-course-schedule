package qqapi

import (
	"errors"
	"fmt"
)

// ErrorCode returns the platform code of an APIError, or 0.
func ErrorCode(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return 0
}

// Known platform error codes.
const (
	CodeActiveMessageRateLimited = 40034100
	CodeBotNotInGroup            = 40034101
	CodeActiveMessageNoPerm      = 40034105
	CodePassiveMsgIDExpired      = 40034005
	CodeMsgIDInvalid             = 40034024
	CodeEventIDExpired           = 40034026
	CodeMediaUploadFailed        = 40034004
	CodeKeyboardInvalid          = 305007
	CodeKeyboardLimit            = 40034029
)

// IsActiveMessageDenied reports a proactive message rejected for permission
// reasons (group switch off, bot removed).
func IsActiveMessageDenied(err error) bool {
	code := ErrorCode(err)
	return code == CodeActiveMessageNoPerm || code == CodeBotNotInGroup
}

// IsRateLimited reports a proactive message rejected by frequency control.
func IsRateLimited(err error) bool {
	return ErrorCode(err) == CodeActiveMessageRateLimited
}

// IsPassiveExpired reports an expired/invalid passive reply target.
func IsPassiveExpired(err error) bool {
	switch ErrorCode(err) {
	case CodePassiveMsgIDExpired, CodeMsgIDInvalid, CodeEventIDExpired:
		return true
	default:
		return false
	}
}

// FriendlyError maps platform errors to hints shown in chat.
func FriendlyError(err error) string {
	switch {
	case IsActiveMessageDenied(err):
		return "平台拒绝主动消息：请在机器人资料页打开「消息推送」后再试。"
	case IsRateLimited(err):
		return "主动消息触发频控，请稍后再试。"
	case IsPassiveExpired(err):
		return "消息已过期（被动回复窗口 5 分钟），请重新发送指令。"
	case ErrorCode(err) == CodeMediaUploadFailed:
		return "图片/文件转存失败，请重试。"
	case ErrorCode(err) == CodeKeyboardLimit:
		return "按钮数量超限，请减少按钮。"
	default:
		return fmt.Sprintf("发送失败：%v", err)
	}
}
