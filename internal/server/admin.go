// Package server hosts the public HTTP surface: health check, generated card
// images and the admin schedule manager.
package server

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/web"
)

// RegisterAdmin mounts the schedule manager page and its JSON API under /admin
// and /api. When password is empty only loopback clients may access them.
func RegisterAdmin(router *gin.Engine, service *schedule.Service, password string) {
	auth := adminAuth(password)

	admin := router.Group("/admin", auth)
	admin.GET("", serveAsset("index.html", "text/html; charset=utf-8"))
	admin.GET("/", serveAsset("index.html", "text/html; charset=utf-8"))
	admin.GET("/app.js", serveAsset("app.js", "application/javascript; charset=utf-8"))
	admin.GET("/style.css", serveAsset("style.css", "text/css; charset=utf-8"))

	api := router.Group("/api", auth)
	registerTransferRoutes(api, service)
	api.GET("/scopes", func(c *gin.Context) {
		summaries, err := service.ScopeSummaries()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		scopes := make([]gin.H, 0, len(summaries))
		for _, summary := range summaries {
			kind, targetID := schedule.ParseScope(summary.ScopeID)
			members := make([]gin.H, 0, len(summary.Members))
			eventCount := 0
			for _, member := range summary.Members {
				members = append(members, gin.H{
					"user_id":     member.UserID,
					"name":        member.Name,
					"event_count": member.EventCount,
					"revision":    member.Revision,
				})
				eventCount += member.EventCount
			}
			scopes = append(scopes, gin.H{
				"scope_id":     summary.ScopeID,
				"kind":         kind,
				"target_id":    targetID,
				"label":        scopeLabel(kind, targetID),
				"member_count": len(members),
				"event_count":  eventCount,
				"members":      members,
			})
		}
		c.JSON(http.StatusOK, gin.H{"scopes": scopes})
	})

	api.GET("/schedule", func(c *gin.Context) {
		page, found, err := service.PageSchedule(c.Query("scope_id"), c.Query("user_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "找不到指定成员的课程表。"})
			return
		}
		c.JSON(http.StatusOK, page)
	})

	api.POST("/schedule/save", func(c *gin.Context) {
		var payload schedule.SavePagePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		result, err := service.SavePageSchedule(payload, "webui")
		if err != nil {
			if errors.Is(err, schedule.ErrConflict) {
				c.JSON(http.StatusConflict, gin.H{"error": "课表已被其他操作更新，请刷新后重试。"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	})

	api.GET("/members", func(c *gin.Context) {
		scopeID := c.Query("scope_id")
		if scopeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope_id 不能为空。"})
			return
		}
		pending, err := service.PendingMembers(scopeID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		note := ""
		if len(pending) == 0 {
			note = "官方群成员列表为内邀能力，暂不可用；这里只显示与机器人互动过、且还没有课表的成员。"
		}
		c.JSON(http.StatusOK, gin.H{
			"scope_id":     scopeID,
			"members":      pending,
			"member_count": len(pending),
			"note":         note,
		})
	})

	api.POST("/schedule/create", func(c *gin.Context) {
		var payload struct {
			ScopeID string               `json:"scope_id"`
			Members []schedule.NewMember `json:"members"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
			return
		}
		created, err := service.CreateMemberSchedules(payload.ScopeID, payload.Members, "webui")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"scope_id":      payload.ScopeID,
			"created":       created,
			"created_count": len(created),
		})
	})
}

func scopeLabel(kind, targetID string) string {
	switch kind {
	case "group":
		return "群聊 " + targetID
	case "private":
		return "私聊 " + targetID
	default:
		return kind + " " + targetID
	}
}

func serveAsset(name, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := web.FS.ReadFile(name)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		// Admin assets are small; never let a stale app.js survive an upgrade.
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, contentType, data)
	}
}

func adminAuth(password string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if password == "" {
			if !isLoopback(c.ClientIP()) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "未设置 admin_password，管理页面仅允许从服务器本机访问"})
				return
			}
			c.Next()
			return
		}
		user, pass, ok := c.Request.BasicAuth()
		if !ok || user != "admin" || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			c.Header("WWW-Authenticate", `Basic realm="qqbot-course-schedule"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func isLoopback(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}
