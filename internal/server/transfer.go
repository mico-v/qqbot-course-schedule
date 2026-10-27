package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

func registerTransferRoutes(api *gin.RouterGroup, service *admin.Service) {
	api.GET("/export", func(c *gin.Context) {
		scopeID := strings.TrimSpace(c.Query("scope_id"))
		if scopeID == "" {
			writeBadRequest(c, "scope_id 不能为空。")
			return
		}
		format := c.DefaultQuery("format", "ics")
		userID := strings.TrimSpace(c.Query("user_id"))
		if format != "backup" && userID != "" {
			name, content, found, err := service.WebMemberICS(scopeID, userID)
			if err != nil {
				writeAPIError(c, err)
				return
			}
			if !found {
				writeAPIError(c, schedule.NotFoundErrorf("找不到指定成员的课程表。"))
				return
			}
			if content == "" {
				writeBadRequest(c, "该成员还没有课程可导出。")
				return
			}
			writeAttachment(
				c,
				"text/calendar; charset=utf-8",
				"schedule-"+admin.SafeTransferToken(name)+".ics",
				[]byte(content),
			)
			return
		}
		if format == "backup" {
			backup, err := service.ExportBackup(scopeID)
			if err != nil {
				writeAPIError(c, err)
				return
			}
			payload, err := json.MarshalIndent(backup, "", "  ")
			if err != nil {
				writeAPIError(c, err)
				return
			}
			writeAttachment(
				c,
				"application/json; charset=utf-8",
				"course-schedule-backup-"+admin.SafeTransferToken(scopeID)+".json",
				payload,
			)
			return
		}
		members, err := service.VisibleMembers(scopeID)
		if err != nil {
			writeAPIError(c, err)
			return
		}
		if len(members) == 0 {
			writeBadRequest(c, "该会话还没有课表。")
			return
		}
		payload, err := service.BuildICSArchive(scopeID, members)
		if err != nil {
			writeAPIError(c, err)
			return
		}
		writeAttachment(
			c,
			"application/zip",
			"course-schedule-ics-"+admin.SafeTransferToken(scopeID)+".zip",
			payload,
		)
	})

	api.POST("/import", func(c *gin.Context) {
		scopeID := strings.TrimSpace(c.PostForm("scope_id"))
		if scopeID == "" {
			writeBadRequest(c, "scope_id 不能为空。")
			return
		}
		userID := strings.TrimSpace(c.PostForm("user_id"))
		fileHeader, err := c.FormFile("file")
		if err != nil {
			writeBadRequest(c, "请选择要导入的文件。")
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			writeBadRequest(c, "读取上传文件失败。")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, admin.MaxBackupBytes+1))
		if err != nil {
			writeBadRequest(c, "读取上传文件失败。")
			return
		}
		if int64(len(data)) > admin.MaxBackupBytes {
			writeBadRequest(c, fmt.Sprintf("文件超过 %d MiB。", admin.MaxBackupBytes>>20))
			return
		}

		var result *admin.ImportResult
		switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
		case ".zip":
			result, err = service.ImportArchive(scopeID, data)
		case ".ics":
			result, err = service.ImportSingleICS(scopeID, userID, fileHeader.Filename, data)
		case ".json":
			result, err = service.ImportBackupFile(scopeID, data, "webui")
		default:
			writeBadRequest(c, "只支持 .zip / .ics / .json 文件。")
			return
		}
		if err != nil {
			writeAPIError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

func writeAttachment(c *gin.Context, contentType, filename string, data []byte) {
	c.Header("Content-Type", contentType)
	c.Header(
		"Content-Disposition",
		fmt.Sprintf(
			`attachment; filename="%s"; filename*=UTF-8''%s`,
			admin.SafeTransferToken(filename),
			url.PathEscape(filename),
		),
	)
	c.Data(http.StatusOK, contentType, data)
}
