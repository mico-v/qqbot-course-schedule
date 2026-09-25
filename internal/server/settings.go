package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

// registerSettingsRoutes mounts the admin API for the bot switches.
func registerSettingsRoutes(api *gin.RouterGroup, service *schedule.Service) {
	api.GET("/settings", func(c *gin.Context) {
		settings, err := service.BotSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"settings": settings})
	})

	api.POST("/settings", func(c *gin.Context) {
		var payload schedule.Settings
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		if err := service.SaveBotSettings(payload); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "settings": payload})
	})
}
