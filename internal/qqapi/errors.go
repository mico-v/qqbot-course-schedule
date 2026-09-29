package qqapi

import "errors"

// ErrorCode returns the platform code of an APIError, or 0.
func ErrorCode(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return 0
}
