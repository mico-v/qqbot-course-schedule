package qqapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type mediaUploadResponse struct {
	FileInfo string `json:"file_info"`
	TTL      int    `json:"ttl"`
}

type mediaContent struct {
	FileInfo string `json:"file_info"`
}

type mediaMessage struct {
	MsgType int          `json:"msg_type"`
	Media   mediaContent `json:"media"`
	MsgID   string       `json:"msg_id,omitempty"`
	EventID string       `json:"event_id,omitempty"`
	MsgSeq  int          `json:"msg_seq,omitempty"`
}

// SendGroupImage uploads an image by public URL and sends it to a group.
func (c *Client) SendGroupImage(ctx context.Context, groupOpenID, imageURL, msgID string, msgSeq int) error {
	fileInfo, err := c.uploadMediaByURL(ctx, fmt.Sprintf("/v2/groups/%s/files", url.PathEscape(groupOpenID)), 1, imageURL)
	if err != nil {
		return err
	}
	return c.sendMedia(ctx, fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID)), fileInfo, msgID, "", msgSeq)
}

// SendC2CImage uploads an image by public URL and sends it to a single chat.
func (c *Client) SendC2CImage(ctx context.Context, userOpenID, imageURL, msgID string, msgSeq int) error {
	fileInfo, err := c.uploadMediaByURL(ctx, fmt.Sprintf("/v2/users/%s/files", url.PathEscape(userOpenID)), 1, imageURL)
	if err != nil {
		return err
	}
	return c.sendMedia(ctx, fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID)), fileInfo, msgID, "", msgSeq)
}

// SendGroupImageEvent replies to an interaction event with an image.
func (c *Client) SendGroupImageEvent(ctx context.Context, groupOpenID, imageURL, eventID string) error {
	fileInfo, err := c.uploadMediaByURL(ctx, fmt.Sprintf("/v2/groups/%s/files", url.PathEscape(groupOpenID)), 1, imageURL)
	if err != nil {
		return err
	}
	return c.sendMedia(ctx, fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID)), fileInfo, "", eventID, 1)
}

// SendC2CImageEvent replies to an interaction event with an image.
func (c *Client) SendC2CImageEvent(ctx context.Context, userOpenID, imageURL, eventID string) error {
	fileInfo, err := c.uploadMediaByURL(ctx, fmt.Sprintf("/v2/users/%s/files", url.PathEscape(userOpenID)), 1, imageURL)
	if err != nil {
		return err
	}
	return c.sendMedia(ctx, fmt.Sprintf("/v2/users/%s/messages", url.PathEscape(userOpenID)), fileInfo, "", eventID, 1)
}

// SendGroupFile uploads a file by public URL and sends it to a group.
func (c *Client) SendGroupFile(ctx context.Context, groupOpenID, fileURL, fileName, msgID string, msgSeq int) error {
	fileInfo, err := c.uploadFileByURL(ctx, fmt.Sprintf("/v2/groups/%s/files", url.PathEscape(groupOpenID)), fileURL, fileName)
	if err != nil {
		return err
	}
	return c.sendMedia(ctx, fmt.Sprintf("/v2/groups/%s/messages", url.PathEscape(groupOpenID)), fileInfo, msgID, "", msgSeq)
}

func (c *Client) uploadMediaByURL(ctx context.Context, path string, fileType int, fileURL string) (string, error) {
	body := map[string]any{"file_type": fileType, "url": fileURL, "srv_send_msg": false}
	var result mediaUploadResponse
	if err := c.request(ctx, http.MethodPost, path, body, &result, true); err != nil {
		return "", err
	}
	if result.FileInfo == "" {
		return "", fmt.Errorf("媒体上传失败: 响应缺少 file_info")
	}
	return result.FileInfo, nil
}

func (c *Client) uploadFileByURL(ctx context.Context, path, fileURL, fileName string) (string, error) {
	body := map[string]any{"file_type": 4, "url": fileURL, "srv_send_msg": false}
	if fileName != "" {
		body["file_name"] = fileName
	}
	var result mediaUploadResponse
	if err := c.request(ctx, http.MethodPost, path, body, &result, true); err != nil {
		return "", err
	}
	if result.FileInfo == "" {
		return "", fmt.Errorf("文件上传失败: 响应缺少 file_info")
	}
	return result.FileInfo, nil
}

func (c *Client) sendMedia(ctx context.Context, path, fileInfo, msgID, eventID string, msgSeq int) error {
	if msgID == "" && eventID == "" {
		msgSeq = 0
	}
	body := mediaMessage{MsgType: 7, Media: mediaContent{FileInfo: fileInfo}, MsgID: msgID, EventID: eventID, MsgSeq: msgSeq}
	return c.request(ctx, http.MethodPost, path, body, nil, true)
}
