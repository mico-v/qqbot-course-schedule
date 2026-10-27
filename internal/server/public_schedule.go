package server

import (
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
		page, found, err := service.PublicPageScheduleContext(c.Request.Context(), c.Query("token"), time.Now())
		if err != nil {
			writeAPIError(c, err)
			return
		}
		if !found {
			writeAPIError(c, schedule.NotFoundErrorf("找不到该成员的课程表。"))
			return
		}
		c.JSON(http.StatusOK, page)
	})

	public.POST("/schedule/save", func(c *gin.Context) {
		var payload admin.PublicSavePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			writeBadRequest(c, "请求体必须是 JSON 对象。")
			return
		}
		result, err := service.SavePublicScheduleContext(c.Request.Context(), c.Query("token"), payload, time.Now())
		if err != nil {
			writeAPIError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}
