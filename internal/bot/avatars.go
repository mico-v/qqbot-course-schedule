package bot

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"sync"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/render"
)

const (
	qqAvatarSize       = 100
	qqAvatarTTL        = 24 * time.Hour
	qqAvatarFailTTL    = 10 * time.Minute
	qqAvatarFetchLimit = 6
)

// qqAvatarURL builds the public QQ avatar URL; tests override it.
var qqAvatarURL = func(qq string) string {
	return fmt.Sprintf("https://q1.qlogo.cn/g?b=qq&nk=%s&s=%d", qq, qqAvatarSize)
}

type avatarCacheEntry struct {
	image   image.Image
	expires time.Time
}

// MemberAvatars resolves real avatars for members with a bound QQ number.
// Bindings and fetch failures are simply absent from the result, so the
// renderer falls back to the initial avatar.
func (e *Env) MemberAvatars(ctx context.Context, scopeID string, userIDs []string) map[string]image.Image {
	if e == nil || e.Service == nil || len(userIDs) == 0 {
		return nil
	}
	bindings, err := e.Service.MemberQQBindings(scopeID)
	if err != nil || len(bindings) == 0 {
		return nil
	}
	qqForUser := make(map[string]string)
	var qqs []string
	seen := make(map[string]bool)
	for _, userID := range userIDs {
		qq := bindings[userID]
		if qq == "" {
			continue
		}
		if _, ok := qqForUser[userID]; ok {
			continue
		}
		qqForUser[userID] = qq
		if !seen[qq] {
			seen[qq] = true
			qqs = append(qqs, qq)
		}
	}
	if len(qqs) == 0 {
		return nil
	}

	images := make(map[string]image.Image, len(qqs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, qqAvatarFetchLimit)
	for _, qq := range qqs {
		wg.Add(1)
		go func(qq string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if img := e.fetchQQAvatar(ctx, qq); img != nil {
				mu.Lock()
				images[qq] = img
				mu.Unlock()
			}
		}(qq)
	}
	wg.Wait()

	result := make(map[string]image.Image, len(qqForUser))
	for userID, qq := range qqForUser {
		if img := images[qq]; img != nil {
			result[userID] = img
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (e *Env) fetchQQAvatar(ctx context.Context, qq string) image.Image {
	if img, ok := e.avatarFromCache(qq); ok {
		return img
	}
	path := filepath.Join(e.DataDir, "avatars", "qq", qq+".png")
	img := render.FetchImage(ctx, qqAvatarURL(qq), path, qqAvatarTTL)
	ttl := qqAvatarTTL
	if img == nil {
		ttl = qqAvatarFailTTL
	}
	e.rememberAvatar(qq, img, ttl)
	return img
}

func (e *Env) avatarFromCache(qq string) (image.Image, bool) {
	e.avatarMu.Lock()
	defer e.avatarMu.Unlock()
	entry, ok := e.avatarCache[qq]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.image, true
}

func (e *Env) rememberAvatar(qq string, img image.Image, ttl time.Duration) {
	e.avatarMu.Lock()
	defer e.avatarMu.Unlock()
	if e.avatarCache == nil {
		e.avatarCache = make(map[string]avatarCacheEntry)
	}
	e.avatarCache[qq] = avatarCacheEntry{image: img, expires: time.Now().Add(ttl)}
}
