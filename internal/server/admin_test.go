package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mico-v/qqbot-course-schedule/internal/admin"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

const adminTestICS = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:math-1
SUMMARY:高等数学
DTSTART;TZID=Asia/Shanghai:20260901T080000
DTEND;TZID=Asia/Shanghai:20260901T093000
END:VEVENT
END:VCALENDAR
`

const adminPassword = "test-password"

func newAdminRouter(t *testing.T, password string) (*gin.Engine, *admin.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	storeHandle, err := store.Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { storeHandle.Close() })
	domainService := schedule.NewService(storeHandle)
	service := admin.NewService(domainService, storeHandle)
	router := gin.New()
	RegisterAdmin(router, service, password)
	return router, service
}

func newAdminRequest(method, path string, body any) *http.Request {
	var reader *bytes.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	return req
}

// loginCookie performs the real login round-trip so tests exercise the same
// cookie session path as the browser.
func loginCookie(router *gin.Engine, password string) *http.Cookie {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, newAdminRequest(http.MethodPost, "/api/login", map[string]string{"password": password}))
	if recorder.Code != http.StatusOK {
		panic(fmt.Sprintf("test login = %d: %s", recorder.Code, recorder.Body.String()))
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == adminSessionCookie {
			return cookie
		}
	}
	panic("test login response missing session cookie")
}

func adminRequest(router *gin.Engine, method, path string, body any, withAuth bool) *httptest.ResponseRecorder {
	req := newAdminRequest(method, path, body)
	if withAuth {
		req.AddCookie(loginCookie(router, adminPassword))
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestAdminRequiresPassword(t *testing.T) {
	router, _ := newAdminRouter(t, adminPassword)

	// 登录页与登录接口本身无需会话。
	recorder := adminRequest(router, http.MethodGet, "/login", nil, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login page = %d, want 200", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("课表管理")) {
		t.Fatal("login page body missing SPA title")
	}

	// 未登录访问页面：跳转登录页并携带返回地址。
	recorder = adminRequest(router, http.MethodGet, "/admin", nil, false)
	if recorder.Code != http.StatusFound {
		t.Fatalf("unauthenticated /admin = %d, want 302", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/login?next=%2Fadmin" {
		t.Fatalf("redirect location = %q", location)
	}

	// 未登录调用 API：JSON 401，且不得触发 Basic Auth 弹窗。
	recorder = adminRequest(router, http.MethodGet, "/api/scopes", nil, false)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/scopes = %d, want 401", recorder.Code)
	}
	if header := recorder.Header().Get("WWW-Authenticate"); header != "" {
		t.Fatalf("unexpected WWW-Authenticate header %q", header)
	}

	// 密码错误。
	recorder = adminRequest(router, http.MethodPost, "/api/login", map[string]string{"password": "wrong-password"}, false)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password login = %d, want 401", recorder.Code)
	}

	// 密码正确：下发 HttpOnly 会话 Cookie。
	recorder = adminRequest(router, http.MethodPost, "/api/login", map[string]string{"password": adminPassword}, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", recorder.Code)
	}
	var session *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == adminSessionCookie {
			session = cookie
		}
	}
	if session == nil || session.Value == "" {
		t.Fatal("login response missing session cookie")
	}
	if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || session.MaxAge <= 0 {
		t.Fatalf("session cookie attributes = %+v", session)
	}

	// 携带会话后可访问页面与 API。
	recorder = adminRequest(router, http.MethodGet, "/admin", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated /admin = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("课表管理")) {
		t.Fatal("admin page body missing title")
	}
	if recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, true); recorder.Code != http.StatusOK {
		t.Fatalf("authenticated /api/scopes = %d, want 200", recorder.Code)
	}
}

func TestAdminLogoutInvalidatesSession(t *testing.T) {
	router, _ := newAdminRouter(t, adminPassword)
	cookie := loginCookie(router, adminPassword)

	logout := newAdminRequest(http.MethodPost, "/api/logout", nil)
	logout.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, logout)
	if recorder.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", recorder.Code)
	}

	reuse := newAdminRequest(http.MethodGet, "/api/scopes", nil)
	reuse.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, reuse)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("reused session = %d, want 401", recorder.Code)
	}
}

func TestAdminLoginRateLimit(t *testing.T) {
	router, _ := newAdminRouter(t, adminPassword)
	for attempt := 0; attempt < loginMaxFailures; attempt++ {
		recorder := adminRequest(router, http.MethodPost, "/api/login", map[string]string{"password": "wrong-password"}, false)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", attempt+1, recorder.Code)
		}
	}
	recorder := adminRequest(router, http.MethodPost, "/api/login", map[string]string{"password": adminPassword}, false)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked login = %d, want 429", recorder.Code)
	}
}

func TestAdminLoopbackOnlyWithoutPassword(t *testing.T) {
	router, _ := newAdminRouter(t, "")

	remote := httptest.NewRequest(http.MethodGet, "/api/scopes", nil)
	remote.RemoteAddr = "192.0.2.10:12345"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, remote)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("remote request = %d, want 403", recorder.Code)
	}

	if recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, false); recorder.Code != http.StatusOK {
		t.Fatalf("loopback request = %d, want 200", recorder.Code)
	}
}

func TestAdminAPIFlow(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	if _, err := service.SaveICS(scope, "U1", "小明", adminTestICS, "schedule.ics", "U1"); err != nil {
		t.Fatalf("SaveICS: %v", err)
	}

	// scopes
	recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("scopes = %d", recorder.Code)
	}
	var scopesResponse struct {
		Scopes []struct {
			ScopeID     string `json:"scope_id"`
			Kind        string `json:"kind"`
			MemberCount int    `json:"member_count"`
			EventCount  int    `json:"event_count"`
			Members     []struct {
				UserID     string `json:"user_id"`
				EventCount int    `json:"event_count"`
				Revision   int64  `json:"revision"`
			} `json:"members"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &scopesResponse); err != nil {
		t.Fatalf("decode scopes: %v", err)
	}
	if len(scopesResponse.Scopes) != 1 || scopesResponse.Scopes[0].Kind != "group" ||
		scopesResponse.Scopes[0].MemberCount != 1 || scopesResponse.Scopes[0].EventCount != 1 {
		t.Fatalf("scopes = %+v", scopesResponse)
	}

	// schedule
	recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U1", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("schedule = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var page admin.PageSchedule
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if page.Revision != 1 || len(page.Events) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U9", nil, true); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing member = %d, want 404", recorder.Code)
	}

	// save
	saveBody := map[string]any{
		"scope_id": scope, "user_id": "U1", "revision": page.Revision, "name": "小明",
		"events": []map[string]any{{
			"id": 1, "uid": "math-1", "course": "高等数学A",
			"start": "2026-09-01T08:00", "end": "2026-09-01T09:30",
		}},
	}
	recorder = adminRequest(router, http.MethodPost, "/api/schedule/save", saveBody, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save = %d (%s)", recorder.Code, recorder.Body.String())
	}
	// stale revision -> 409
	recorder = adminRequest(router, http.MethodPost, "/api/schedule/save", saveBody, true)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale save = %d, want 409 (%s)", recorder.Code, recorder.Body.String())
	}

	// members: only observed members without a schedule
	if recorder = adminRequest(router, http.MethodGet, "/api/members?scope_id="+scope, nil, true); recorder.Code != http.StatusOK {
		t.Fatalf("members = %d", recorder.Code)
	}
	var membersResponse struct {
		Members     []admin.NewMember `json:"members"`
		MemberCount int               `json:"member_count"`
		Note        string            `json:"note"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &membersResponse); err != nil {
		t.Fatal(err)
	}
	if membersResponse.MemberCount != 0 || membersResponse.Note == "" {
		t.Fatalf("members = %+v", membersResponse)
	}

	if err := service.RecordSeenMember(scope, "U2", "小红"); err != nil {
		t.Fatal(err)
	}
	recorder = adminRequest(router, http.MethodGet, "/api/members?scope_id="+scope, nil, true)
	if err := json.Unmarshal(recorder.Body.Bytes(), &membersResponse); err != nil {
		t.Fatal(err)
	}
	if membersResponse.MemberCount != 1 || membersResponse.Members[0].UserID != "U2" {
		t.Fatalf("members after seen = %+v", membersResponse)
	}

	// create an empty schedule for the observed member
	createBody := map[string]any{
		"scope_id": scope,
		"members":  []map[string]string{{"user_id": "U2", "name": "小红"}},
	}
	recorder = adminRequest(router, http.MethodPost, "/api/schedule/create", createBody, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create = %d (%s)", recorder.Code, recorder.Body.String())
	}
	var createResponse struct {
		CreatedCount int `json:"created_count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &createResponse); err != nil {
		t.Fatal(err)
	}
	if createResponse.CreatedCount != 1 {
		t.Fatalf("create response = %+v", createResponse)
	}
	recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U2", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("created member schedule = %d", recorder.Code)
	}
}

