package bot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

const attachmentTimeout = 10 * time.Second

// ImportICS downloads one .ics attachment and stores it as the sender's schedule.
func (e *Env) ImportICS(ctx context.Context, msg *Message, attachment Attachment) error {
	data, err := downloadAttachment(ctx, attachment.URL, schedule.MaxICSBytes)
	if err != nil {
		slog.Warn("下载课表文件失败", "filename", attachment.Filename, "err", err)
		return msg.Reply(ctx, "下载 .ics 文件失败："+err.Error())
	}
	content := schedule.DecodeICSBytes(data)
	slog.Info("收到课表文件",
		"filename", attachment.Filename,
		"bytes", len(data),
		"content_type", attachment.ContentType,
		"origin", msg.Origin,
	)

	result, err := e.Service.SaveICS(
		e.Scope(msg), msg.UserOpenID, msg.Username,
		content, attachment.Filename, msg.UserOpenID,
	)
	if err != nil {
		slog.Warn("导入课表失败", "filename", attachment.Filename, "bytes", len(data), "err", err)
		e.saveFailedICS(attachment.Filename, data)
		return msg.Reply(ctx, err.Error())
	}
	slog.Info("导入课表成功",
		"filename", attachment.Filename,
		"events", result.EventCount,
		"created", result.Created,
		"name", result.Name,
	)
	action := "已更新"
	if result.Created {
		action = "已创建"
	}
	return msg.Reply(ctx, fmt.Sprintf("%s %s 的课表：%d 个课程事件。", action, result.Name, result.EventCount))
}

// saveFailedICS keeps the raw upload so a parse failure can be inspected later.
func (e *Env) saveFailedICS(filename string, data []byte) {
	if e.DataDir == "" {
		return
	}
	dir := filepath.Join(e.DataDir, "failed_ics")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	name := time.Now().Format("20060102_150405") + "_" + sanitizeFilename(filename)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		slog.Warn("保存失败文件出错", "err", err)
		return
	}
	slog.Info("失败文件已留存", "path", filepath.Join(dir, name))
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "upload.ics"
	}
	return result
}

func isICSFile(attachment Attachment) bool {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(attachment.Filename)), ".ics") {
		return true
	}
	return strings.Contains(strings.ToLower(attachment.ContentType), "calendar")
}

func downloadAttachment(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	client := &http.Client{Timeout: attachmentTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("请求地址无效")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取失败: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("文件超过 %d MiB", maxBytes>>20)
	}
	return data, nil
}
