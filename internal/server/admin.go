// Package server hosts the public HTTP surface: health check, generated card
// images and the admin schedule manager.
package server

import (
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/web"
)

// RegisterAdmin mounts the schedule manager SPA and its JSON API under /admin
// and /api. The SPA bundle is served for both /login and /admin; a successful
// login issues an HttpOnly session cookie. When password is empty only
// loopback clients may access them.
func RegisterAdmin(router *gin.Engine, service *admin.Service, password string) {
	auth := newAdminAuth(password)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/admin")
	})
	router.GET("/login", serveAdminIndex)
	router.GET("/schedule/edit", serveAdminIndex)
	// Hashed build assets stay public: the login page itself loads them before
	// a session exists. Content is immutable per filename.
	router.GET("/admin/assets/*filepath", serveAdminAsset)
	router.POST("/api/login", auth.login)
	router.POST("/api/logout", auth.logout)
	registerPublicScheduleRoutes(router, service)

	adminGroup := router.Group("/admin", auth.middleware())
	adminGroup.GET("", serveAdminIndex)
	adminGroup.GET("/", serveAdminIndex)

	api := router.Group("/api", auth.middleware())
	registerTransferRoutes(api, service)
	registerOverrideRoutes(api, service)
	registerSettingsRoutes(api, service)
	api.GET("/scopes", func(c *gin.Context) {
		summaries, err := service.ScopeSummaries()
		if err != nil {
			writeAPIError(c, err)
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
			pendingCount := 0
			if len(members) == 0 {
				if pending, pendingErr := service.PendingMembers(summary.ScopeID); pendingErr == nil {
					pendingCount = len(pending)
				}
			}
			scopes = append(scopes, gin.H{
				"scope_id":      summary.ScopeID,
				"kind":          kind,
				"target_id":     targetID,
				"label":         scopeLabel(kind, targetID),
				"member_count":  len(members),
				"event_count":   eventCount,
				"pending_count": pendingCount,
				"members":       members,
			})
		}
		c.JSON(http.StatusOK, gin.H{"scopes": scopes})
	})

	api.GET("/schedule", func(c *gin.Context) {
		page, found, err := service.PageScheduleContext(c.Request.Context(), c.Query("scope_id"), c.Query("user_id"))
		if err != nil {
			writeAPIError(c, err)
			return
		}
		if !found {
			writeAPIError(c, schedule.NotFoundErrorf("找不到指定成员的课程表。"))
			return
		}
		c.JSON(http.StatusOK, page)
	})

	api.POST("/schedule/save", func(c *gin.Context) {
		var payload admin.SavePagePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			writeBadRequest(c, "请求体必须是 JSON 对象。")
			return
		}
		result, err := service.SavePageScheduleContext(c.Request.Context(), payload, "webui")
		if err != nil {
			writeAPIError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})

	api.GET("/members", func(c *gin.Context) {
		scopeID := c.Query("scope_id")
		if scopeID == "" {
			writeBadRequest(c, "scope_id 不能为空。")
			return
		}
		pending, err := service.PendingMembers(scopeID)
		if err != nil {
			writeAPIError(c, err)
			return
		}
		note := ""
		if len(pending) == 0 {
			note = "官方群成员列表为内邀能力，暂不可用；这里显示在群里发过言或与机器人互动过、且还没有课表的成员。"
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
			ScopeID string            `json:"scope_id"`
			Members []admin.NewMember `json:"members"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			writeBadRequest(c, "请求体必须是 JSON 对象。")
			return
		}
		created, err := service.CreateMemberSchedules(payload.ScopeID, payload.Members, "webui")
		if err != nil {
			writeAPIError(c, err)
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

// serveAdminIndex serves the SPA shell for both /login and /admin. The file is
// tiny and references hashed assets, so it must never be cached.
func serveAdminIndex(c *gin.Context) {
	data, err := web.FS.ReadFile("dist/index.html")
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// serveAdminAsset serves hashed files from web/dist/assets. embed.FS rejects
// paths escaping dist, so no extra traversal guard is needed.
func serveAdminAsset(c *gin.Context) {
	name := strings.TrimPrefix(c.Param("filepath"), "/")
	if name == "" {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := web.FS.ReadFile("dist/assets/" + name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, assetContentType(name), data)
}

func assetContentType(name string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func isLoopback(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}
