package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

type fakePanelServer struct {
	mu      sync.Mutex
	panels  map[string]qqapi.PanelRecord
	nextID  int
	creates int
	updates int
}

func newFakePanelServer() *fakePanelServer {
	return &fakePanelServer{panels: make(map[string]qqapi.PanelRecord)}
}

func (f *fakePanelServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/getAppAccessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok", "expires_in": "7200"})
	})
	mux.HandleFunc("/v2/panels", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Scope      string      `json:"scope"`
				TargetType string      `json:"target_type"`
				Panel      qqapi.Panel `json:"panel"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.nextID++
			id := fmt.Sprintf("p_%d", f.nextID)
			f.panels[id] = qqapi.PanelRecord{
				PanelID: id, Scope: body.Scope, TargetType: body.TargetType, Panel: body.Panel,
			}
			f.creates++
			_ = json.NewEncoder(w).Encode(map[string]string{"panel_id": id})
		case http.MethodGet:
			scope := r.URL.Query().Get("scope")
			var records []qqapi.PanelRecord
			for _, record := range f.panels {
				if record.Scope == scope {
					records = append(records, record)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"records": records, "next_cursor": "", "is_end": true,
			})
		}
	})
	mux.HandleFunc("/v2/panels/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v2/panels/")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			var body struct {
				Panel qqapi.Panel `json:"panel"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			record := f.panels[id]
			record.Panel = body.Panel
			f.panels[id] = record
			f.updates++
			_ = json.NewEncoder(w).Encode(map[string]int{"version": record.Panel.Version + 1})
		case http.MethodDelete:
			delete(f.panels, id)
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	})
	return mux
}

func (f *fakePanelServer) panelForScope(scope string) *qqapi.PanelRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, record := range f.panels {
		if record.Scope == scope {
			copy := record
			return &copy
		}
	}
	return nil
}

func newPanelTestEnv(t *testing.T, fake *fakePanelServer) (*Env, *Handler) {
	t.Helper()
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)

	cfg := &config.Config{
		Port: 8080, AppID: "1", Secret: "s",
		Domain: server.URL, TokenEndpoint: server.URL + "/app/getAppAccessToken",
		DataDir: t.TempDir(),
	}
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })

	env := &Env{Client: qqapi.New(cfg), Store: storeHandle}
	handler := NewDefaultHandler(env)
	return env, handler
}

func TestSyncPanelsCreatesBothScopes(t *testing.T) {
	fake := newFakePanelServer()
	env, handler := newPanelTestEnv(t, fake)

	created, updated, err := SyncPanels(context.Background(), env, handler)
	if err != nil {
		t.Fatalf("SyncPanels: %v", err)
	}
	if created != 2 || updated != 0 {
		t.Fatalf("created=%d updated=%d, want 2/0", created, updated)
	}
	for _, scope := range []string{"c2c", "group"} {
		record := fake.panelForScope(scope)
		if record == nil {
			t.Fatalf("no panel for scope %s", scope)
		}
		if record.TargetType != "all" || record.Panel.Remark != panelRemark {
			t.Errorf("%s record = %+v", scope, record)
		}
		names := make([]string, 0, len(record.Panel.Items))
		for _, item := range record.Panel.Items {
			if item.Type != "command" {
				t.Errorf("item type = %q", item.Type)
			}
			names = append(names, item.Name)
		}
		expected := []string{
			"/今日课表", "/明日课表", "/课表", "/上课时长榜",
			"/休假", "/调休", "/销假", "/假期", "/导入课表", "/导出课表",
			"/启用推送", "/关闭推送", "/help",
		}
		if strings.Join(names, ",") != strings.Join(expected, ",") {
			t.Errorf("%s items = %v, want %v", scope, names, expected)
		}
	}

	// Second run is a no-op thanks to the KV state.
	created, updated, err = SyncPanels(context.Background(), env, handler)
	if err != nil {
		t.Fatalf("second SyncPanels: %v", err)
	}
	if created != 0 || updated != 0 {
		t.Fatalf("second run created=%d updated=%d, want 0/0", created, updated)
	}
	if fake.creates != 2 {
		t.Fatalf("creates = %d, want 2", fake.creates)
	}
}

func TestSyncPanelsUpdatesWhenItemsChange(t *testing.T) {
	fake := newFakePanelServer()
	env, handler := newPanelTestEnv(t, fake)
	if _, _, err := SyncPanels(context.Background(), env, handler); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	handler.Register(&Command{
		Prefix:      "/课表",
		Description: "新的描述",
		Ready:       true,
		Handle:      func(ctx context.Context, msg *Message) error { return nil },
	})

	created, updated, err := SyncPanels(context.Background(), env, handler)
	if err != nil {
		t.Fatalf("update sync: %v", err)
	}
	if created != 0 || updated != 2 {
		t.Fatalf("created=%d updated=%d, want 0/2", created, updated)
	}
	if fake.updates != 2 {
		t.Fatalf("updates = %d, want 2", fake.updates)
	}
	record := fake.panelForScope("group")
	if record == nil || record.Panel.Items[2].Desc != "新的描述" {
		t.Fatalf("panel not updated: %+v", record)
	}
}

func TestSyncPanelsAdoptsExistingAndRecreatesDeleted(t *testing.T) {
	fake := newFakePanelServer()
	env, handler := newPanelTestEnv(t, fake)
	if _, _, err := SyncPanels(context.Background(), env, handler); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// Lose local state: the existing panels must be adopted, not duplicated.
	if err := env.Store.DeleteKV(panelKVScope, panelNamespace, "c2c"); err != nil {
		t.Fatal(err)
	}
	if err := env.Store.DeleteKV(panelKVScope, panelNamespace, "group"); err != nil {
		t.Fatal(err)
	}
	created, _, err := SyncPanels(context.Background(), env, handler)
	if err != nil {
		t.Fatalf("adopt sync: %v", err)
	}
	if created != 0 || fake.creates != 2 {
		t.Fatalf("adopt created=%d creates=%d, want 0/2", created, fake.creates)
	}

	// Delete the group panel on the platform and sync again: it is recreated.
	record := fake.panelForScope("group")
	fake.mu.Lock()
	delete(fake.panels, record.PanelID)
	fake.mu.Unlock()
	if err := env.Store.DeleteKV(panelKVScope, panelNamespace, "group"); err != nil {
		t.Fatal(err)
	}
	created, _, err = SyncPanels(context.Background(), env, handler)
	if err != nil {
		t.Fatalf("recreate sync: %v", err)
	}
	if created != 1 || fake.creates != 3 {
		t.Fatalf("recreate created=%d creates=%d, want 1/3", created, fake.creates)
	}
}

func TestPanelItemsRespectDisplayWidth(t *testing.T) {
	if got := truncateDisplay("生成当前会话今日课程表图片", panelDescLimit); displayColumns(got) > panelDescLimit {
		t.Errorf("desc %q width = %d", got, displayColumns(got))
	}
	if got := truncateDisplay("一个非常非常非常长的指令名字", panelNameLimit); displayColumns(got) > panelNameLimit {
		t.Errorf("name %q width = %d", got, displayColumns(got))
	}
	if got := truncateDisplay("/课表", panelNameLimit); got != "/课表" {
		t.Errorf("short text changed: %q", got)
	}
}
