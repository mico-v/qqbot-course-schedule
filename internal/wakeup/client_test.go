package wakeup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExtractShareCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"official message", "……分享口令为「aBc123_XyZ」请打开App", "aBc123_XyZ"},
		{"bracket variant", "分享口令为【deadbeef】", "deadbeef"},
		{"bare code", "  a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6  ", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"},
		{"url query", "https://example.com/share?code=abc123", "abc123"},
		{"url path", "https://example.com/share/abc123", "abc123"},
		{"empty", "   ", ""},
	}
	for _, tc := range cases {
		if got := ExtractShareCode(tc.in); got != tc.want {
			t.Errorf("%s: ExtractShareCode = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// fakeGateway completes the antispam handshake for the given token and lets
// the caller shape the share response. The responder receives the RC4 key the
// fake client derived so it can encrypt a well-formed payload.
func fakeGateway(t *testing.T, token string, shareResponder func(key string) (int, string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case antispamPath:
			body, _ := io.ReadAll(r.Body)
			values, err := url.ParseQuery(strings.TrimSuffix(string(body), "&"))
			if err != nil {
				t.Errorf("parse antispam body: %v", err)
			}
			cipher, err := nativeHexDecode(values.Get("data"))
			if err != nil {
				t.Errorf("decode signA: %v", err)
			}
			plain, err := nativeDESDecrypt(cipher, []byte(signAKey))
			if err != nil {
				t.Errorf("decrypt signA: %v", err)
			}
			parts := strings.SplitN(string(plain), "##", 4)
			if len(parts) != 4 {
				t.Fatalf("unexpected signA plaintext %q", plain)
			}
			rand10 := parts[1]
			enc, err := nativeDESEncrypt([]byte(rand10+"##"+token), []byte(rand10[:5]+"#G4"))
			if err != nil {
				t.Errorf("encrypt signB: %v", err)
			}
			writeJSON(w, map[string]any{"errNo": 0, "data": map[string]any{"data": nativeHexEncode(enc)}})
		case sharePath:
			key := nativeGetKey(strconv.Itoa(APKVersionCode), token)
			status, body := shareResponder(key)
			if status != 0 {
				w.WriteHeader(status)
			}
			_, _ = io.WriteString(w, body)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestFetchShareDataRejectsEmptyCode(t *testing.T) {
	client := NewClient(Options{Host: "http://127.0.0.1:0"})
	if _, err := client.FetchShareData(context.Background(), "   "); err == nil {
		t.Fatal("blank code should fail before any request")
	}
}

func TestFetchShareDataAntispamErrors(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantWord string
	}{
		{
			name: "non-200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			wantWord: "HTTP 500",
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "not json")
			},
			wantWord: "不是 JSON",
		},
		{
			name: "errNo set",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{"errNo": 2, "errstr": "declined"})
			},
			wantWord: "declined",
		},
		{
			name: "missing signB",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{"errNo": 0, "data": map[string]any{}})
			},
			wantWord: "缺少 signB",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.handler(w, r)
			}))
			defer server.Close()
			client := NewClient(Options{Host: server.URL})
			_, err := client.FetchShareData(context.Background(), "CODE")
			if err == nil || !strings.Contains(err.Error(), tc.wantWord) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantWord)
			}
		})
	}
}

func TestFetchShareDataShareErrors(t *testing.T) {
	const token = "TOKEN12345"
	cases := []struct {
		name     string
		respond  func(key string) (int, string)
		wantWord string
	}{
		{
			name: "non-200",
			respond: func(string) (int, string) {
				return http.StatusBadGateway, "nope"
			},
			wantWord: "HTTP 502",
		},
		{
			name: "errNo set",
			respond: func(string) (int, string) {
				return http.StatusOK, `{"errNo":3,"errstr":"expired"}`
			},
			wantWord: "expired",
		},
		{
			name: "missing ciphertext",
			respond: func(string) (int, string) {
				return http.StatusOK, `{"errNo":0,"data":{}}`
			},
			wantWord: "缺少密文",
		},
		{
			name: "invalid base64",
			respond: func(string) (int, string) {
				return http.StatusOK, `{"errNo":0,"data":{"data":"!!!notbase64!!!"}}`
			},
			wantWord: "base64",
		},
		{
			name: "decrypted not json",
			respond: func(key string) (int, string) {
				cipher, _ := rc4Crypt([]byte("definitely not json"), []byte(key))
				return http.StatusOK, `{"errNo":0,"data":{"data":"` + base64.StdEncoding.EncodeToString(cipher) + `"}}`
			},
			wantWord: "不是 JSON",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := fakeGateway(t, token, tc.respond)
			defer server.Close()
			client := NewClient(Options{Host: server.URL})
			_, err := client.FetchShareData(context.Background(), "CODE")
			if err == nil || !strings.Contains(err.Error(), tc.wantWord) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantWord)
			}
		})
	}
}

