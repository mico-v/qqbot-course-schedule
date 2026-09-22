package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

var archiveICSNameRe = regexp.MustCompile(`^schedule[_-](.+)\.ics$`)

type archiveManifestEntry struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	File   string `json:"file"`
}

type archiveManifest struct {
	Version int                    `json:"version"`
	ScopeID string                 `json:"scope_id"`
	Members []archiveManifestEntry `json:"members"`
}

func registerTransferRoutes(api *gin.RouterGroup, service *schedule.Service) {
	api.GET("/export", func(c *gin.Context) {
		scopeID := strings.TrimSpace(c.Query("scope_id"))
		if scopeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope_id 不能为空。"})
			return
		}
		format := c.DefaultQuery("format", "ics")
		userID := strings.TrimSpace(c.Query("user_id"))
		if format != "backup" && userID != "" {
			name, content, found, err := service.WebMemberICS(scopeID, userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if !found {
				c.JSON(http.StatusNotFound, gin.H{"error": "找不到指定成员的课程表。"})
				return
			}
			if content == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "该成员还没有课程可导出。"})
				return
			}
			writeAttachment(c, "text/calendar; charset=utf-8", "schedule-"+safeToken(name)+".ics", []byte(content))
			return
		}
		if format == "backup" {
			backup, err := service.ExportBackup(scopeID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			payload, err := json.MarshalIndent(backup, "", "  ")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			writeAttachment(c, "application/json; charset=utf-8", "course-schedule-backup-"+safeToken(scopeID)+".json", payload)
			return
		}
		members, err := service.ScopeMembers(scopeID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(members) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该会话还没有课表。"})
			return
		}
		payload, err := buildICSArchive(scopeID, members)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		writeAttachment(c, "application/zip", "course-schedule-ics-"+safeToken(scopeID)+".zip", payload)
	})

	api.POST("/import", func(c *gin.Context) {
		scopeID := strings.TrimSpace(c.PostForm("scope_id"))
		if scopeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope_id 不能为空。"})
			return
		}
		userID := strings.TrimSpace(c.PostForm("user_id"))
		fileHeader, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要导入的文件。"})
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败。"})
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, schedule.MaxBackupBytes+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败。"})
			return
		}
		if int64(len(data)) > schedule.MaxBackupBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("文件超过 %d MiB。", schedule.MaxBackupBytes>>20)})
			return
		}

		var result *schedule.ImportResult
		switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
		case ".zip":
			result, err = importICSArchive(service, scopeID, data)
		case ".ics":
			result, err = importSingleICS(service, scopeID, userID, fileHeader.Filename, data)
		case ".json":
			result, err = importBackupFile(service, scopeID, data)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "只支持 .zip / .ics / .json 文件。"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

func buildICSArchive(scopeID string, members map[string]*schedule.Member) ([]byte, error) {
	ids := make([]string, 0, len(members))
	for userID := range members {
		ids = append(ids, userID)
	}
	sort.Strings(ids)

	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	manifest := archiveManifest{Version: 1, ScopeID: scopeID}
	for _, userID := range ids {
		member := members[userID]
		if len(member.Events) == 0 {
			// Nothing to import; empty members only exist in the backup format.
			continue
		}
		content := strings.TrimSpace(member.ICS)
		if content == "" {
			content = schedule.SerializeScheduleICS(member.Events, "", member.Name)
		}
		fileName := "schedule_" + safeToken(userID) + ".ics"
		entry, err := writer.Create(fileName)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			return nil, err
		}
		manifest.Members = append(manifest.Members, archiveManifestEntry{
			UserID: userID, Name: member.Name, File: fileName,
		})
	}
	if len(manifest.Members) == 0 {
		return nil, fmt.Errorf("该会话还没有课程可导出。")
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	entry, err := writer.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err := entry.Write(manifestBytes); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func importICSArchive(service *schedule.Service, scopeID string, data []byte) (*schedule.ImportResult, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("不是有效的 zip 文件。")
	}
	manifest := make(map[string]archiveManifestEntry)
	for _, file := range reader.File {
		if !strings.EqualFold(filepath.Base(file.Name), "manifest.json") {
			continue
		}
		raw, err := readZipEntry(file, 1<<20)
		if err != nil {
			continue
		}
		var parsed archiveManifest
		if err := json.Unmarshal(raw, &parsed); err != nil {
			continue
		}
		for _, entry := range parsed.Members {
			manifest[filepath.Base(entry.File)] = entry
		}
	}

	result := &schedule.ImportResult{}
	entries := 0
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		entries++
		if entries > schedule.MaxBackupMembers+10 {
			break
		}
		base := filepath.Base(file.Name)
		if strings.EqualFold(base, "manifest.json") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(base), ".ics") {
			result.Skipped = append(result.Skipped, base)
			continue
		}
		entry := manifest[base]
		target := strings.TrimSpace(entry.UserID)
		if target == "" {
			if matched := archiveICSNameRe.FindStringSubmatch(base); matched != nil {
				target = matched[1]
			}
		}
		if target == "" {
			result.Skipped = append(result.Skipped, base)
			continue
		}
		raw, err := readZipEntry(file, schedule.MaxICSBytes)
		if err != nil {
			result.Skipped = append(result.Skipped, base)
			continue
		}
		saved, err := service.SaveICS(scopeID, target, entry.Name, string(raw), base, "webui")
		if err != nil {
			result.Skipped = append(result.Skipped, base)
			continue
		}
		result.MemberCount++
		result.EventCount += saved.EventCount
		if saved.Created {
			result.CreatedCount++
		} else {
			result.UpdatedCount++
		}
	}
	if result.MemberCount == 0 && len(result.Skipped) == 0 {
		return nil, fmt.Errorf("压缩包里没有可导入的课表。")
	}
	return result, nil
}

func importSingleICS(service *schedule.Service, scopeID, userID, filename string, data []byte) (*schedule.ImportResult, error) {
	target := strings.TrimSpace(userID)
	if target == "" {
		if matched := archiveICSNameRe.FindStringSubmatch(filepath.Base(filename)); matched != nil {
			target = matched[1]
		}
	}
	if target == "" {
		return nil, fmt.Errorf("请先在左侧选中成员，或把文件命名为 schedule<OpenID>.ics。")
	}
	saved, err := service.SaveICS(scopeID, target, "", string(data), filepath.Base(filename), "webui")
	if err != nil {
		return nil, err
	}
	result := &schedule.ImportResult{MemberCount: 1, EventCount: saved.EventCount}
	if saved.Created {
		result.CreatedCount = 1
	} else {
		result.UpdatedCount = 1
	}
	return result, nil
}

func importBackupFile(service *schedule.Service, scopeID string, data []byte) (*schedule.ImportResult, error) {
	var backup schedule.BackupFile
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("不是有效的备份文件。")
	}
	return service.ImportBackup(scopeID, &backup, "webui")
}

func readZipEntry(file *zip.File, maxBytes int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("entry too large")
	}
	return data, nil
}

func writeAttachment(c *gin.Context, contentType, filename string, data []byte) {
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		safeToken(filename), url.PathEscape(filename)))
	c.Data(http.StatusOK, contentType, data)
}

// safeToken keeps a filename usable in ASCII headers and zip entries.
func safeToken(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_.")
	if result == "" {
		return "scope"
	}
	return result
}
