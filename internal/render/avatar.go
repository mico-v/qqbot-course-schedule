package render

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	botAvatarTTL     = 24 * time.Hour
	botAvatarTimeout = 3 * time.Second
	maxAvatarBytes   = 5 << 20
	botAvatarSize    = 88
)

// FetchBotAvatar downloads the bot's own avatar (URL comes from GET /users/@me),
// caching it under dataDir/avatars for 24h. It returns nil on any failure so a
// missing avatar never blocks rendering.
func FetchBotAvatar(ctx context.Context, avatarURL, dataDir string) *image.RGBA {
	avatarURL = stringTrimSpace(avatarURL)
	if avatarURL == "" {
		return nil
	}
	cachePath := filepath.Join(dataDir, "avatars", "bot.png")
	if cached := readFreshImage(cachePath, botAvatarTTL); cached != nil {
		return circleImage(cached, botAvatarSize)
	}
	client := &http.Client{Timeout: botAvatarTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	downloaded, _, err := image.Decode(io.LimitReader(resp.Body, maxAvatarBytes))
	if err != nil {
		return nil
	}
	writeImageCache(cachePath, downloaded)
	return circleImage(downloaded, botAvatarSize)
}

func readFreshImage(path string, maxAge time.Duration) image.Image {
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > maxAge {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil
	}
	return img
}

func writeImageCache(path string, img image.Image) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, time.Now().UnixNano())
	file, err := os.Create(tmp)
	if err != nil {
		return
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		os.Remove(tmp)
		return
	}
	file.Close()
	_ = os.Rename(tmp, path)
}

func stringTrimSpace(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\n' || value[0] == '\r') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != ' ' && last != '\t' && last != '\n' && last != '\r' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}