func TestAdminAssetsAndTrailingSlash(t *testing.T) {
	router, _ := newAdminRouter(t, adminPassword)

	for _, path := range []string{"/admin", "/admin/"} {
		recorder := adminRequest(router, http.MethodGet, path, nil, true)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s = %d, want 200", path, recorder.Code)
		}
		if contentType := recorder.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Fatalf("%s content type = %q", path, contentType)
		}
	}

	// 页面必须用绝对 /admin 路径引用带哈希的构建产物，且产物无需登录即可加载
	// （登录页本身在建立会话前就要用到它们）。
	recorder := adminRequest(router, http.MethodGet, "/admin", nil, true)
	body := recorder.Body.String()
	assets := adminAssetPathRe.FindAllString(body, -1)
	if len(assets) == 0 {
		t.Fatalf("admin page must reference hashed assets by absolute /admin path")
	}
	for _, asset := range assets {
		assetRecorder := adminRequest(router, http.MethodGet, asset, nil, false)
		if assetRecorder.Code != http.StatusOK {
			t.Fatalf("%s = %d, want 200", asset, assetRecorder.Code)
		}
		if cache := assetRecorder.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
			t.Fatalf("%s cache control = %q", asset, cache)
		}
	}
}

var adminAssetPathRe = regexp.MustCompile(`/admin/assets/[A-Za-z0-9._-]+`)

