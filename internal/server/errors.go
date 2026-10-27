package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// writeAPIError is the single HTTP mapping point for application errors.
func writeAPIError(c *gin.Context, err error) {
	kind := schedule.ErrorKindOf(err)
	status := http.StatusInternalServerError
	message := "服务器内部错误，请稍后重试。"
	switch kind {
	case schedule.ErrorValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case schedule.ErrorUnauthorized:
		status = http.StatusUnauthorized
		message = err.Error()
	case schedule.ErrorForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case schedule.ErrorNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case schedule.ErrorConflict:
		status = http.StatusConflict
		message = err.Error()
	case schedule.ErrorGone:
		status = http.StatusGone
		message = err.Error()
	case schedule.ErrorTooManyRequest:
		status = http.StatusTooManyRequests
		message = err.Error()
	}
	body := gin.H{"error": message, "code": string(kind)}
	if status == http.StatusTooManyRequests {
		c.Header("Retry-After", "60")
	}
	c.JSON(status, body)
}

func writeBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": message,
		"code":  string(schedule.ErrorValidation),
	})
}
