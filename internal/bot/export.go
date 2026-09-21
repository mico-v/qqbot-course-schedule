package bot

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const publicFileTTL = 24 * time.Hour

// SavePublicFile writes an export to data/files and returns its public URL.
func (e *Env) SavePublicFile(content, extension string) (string, error) {
	dir := e.filesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建导出目录失败: %w", err)
	}
	prunePublicFiles(dir)
	name := randomFileName(extension)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("写入导出文件失败: %w", err)
	}
	return strings.TrimRight(e.PublicBaseURL, "/") + "/files/" + name, nil
}

func (e *Env) filesDir() string {
	if e.FilesDir != "" {
		return e.FilesDir
	}
	return filepath.Join(e.DataDir, "files")
}

func randomFileName(extension string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("schedule_%d%s", time.Now().UnixNano(), extension)
	}
	return "schedule_" + hex.EncodeToString(buf) + extension
}

func prunePublicFiles(dir string) {
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
		if now.Sub(info.ModTime()) > publicFileTTL {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// exportFileName is the human-readable name shown in the chat file card.
func exportFileName(memberName string) string {
	name := strings.TrimSpace(memberName)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, name)
	if name == "" {
		name = "成员"
	}
	runes := []rune(name)
	if len(runes) > 40 {
		name = string(runes[:40])
	}
	return name + "-课表.ics"
}