func TestScopesIncludeSeenOnlyGroup(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("GSEEN")
	if err := service.RecordSeenMember(scope, "U9", "小九"); err != nil {
		t.Fatalf("RecordSeenMember: %v", err)
	}

	recorder := adminRequest(router, http.MethodGet, "/api/scopes", nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("scopes = %d", recorder.Code)
	}
	var response struct {
		Scopes []struct {
			ScopeID      string `json:"scope_id"`
			MemberCount  int    `json:"member_count"`
			PendingCount int    `json:"pending_count"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Scopes) != 1 || response.Scopes[0].ScopeID != scope ||
		response.Scopes[0].MemberCount != 0 || response.Scopes[0].PendingCount != 1 {
		t.Fatalf("scopes = %+v", response.Scopes)
	}

	recorder = adminRequest(router, http.MethodGet, "/api/members?scope_id="+scope, nil, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("members = %d", recorder.Code)
	}
	var members struct {
		Members []admin.NewMember `json:"members"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &members); err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 1 || members.Members[0].UserID != "U9" || members.Members[0].Name != "小九" {
		t.Fatalf("members = %+v", members.Members)
	}
}

func TestSaveScheduleWithQQ(t *testing.T) {
	router, service := newAdminRouter(t, adminPassword)
	scope := schedule.ScopeGroup("G1")
	seedScope(t, service, scope)

	recorder := adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U1", nil, true)
	var page admin.PageSchedule
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.QQ != "" {
		t.Fatalf("initial qq = %q", page.QQ)
	}

	save := func(qq string) *httptest.ResponseRecorder {
		return adminRequest(router, http.MethodPost, "/api/schedule/save", map[string]any{
			"scope_id": scope, "user_id": "U1", "revision": page.Revision, "name": "小明", "qq": qq,
			"events": []map[string]any{{
				"id": 1, "course": "高等数学", "start": "2026-09-01T08:00", "end": "2026-09-01T09:30",
			}},
		}, true)
	}
	if recorder := save("123456789"); recorder.Code != http.StatusOK {
		t.Fatalf("save = %d (%s)", recorder.Code, recorder.Body.String())
	}
	recorder = adminRequest(router, http.MethodGet, "/api/schedule?scope_id="+scope+"&user_id=U1", nil, true)
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.QQ != "123456789" {
		t.Fatalf("saved qq = %q", page.QQ)
	}
	if recorder := save("not-a-qq"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid qq save = %d, want 400", recorder.Code)
	}
}
