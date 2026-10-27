package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/bot"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/wakeup"
	"github.com/mico-v/qqbot-course-schedule/internal/webhook"
)

func (a *App) build() error {
	client := qqapi.New(&a.config)
	domain := schedule.NewService(a.store)
	a.env = &bot.Env{
		Client:        client,
		PanelStore:    a.store,
		Service:       domain,
		Renderer:      a.renderer,
		DataDir:       a.config.DataDir,
		ImagesDir:     filepath.Join(a.config.DataDir, "images"),
		FilesDir:      filepath.Join(a.config.DataDir, "files"),
		PublicBaseURL: a.config.PublicImageBase(),
		Buttons:       a.config.Buttons,
		WakeUp:        wakeup.NewClient(wakeup.Options{AndroidID: a.config.WakeUpAndroidID}),
	}
	a.handler = bot.NewDefaultHandler(a.env)
	dispatcher := webhook.NewDispatcher(client, a.config.Secret, a.handler)
	verify, err := webhook.Verify(a.config.Secret)
	if err != nil {
		return fmt.Errorf("初始化 webhook 签名校验失败: %w", err)
	}
	adminService := admin.NewService(domain, a.store)
	router := a.routes(verify, dispatcher, adminService)
	a.httpServer = &http.Server{
		Addr:              a.config.ListenAddr(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if cached := render.LoadCachedBotAvatar(a.env.DataDir); cached != nil {
		a.env.SetBotAvatar(cached)
	}
	return nil
}
