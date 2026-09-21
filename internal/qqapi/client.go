// Package qqapi is a minimal client for the QQ open platform bot HTTP API (v2).
package qqapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
)

const (
	tokenRefreshMargin = 50 * time.Second
	defaultTimeout     = 10 * time.Second
	maxResponseBytes   = 1 << 20 // 1 MiB
	maxAttempts        = 3
)

// Client talks to the QQ open platform on behalf of one bot application.
type Client struct {
	cfg  *config.Config
	http *http.Client

	mu       sync.RWMutex
	token    string
	expireAt time.Time
}

// New returns a client using the given configuration.
func New(cfg *config.Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: defaultTimeout},
	}
}

// AccessToken returns a cached token, refreshing it shortly before it expires.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.expireAt) {
		token := c.token
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Another goroutine may have refreshed while we waited for the lock.
	if c.token != "" && time.Now().Before(c.expireAt) {
		return c.token, nil
	}

	var resp struct {
		AccessToken string  `json:"access_token"`
		ExpiresIn   flexInt `json:"expires_in"`
	}
	body := map[string]string{"appId": c.cfg.AppID, "clientSecret": c.cfg.Secret}
	if err := c.request(ctx, http.MethodPost, c.cfg.TokenEndpoint, body, &resp, false); err != nil {
		return "", fmt.Errorf("获取 AccessToken 失败: %w", err)
	}
	if resp.AccessToken == "" {
		return "", errors.New("获取 AccessToken 失败: 响应中没有 access_token")
	}
	expires := time.Duration(resp.ExpiresIn) * time.Second
	if expires <= tokenRefreshMargin {
		expires = 2 * time.Minute
	}
	c.token = resp.AccessToken
	c.expireAt = time.Now().Add(expires - tokenRefreshMargin)
	slog.Debug("access token refreshed", "expires_in", expires.String())
	return c.token, nil
}

// request performs one API call with retries for rate limits and server errors.
// When authorized is false the Authorization header is omitted (token endpoint).
func (c *Client) request(ctx context.Context, method, rawURL string, body, out any, authorized bool) error {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("编码请求体失败: %w", err)
		}
		payload = encoded
	}

	endpoint := rawURL
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimRight(c.cfg.Domain, "/") + endpoint
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt-1) * 300 * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("构造请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			token, err := c.AccessToken(ctx)
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "QQBot "+token)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s %s 请求失败: %w", method, safePath(endpoint), err)
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("%s %s 读取响应失败: %w", method, safePath(endpoint), readErr)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = parseAPIError(resp.StatusCode, raw)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return parseAPIError(resp.StatusCode, raw)
		}
		if out != nil {
			if err := json.Unmarshal(raw, out); err != nil {
				return fmt.Errorf("解析 %s %s 响应失败: %w", method, safePath(endpoint), err)
			}
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("请求失败")
	}
	return lastErr
}

func parseAPIError(status int, raw []byte) error {
	apiErr := &APIError{Status: status}
	if err := json.Unmarshal(raw, apiErr); err != nil || (apiErr.Code == 0 && apiErr.Message == "") {
		message := strings.TrimSpace(string(raw))
		if len(message) > 200 {
			message = message[:200]
		}
		apiErr.Message = message
	}
	return apiErr
}

func safePath(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	segments := strings.Split(parsed.Path, "/")
	for i, segment := range segments {
		if len(segment) > 12 && !strings.ContainsAny(segment, ".{") {
			segments[i] = segment[:6] + "…"
		}
	}
	return strings.Join(segments, "/")
}

// flexInt accepts a JSON number or a numeric string.
type flexInt int

func (f *flexInt) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return fmt.Errorf("无法解析整数 %q: %w", text, err)
	}
	*f = flexInt(value)
	return nil
}
