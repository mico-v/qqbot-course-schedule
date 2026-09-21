// Package config loads and validates the bot configuration.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Defaults for optional configuration values.
const (
	DefaultPort          = 8080
	DefaultDomain        = "https://api.bot.qq.com"
	DefaultTokenEndpoint = "https://bots.qq.com/app/getAppAccessToken"
	DefaultDatabase      = "data/course_schedule.sqlite3"
	DefaultDataDir       = "data"
	DefaultLogLevel      = "info"
	DefaultPushCron      = "30 7 * * *"
)

// AllowedWebhookPorts lists the callback ports accepted by the QQ open platform.
var AllowedWebhookPorts = []int{80, 443, 8080, 8443}

// Config mirrors config.json.
type Config struct {
	Port          int    `json:"port"`
	Bind          string `json:"bind"`
	AppID         string `json:"appid"`
	Secret        string `json:"secret"`
	Domain        string `json:"domain"`
	TokenEndpoint string `json:"token_endpoint"`
	PublicBaseURL string `json:"public_base_url"`
	Database      string `json:"database"`
	DataDir       string `json:"data_dir"`
	AdminPassword string `json:"admin_password"`
	LogLevel      string `json:"log_level"`
	PushCron      string `json:"push_cron"`
	Buttons       bool   `json:"buttons"`
}

// Load reads path, applies defaults and validates the result.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w（可参考 config.example.json）", path, err)
	}

	var cfg Config
	// Unknown keys are intentionally accepted so newer config files keep working.
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}), &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *Config) applyDefaults() {
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	if cfg.Domain == "" {
		cfg.Domain = DefaultDomain
	}
	if cfg.TokenEndpoint == "" {
		cfg.TokenEndpoint = DefaultTokenEndpoint
	}
	if cfg.Database == "" {
		cfg.Database = DefaultDatabase
	}
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDataDir
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
	if cfg.PushCron == "" {
		cfg.PushCron = DefaultPushCron
	}
}

// Validate reports the first configuration problem, if any.
func (cfg *Config) Validate() error {
	if !validPort(cfg.Port) && !cfg.isLoopbackBind() {
		return fmt.Errorf("port %d 直接对外时不被 QQ 平台接受，必须是 %v 之一；若通过反向代理访问，请设置 bind 为 127.0.0.1", cfg.Port, AllowedWebhookPorts)
	}
	if strings.TrimSpace(cfg.AppID) == "" {
		return fmt.Errorf("appid 不能为空，请填写 QQ 机器人 AppID")
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		return fmt.Errorf("secret 不能为空，请填写 QQ 机器人 AppSecret")
	}
	if _, err := url.ParseRequestURI(cfg.Domain); err != nil {
		return fmt.Errorf("domain 不是合法 URL: %w", err)
	}
	if cfg.Bind != "" && net.ParseIP(cfg.Bind) == nil {
		return fmt.Errorf("bind %q 不是合法 IP，例如 127.0.0.1（留空表示监听所有网卡）", cfg.Bind)
	}
	if _, err := url.ParseRequestURI(cfg.TokenEndpoint); err != nil {
		return fmt.Errorf("token_endpoint 不是合法 URL: %w", err)
	}
	if cfg.PublicBaseURL != "" {
		parsed, err := url.Parse(cfg.PublicBaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("public_base_url 不是合法 URL: %q", cfg.PublicBaseURL)
		}
	}
	switch strings.ToLower(cfg.LogLevel) {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("log_level %q 无效，可选 debug/info/warn/error", cfg.LogLevel)
	}
	return nil
}

// PublicImageBase returns the public base URL without a trailing slash.
func (cfg *Config) PublicImageBase() string {
	return strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
}

// ListenAddr returns the address the HTTP server should bind to.
func (cfg *Config) ListenAddr() string {
	return net.JoinHostPort(cfg.Bind, strconv.Itoa(cfg.Port))
}

func validPort(port int) bool {
	for _, allowed := range AllowedWebhookPorts {
		if port == allowed {
			return true
		}
	}
	return false
}

// isLoopbackBind reports whether the bot only listens on the local machine,
// in which case the platform-facing port restriction does not apply.
func (cfg *Config) isLoopbackBind() bool {
	if cfg.Bind == "" {
		return false
	}
	ip := net.ParseIP(cfg.Bind)
	return ip != nil && ip.IsLoopback()
}
