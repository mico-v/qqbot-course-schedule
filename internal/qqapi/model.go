package qqapi

import "fmt"

// APIError is a non-2xx response from the QQ open platform.
type APIError struct {
	Status  int    `json:"-"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	ErrCode int    `json:"err_code"`
}

func (e *APIError) Error() string {
	if e.ErrCode != 0 {
		return fmt.Sprintf("QQ API 错误: http=%d code=%d err_code=%d message=%s", e.Status, e.Code, e.ErrCode, e.Message)
	}
	return fmt.Sprintf("QQ API 错误: http=%d code=%d message=%s", e.Status, e.Code, e.Message)
}

// SendResult is the response of a message send call.
type SendResult struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
}
