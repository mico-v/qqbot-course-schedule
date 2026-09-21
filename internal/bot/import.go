package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
)

const attachmentTimeout = 10 * time.Second

// ImportICS downloads one .ics attachment and stores it as the sender's schedule.
func (e *Env) ImportICS(ctx context.Context, msg *Message, attachment Attachment) error {
	content, err := downloadAttachment(ctx, attachment.URL, schedule.MaxICSBytes)
	if err != nil {
		return msg.Reply(ctx, "下载 .ics 文件失败："+err.Error())
	}
	result, err := e.Service.SaveICS(
		e.Scope(msg), msg.UserOpenID, msg.Username,
		string(content), attachment.Filename, msg.UserOpenID,
	)
	if err != nil {
		return msg.Reply(ctx, err.Error())
	}
	action := "已更新"
	if result.Created {
		action = "已创建"
	}
	return msg.Reply(ctx, fmt.Sprintf("%s %s 的课表：%d 个课程事件。", action, result.Name, result.EventCount))
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
		return nil, fmt.Errorf("网络错误")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取失败")
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("文件超过 %d MiB", maxBytes>>20)
	}
	return data, nil
}
