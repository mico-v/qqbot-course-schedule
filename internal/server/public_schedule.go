package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// registerPublicScheduleRoutes mounts the bearer-token editor API. These
// routes intentionally bypass the admin session because the token itself is
// scoped to one member and expires.
func registerPublicScheduleRoutes(router *gin.Engine, service *admin.Service) {
	public := router.Group("/api/public")
	public.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	})

	public.GET("/schedule", func(c *gin.Context) {
		page, found, err := service.PublicPageSchedule(c.Query("token"), time.Now())
		if err != nil {
			if !writeEditLinkError(c, err) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "找不到该成员的课程表。"})
			return
		}
		c.JSON(http.StatusOK, page)
	})

	public.POST("/schedule/save", func(c *gin.Context) {
		var payload admin.PublicSavePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		result, err := service.SavePublicSchedule(c.Query("token"), payload, time.Now())
		if err != nil {
			if errors.Is(err, schedule.ErrConflict) {
				c.JSON(http.StatusConflict, gin.H{"error": "课表已被其他操作更新，请刷新后重试。"})
				return
			}
			if writeEditLinkError(c, err) {
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

func writeEditLinkError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, schedule.ErrScheduleEditLinkExpired):
		c.JSON(http.StatusGone, gin.H{"error": err.Error()})
	case errors.Is(err, schedule.ErrScheduleEditLinkInvalid):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		return false
	}
	return true
}
