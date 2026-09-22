package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// registerOverrideRoutes mounts the admin API for holiday/shift markers.
func registerOverrideRoutes(api *gin.RouterGroup, service *schedule.Service) {
	api.GET("/overrides", func(c *gin.Context) {
		scopeID := strings.TrimSpace(c.Query("scope_id"))
		if scopeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope_id 不能为空。"})
			return
		}
		overrides, err := service.WebDayOverrides(scopeID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"scope_id": scopeID, "overrides": overrides})
	})

	api.POST("/overrides/set", func(c *gin.Context) {
		var payload struct {
			ScopeID   string `json:"scope_id"`
			UserID    string `json:"user_id"`
			Day       string `json:"day"`
			Kind      string `json:"kind"`
			SourceDay string `json:"source_day"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		if err := service.SetWebDayOverride(payload.ScopeID, payload.UserID, payload.Day, payload.Kind, payload.SourceDay, "webui"); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	api.POST("/overrides/delete", func(c *gin.Context) {
		var payload struct {
			ScopeID string `json:"scope_id"`
			UserID  string `json:"user_id"`
			Day     string `json:"day"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		deleted, err := service.DeleteWebDayOverride(payload.ScopeID, payload.UserID, payload.Day)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
	})
}
