package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
	"github.com/mico-v/qqbot-course-schedule/internal/config"
	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

type sentMessage struct {
	Path          string
	Authorization string
	Content       string
	MsgID         string
	MsgSeq        int
	MsgType       int
}

type fakeQQ struct {
	mu      sync.Mutex
	tokens  int
	sends   []sentMessage
	acks    []string
	ackBody string
	sendCh  chan sentMessage
	ackCh   chan string
}

func newFakeQQ() *fakeQQ {
	return &fakeQQ{
		sendCh: make(chan sentMessage, 16),
		ackCh:  make(chan string, 16),
	}
}

func (f *fakeQQ) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/app/getAppAccessToken", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.tokens++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "test-token", "expires_in": "7200"})
	})
	mux.HandleFunc("/v2/groups/", func(w http.ResponseWriter, r *http.Request) {
		f.recordSend(w, r)
	})
	mux.HandleFunc("/v2/users/", func(w http.ResponseWriter, r *http.Request) {
		f.recordSend(w, r)
	})
	mux.HandleFunc("/interactions/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(r.URL.Path)
		f.mu.Lock()
		f.acks = append(f.acks, r.URL.Path)
		f.ackBody = string(body)
		f.mu.Unlock()
		f.ackCh <- r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	})
	return mux
}

func (f *fakeQQ) recordSend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
		MsgID   string `json:"msg_id"`
		MsgSeq  int    `json:"msg_seq"`
		MsgType int    `json:"msg_type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	message := sentMessage{
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
		Content:       body.Content,
		MsgID:         body.MsgID,
		MsgSeq:        body.MsgSeq,
		MsgType:       body.MsgType,
	}
	f.mu.Lock()
	f.sends = append(f.sends, message)
	f.mu.Unlock()
	f.sendCh <- message
	_ = json.NewEncoder(w).Encode(map[string]string{"id": "sent-1"})
}

func (f *fakeQQ) sendCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

func newTestRouter(t *testing.T) (*gin.Engine, *fakeQQ) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	cfg := &config.Config{
		Port:          8080,
		AppID:         "10000",
		Secret:        testSecret,
		Domain:        apiServer.URL,
		TokenEndpoint: apiServer.URL + "/app/getAppAccessToken",
		Database:      "test.db",
		DataDir:       t.TempDir(),
		LogLevel:      "error",
	}
	client := qqapi.New(cfg)
	dispatcher := NewDispatcher(client, cfg.Secret, bot.NewDefaultHandler())
	verify, err := Verify(cfg.Secret)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	router := gin.New()
	router.POST("/webhook", verify, dispatcher.Handle)
	return router, fake
}

func postSigned(t *testing.T, router *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	const timestamp = "1700000000"
	payload := []byte(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-Ed25519", signBody(t, testSecret, timestamp, payload))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func waitForSend(t *testing.T, fake *fakeQQ) sentMessage {
	t.Helper()
	select {
	case message := <-fake.sendCh:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for outgoing message")
		return sentMessage{}
	}
}

func TestGroupPingRepliesPong(t *testing.T) {
	router, fake := newTestRouter(t)

	recorder := postSigned(t, router, `{
		"id": "evt-1", "op": 0, "t": "GROUP_AT_MESSAGE_CREATE",
		"d": {
			"id": "msg-1", "content": "/ping", "group_openid": "GROUP",
			"author": {"id": "U1", "member_openid": "U1", "username": "小明", "member_role": "member", "bot": false}
		}
	}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	message := waitForSend(t, fake)
	if message.Content != "pong" {
		t.Errorf("content = %q, want pong", message.Content)
	}
	if message.MsgID != "msg-1" {
		t.Errorf("msg_id = %q, want msg-1", message.MsgID)
	}
	if message.MsgSeq != 1 {
		t.Errorf("msg_seq = %d, want 1", message.MsgSeq)
	}
	if message.MsgType != 0 {
		t.Errorf("msg_type = %d, want 0", message.MsgType)
	}
	if !strings.HasSuffix(message.Path, "/v2/groups/GROUP/messages") {
		t.Errorf("path = %q", message.Path)
	}
	if message.Authorization != "QQBot test-token" {
		t.Errorf("authorization = %q", message.Authorization)
	}
}

func TestC2CPingRepliesPong(t *testing.T) {
	router, fake := newTestRouter(t)

	postSigned(t, router, `{
		"id": "evt-2", "op": 0, "t": "C2C_MESSAGE_CREATE",
		"d": {
			"id": "msg-2", "content": "/ping",
			"author": {"id": "U2", "user_openid": "U2", "username": "小红", "bot": false}
		}
	}`)

	message := waitForSend(t, fake)
	if message.Content != "pong" {
		t.Errorf("content = %q, want pong", message.Content)
	}
	if !strings.HasSuffix(message.Path, "/v2/users/U2/messages") {
		t.Errorf("path = %q", message.Path)
	}
}

func TestUnknownCommandIsIgnored(t *testing.T) {
	router, fake := newTestRouter(t)

	postSigned(t, router, `{
		"id": "evt-3", "op": 0, "t": "GROUP_AT_MESSAGE_CREATE",
		"d": {
			"id": "msg-3", "content": "你好", "group_openid": "GROUP",
			"author": {"id": "U1", "member_openid": "U1", "bot": false}
		}
	}`)

	time.Sleep(300 * time.Millisecond)
	if fake.sendCount() != 0 {
		t.Fatalf("sent %d messages, want 0", fake.sendCount())
	}
}

func TestDuplicateEventIsProcessedOnce(t *testing.T) {
	router, fake := newTestRouter(t)
	const payload = `{
		"id": "evt-dup", "op": 0, "t": "GROUP_AT_MESSAGE_CREATE",
		"d": {
			"id": "msg-dup", "content": "/ping", "group_openid": "GROUP",
			"author": {"id": "U1", "member_openid": "U1", "bot": false}
		}
	}`

	postSigned(t, router, payload)
	waitForSend(t, fake)
	postSigned(t, router, payload)
	time.Sleep(300 * time.Millisecond)

	if got := fake.sendCount(); got != 1 {
		t.Fatalf("sent %d messages, want 1", got)
	}
}

func TestInteractionIsAcknowledged(t *testing.T) {
	router, fake := newTestRouter(t)

	postSigned(t, router, `{
		"id": "INT-1", "op": 0, "t": "INTERACTION_CREATE",
		"d": {
			"id": "INT-1", "type": 11, "scene": "group",
			"group_openid": "GROUP", "group_member_openid": "U1",
			"data": {"resolved": {"button_id": "prev", "button_data": "day:-1"}}
		}
	}`)

	select {
	case path := <-fake.ackCh:
		if !strings.HasSuffix(path, "/interactions/INT-1") {
			t.Fatalf("acked path = %q", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for interaction ack")
	}
}

func TestValidationOpcodeRespondsWithSignature(t *testing.T) {
	router, _ := newTestRouter(t)

	recorder := postSigned(t, router, `{
		"id": "val-1", "op": 13,
		"d": {"plain_token": "plain-token-value", "event_ts": "1700000000"}
	}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var response struct {
		PlainToken string `json:"plain_token"`
		Signature  string `json:"signature"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.PlainToken != "plain-token-value" {
		t.Errorf("plain_token = %q", response.PlainToken)
	}
	expected, err := SignValidation(testSecret, "1700000000", "plain-token-value")
	if err != nil {
		t.Fatalf("SignValidation: %v", err)
	}
	if response.Signature != expected {
		t.Errorf("signature = %q, want %q", response.Signature, expected)
	}
}
