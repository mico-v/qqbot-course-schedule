package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	adminSessionCookie = "qqbot_admin_session"
	adminSessionTTL    = 7 * 24 * time.Hour

	loginFailWindow    = 10 * time.Minute
	loginMaxFailures   = 8
	loginBlockDuration = 15 * time.Minute
)

// adminAuth guards the admin page and its API with an in-memory cookie
// session. Sessions are random tokens that die with the process; passwords
// never reach the browser.
type adminAuth struct {
	password string

	mu       sync.Mutex
	sessions map[string]time.Time
	failures map[string]*loginFailure
}

type loginFailure struct {
	count        int
	first        time.Time
	blockedUntil time.Time
}

func newAdminAuth(password string) *adminAuth {
	return &adminAuth{
		password: password,
		sessions: make(map[string]time.Time),
		failures: make(map[string]*loginFailure),
	}
}

func (a *adminAuth) enabled() bool { return a.password != "" }

// middleware allows requests carrying a valid session cookie. Without a
// password only loopback clients pass (unchanged local-development escape
// hatch). Browser navigations are redirected to the login page while API
// calls receive JSON 401 so the frontend can react.
func (a *adminAuth) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.enabled() {
			if !isLoopback(c.ClientIP()) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "未设置 admin_password，管理页面仅允许从服务器本机访问"})
				return
			}
			c.Next()
			return
		}
		if token, err := c.Cookie(adminSessionCookie); err == nil && a.validate(token) {
			c.Next()
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期，请重新登录。"})
			return
		}
		target := "/login"
		if next := c.Request.URL.RequestURI(); next != "" && next != "/" {
			target += "?next=" + url.QueryEscape(next)
		}
		c.Redirect(http.StatusFound, target)
		c.Abort()
	}
}

func (a *adminAuth) login(c *gin.Context) {
	if !a.enabled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未设置 admin_password，无需登录。"})
		return
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象。"})
		return
	}
	ip := c.ClientIP()
	if wait := a.blockRemaining(ip); wait > 0 {
		c.Header("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": fmt.Sprintf("密码错误次数过多，请 %.0f 分钟后再试。", wait.Minutes())})
		return
	}
	if subtle.ConstantTimeCompare([]byte(payload.Password), []byte(a.password)) != 1 {
		a.recordFailure(ip)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误。"})
		return
	}
	a.resetFailures(ip)
	token := a.createSession()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminSessionCookie, token, int(adminSessionTTL.Seconds()), "/", "", requestIsSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *adminAuth) logout(c *gin.Context) {
	if token, err := c.Cookie(adminSessionCookie); err == nil {
		a.dropSession(token)
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminSessionCookie, "", -1, "/", "", requestIsSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *adminAuth) createSession() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		// crypto/rand never fails on supported platforms; refuse to hand out
		// a predictable token instead of degrading security.
		panic(fmt.Sprintf("生成会话令牌失败: %v", err))
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for existing, expiry := range a.sessions {
		if now.After(expiry) {
			delete(a.sessions, existing)
		}
	}
	a.sessions[token] = now.Add(adminSessionTTL)
	return token
}

func (a *adminAuth) validate(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	expiry, ok := a.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(a.sessions, token)
		return false
	}
	return true
}

func (a *adminAuth) dropSession(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, token)
}

func (a *adminAuth) blockRemaining(ip string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.failures[ip]
	if !ok {
		return 0
	}
	if until := time.Until(entry.blockedUntil); until > 0 {
		return until
	}
	if time.Since(entry.first) > loginFailWindow {
		delete(a.failures, ip)
	}
	return 0
}

func (a *adminAuth) recordFailure(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.failures[ip]
	if !ok || time.Since(entry.first) > loginFailWindow {
		entry = &loginFailure{first: time.Now()}
		a.failures[ip] = entry
	}
	entry.count++
	if entry.count >= loginMaxFailures {
		entry.blockedUntil = time.Now().Add(loginBlockDuration)
	}
}

func (a *adminAuth) resetFailures(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.failures, ip)
}

// requestIsSecure keeps the cookie Secure flag correct behind the TLS
// terminating reverse proxy (Caddy sets X-Forwarded-Proto).
func requestIsSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
