package bot

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newAvatarServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewRGBA(image.Rect(0, 0, 200, 200))
		for y := 0; y < 200; y++ {
			for x := 0; x < 200; x++ {
				img.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
			}
		}
		_ = png.Encode(w, img)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRefreshBotAvatar(t *testing.T) {
	avatarServer := newAvatarServer(t)
	fake := newFakeQQ()
	fake.avatarURL = avatarServer.URL + "/bot.png"
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, _ := newTestEnv(t, fake, apiServer.URL)
	env.RefreshBotAvatar(context.Background())
	if env.BotAvatar() == nil {
		t.Fatal("RefreshBotAvatar did not set the avatar")
	}
}

func TestEnsureBotAvatarSelfHeals(t *testing.T) {
	avatarServer := newAvatarServer(t)
	fake := newFakeQQ()
	fake.avatarURL = avatarServer.URL + "/bot.png"
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	env, _ := newTestEnv(t, fake, apiServer.URL)
	env.EnsureBotAvatar()
	deadline := time.Now().Add(5 * time.Second)
	for env.BotAvatar() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if env.BotAvatar() == nil {
		t.Fatal("EnsureBotAvatar did not self-heal a missing avatar")
	}

	// A second call is a no-op once the avatar is present.
	env.EnsureBotAvatar()
}
