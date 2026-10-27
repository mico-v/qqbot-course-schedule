package qqapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/config"
)

type panelAPIStub struct {
	mu      sync.Mutex
	creates []map[string]any
	updates []map[string]any
	deletes []string
	lists   []string
}

func (s *panelAPIStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/getAppAccessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok", "expires_in": "7200"})
	})
	mux.HandleFunc("/v2/panels", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.mu.Lock()
			s.creates = append(s.creates, body)
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"panel_id": "p_1"})
		case http.MethodGet:
			s.mu.Lock()
			s.lists = append(s.lists, r.URL.RawQuery)
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"records": []map[string]any{{
					"panel_id": "p_1", "scope": "group", "target_type": "all",
					"panel": map[string]any{"remark": "qqbot-course-schedule"},
				}},
				"next_cursor": "",
				"is_end":      true,
			})
		}
	})
	mux.HandleFunc("/v2/panels/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.mu.Lock()
			s.updates = append(s.updates, body)
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]int{"version": 2})
		case http.MethodDelete:
			s.mu.Lock()
			s.deletes = append(s.deletes, strings.TrimPrefix(r.URL.Path, "/v2/panels/"))
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	})
	return mux
}

func newPanelClient(t *testing.T) (*Client, *panelAPIStub) {
	t.Helper()
	stub := &panelAPIStub{}
	server := httptest.NewServer(stub.handler())
	t.Cleanup(server.Close)
	cfg := &config.Config{
		Port: 8080, AppID: "1", Secret: "s",
		Domain: server.URL, TokenEndpoint: server.URL + "/app/getAppAccessToken",
	}
	return New(cfg), stub
}

func TestPanelAPICalls(t *testing.T) {
	client, stub := newPanelClient(t)
	ctx := context.Background()

	panelID, err := client.CreatePanel(ctx, "group", "all", nil, nil, Panel{
		Items:  []PanelItem{{Type: "command", Name: "/课表", Desc: "查询"}},
		Remark: "qqbot-course-schedule",
	})
	if err != nil || panelID != "p_1" {
		t.Fatalf("CreatePanel = %q, %v", panelID, err)
	}
	if len(stub.creates) != 1 || stub.creates[0]["scope"] != "group" || stub.creates[0]["target_type"] != "all" {
		t.Fatalf("create body = %+v", stub.creates)
	}

	if err := client.UpdatePanel(ctx, "p_1", Panel{Items: []PanelItem{{Type: "command", Name: "/课表"}}}); err != nil {
		t.Fatalf("UpdatePanel: %v", err)
	}
	if len(stub.updates) != 1 || stub.updates[0]["panel"] == nil {
		t.Fatalf("update body = %+v", stub.updates)
	}

	records, _, isEnd, err := client.ListPanels(ctx, "group", "", 50)
	if err != nil || len(records) != 1 || !isEnd {
		t.Fatalf("ListPanels = %+v %v %v", records, isEnd, err)
	}
	if !strings.Contains(stub.lists[0], "scope=group") || !strings.Contains(stub.lists[0], "limit=50") {
		t.Fatalf("list query = %q", stub.lists[0])
	}

	if err := client.DeletePanel(ctx, "p_1"); err != nil {
		t.Fatalf("DeletePanel: %v", err)
	}
	if len(stub.deletes) != 1 || stub.deletes[0] != "p_1" {
		t.Fatalf("deletes = %v", stub.deletes)
	}
}
