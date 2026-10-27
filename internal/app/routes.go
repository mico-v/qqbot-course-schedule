package app

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/server"
	"github.com/mico-v/qqbot-course-schedule/internal/webhook"
)

func (a *App) routes(
	verify gin.HandlerFunc,
	dispatcher *webhook.Dispatcher,
	adminService *admin.Service,
) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "version": a.version})
	})
	router.POST("/webhook", verify, dispatcher.Handle)
	server.RegisterImages(router, a.env.ImagesDir)
	server.RegisterFiles(router, a.env.FilesDir)
	server.RegisterAdmin(router, adminService, a.config.AdminPassword)
	return router
}
