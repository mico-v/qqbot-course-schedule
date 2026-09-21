package render

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageTTL is how long generated cards stay on disk.
const ImageTTL = 24 * time.Hour

// SaveJPEG writes img as a JPEG into dir and returns the generated file name.
// The file name is unguessable so the public image route cannot be enumerated.
func SaveJPEG(img image.Image, dir, baseName string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建图片目录失败: %w", err)
	}
	pruneOldFiles(dir, ImageTTL)
	name := fmt.Sprintf("%s_%s.jpg", sanitizeName(baseName), randomHex(4))
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("创建图片失败: %w", err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 80}); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("编码 JPEG 失败: %w", err)
	}
	return name, nil
}

func pruneOldFiles(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func sanitizeName(name string) string {
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "card"
	}
	return result
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