func TestFetchShareDataKeepsCookies(t *testing.T) {
	const token = "TOKEN12345"
	var sawCookie bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case antispamPath:
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc", Path: "/"})
			body, _ := io.ReadAll(r.Body)
			values, _ := url.ParseQuery(strings.TrimSuffix(string(body), "&"))
			cipher, _ := nativeHexDecode(values.Get("data"))
			plain, _ := nativeDESDecrypt(cipher, []byte(signAKey))
			rand10 := strings.SplitN(string(plain), "##", 4)[1]
			enc, _ := nativeDESEncrypt([]byte(rand10+"##"+token), []byte(rand10[:5]+"#G4"))
			writeJSON(w, map[string]any{"errNo": 0, "data": map[string]any{"data": nativeHexEncode(enc)}})
		case sharePath:
			if cookie, err := r.Cookie("sid"); err == nil && cookie.Value == "abc" {
				sawCookie = true
			}
			key := nativeGetKey(strconv.Itoa(APKVersionCode), token)
			cipher, _ := rc4Crypt([]byte(`{"shareData":"ok"}`), []byte(key))
			writeJSON(w, map[string]any{"errNo": 0, "data": map[string]any{"data": base64.StdEncoding.EncodeToString(cipher)}})
		}
	}))
	defer server.Close()

	client := NewClient(Options{Host: server.URL})
	if _, err := client.FetchShareData(context.Background(), "CODE"); err != nil {
		t.Fatalf("FetchShareData: %v", err)
	}
	if !sawCookie {
		t.Fatal("share request did not carry the antispam cookie")
	}
}

func TestNewClientDerivation(t *testing.T) {
	pinned := NewClient(Options{CUID: "PINNED|0"})
	if pinned.cuid != "PINNED|0" {
		t.Errorf("cuid = %q, want PINNED|0", pinned.cuid)
	}
	if pinned.adid != "" {
		t.Errorf("adid = %q, want empty when only CUID is pinned", pinned.adid)
	}

	derived := NewClient(Options{AndroidID: "abc123"})
	if derived.cuid != CUIDFromAndroidID("abc123") {
		t.Errorf("cuid = %q, want derived", derived.cuid)
	}
	if derived.adid != ADIDFromAndroidID("abc123") {
		t.Errorf("adid = %q, want derived", derived.adid)
	}

	def := NewClient(Options{})
	if def.cuid != CUIDFromAndroidID(DefaultAndroidID) || def.adid != ADIDFromAndroidID(DefaultAndroidID) {
		t.Errorf("default client cuid/adid not derived from DefaultAndroidID")
	}
}

// TestFetchShareData drives the full request flow against a fake WakeUp server
// that reimplements the server side with the same primitives.
func TestFetchShareData(t *testing.T) {
	const (
		wantShareData = `{"tableName":"测试","courses":[]}`
		token         = "TOKEN12345"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case antispamPath:
			values, err := url.ParseQuery(strings.TrimSuffix(string(body), "&"))
			if err != nil {
				t.Errorf("parse antispam body: %v", err)
			}
			cipher, err := nativeHexDecode(values.Get("data"))
			if err != nil {
				t.Errorf("decode signA: %v", err)
			}
			plain, err := nativeDESDecrypt(cipher, []byte(signAKey))
			if err != nil {
				t.Errorf("decrypt signA: %v", err)
			}
			parts := strings.SplitN(string(plain), "##", 4)
			if len(parts) != 4 {
				t.Fatalf("unexpected signA plaintext %q", plain)
			}
			rand10 := parts[1]
			signBPlain := rand10 + "##" + token
			enc, err := nativeDESEncrypt([]byte(signBPlain), []byte(rand10[:5]+"#G4"))
			if err != nil {
				t.Errorf("encrypt signB: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errNo": 0,
				"data":  map[string]any{"data": nativeHexEncode(enc)},
			})
		case sharePath:
			key := nativeGetKey(strconv.Itoa(APKVersionCode), token)
			plainJSON := `{"shareData":` + strconv.Quote(wantShareData) + `}`
			cipher, err := rc4Crypt([]byte(plainJSON), []byte(key))
			if err != nil {
				t.Errorf("encrypt share: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errNo": 0,
				"data":  map[string]any{"data": base64.StdEncoding.EncodeToString(cipher)},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Options{Host: server.URL})
	got, err := client.FetchShareData(context.Background(), "SHARECODE")
	if err != nil {
		t.Fatalf("FetchShareData: %v", err)
	}
	if got != wantShareData {
		t.Fatalf("shareData = %q, want %q", got, wantShareData)
	}
}

// TestFetchShareDataLive exercises the real WakeUp API. It is opt-in so the
// normal suite stays hermetic.
func TestFetchShareDataLive(t *testing.T) {
	if os.Getenv("WAKEUP_LIVE") != "1" {
		t.Skip("set WAKEUP_LIVE=1 and WAKEUP_TEST_CODE=<code> to hit the real API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := NewClient(Options{})
	shareData, err := client.FetchShareData(ctx, os.Getenv("WAKEUP_TEST_CODE"))
	if err != nil {
		t.Fatalf("FetchShareData: %v", err)
	}
	t.Logf("shareData = %q", shareData)
}
