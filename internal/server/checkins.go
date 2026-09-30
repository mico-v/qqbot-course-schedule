package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
)

// registerCheckinRoutes mounts the admin API for check-in records.
func registerCheckinRoutes(api *gin.RouterGroup, service *admin.Service) {
	api.GET("/checkins", func(c *gin.Context) {
		scopeID := strings.TrimSpace(c.Query("scope_id"))
		if scopeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope_id 不能为空。"})
			return
		}
		board, err := service.WebCheckinBoard(scopeID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, board)
	})

	api.POST("/checkins/delete", func(c *gin.Context) {
		var payload struct {
			ScopeID string `json:"scope_id"`
			UserID  string `json:"user_id"`
			Day     string `json:"day"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		deleted, err := service.DeleteWebCheckin(payload.ScopeID, payload.UserID, payload.Day)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": deleted})
	})
}
