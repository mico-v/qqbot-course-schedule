package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

type fakeMenuServer struct {
	mu   sync.Mutex
	menu *qqapi.Menu
	puts int
}

func (f *fakeMenuServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/getAppAccessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok", "expires_in": "7200"})
	})
	mux.HandleFunc("/v2/menu", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			if f.menu == nil {
				_ = json.NewEncoder(w).Encode(map[string]any{"version": 0, "menu": map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "menu": f.menu})
		case http.MethodPut:
			var body struct {
				Menu qqapi.Menu `json:"menu"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.menu = &body.Menu
			f.puts++
			_ = json.NewEncoder(w).Encode(map[string]int{"version": f.puts})
		}
	})
	return mux
}

func TestSyncMenu(t *testing.T) {
	fake := &fakeMenuServer{}
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	cfg := &config.Config{
		Port: 8080, AppID: "1", Secret: "s",
		Domain: server.URL, TokenEndpoint: server.URL + "/app/getAppAccessToken",
	}
	env := &Env{Client: qqapi.New(cfg)}
	ctx := context.Background()

	changed, err := SyncMenu(ctx, env)
	if err != nil || !changed {
		t.Fatalf("first sync changed=%v err=%v", changed, err)
	}
	fake.mu.Lock()
	items := fake.menu.Items
	fake.mu.Unlock()
	if len(items) != 5 || items[0].Name != "今日课表" || items[0].SendMessage != "/今日课表" {
		t.Fatalf("menu items = %+v", items)
	}

	changed, err = SyncMenu(ctx, env)
	if err != nil || changed {
		t.Fatalf("second sync changed=%v err=%v", changed, err)
	}

	// A stale menu on the platform gets overwritten.
	fake.mu.Lock()
	fake.menu.Items = fake.menu.Items[:2]
	fake.mu.Unlock()
	changed, err = SyncMenu(ctx, env)
	if err != nil || !changed {
		t.Fatalf("stale sync changed=%v err=%v", changed, err)
	}
}
