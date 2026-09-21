package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeTemp(t, `{"appid":"10000","secret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.Domain != DefaultDomain {
		t.Errorf("Domain = %q, want %q", cfg.Domain, DefaultDomain)
	}
	if cfg.TokenEndpoint != DefaultTokenEndpoint {
		t.Errorf("TokenEndpoint = %q, want %q", cfg.TokenEndpoint, DefaultTokenEndpoint)
	}
	if cfg.Database != DefaultDatabase {
		t.Errorf("Database = %q, want %q", cfg.Database, DefaultDatabase)
	}
	if cfg.DataDir != DefaultDataDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, DefaultDataDir)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	_, err := Load(writeTemp(t, `{"port":9999,"appid":"1","secret":"s"}`))
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("err = %v, want port error", err)
	}
}

func TestLoadRejectsMissingCredentials(t *testing.T) {
	if _, err := Load(writeTemp(t, `{"appid":"","secret":"s"}`)); err == nil {
		t.Fatal("empty appid should fail")
	}
	if _, err := Load(writeTemp(t, `{"appid":"1","secret":"  "}`)); err == nil {
		t.Fatal("blank secret should fail")
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	if _, err := Load(writeTemp(t, `{"appid":"1","secret":"s","log_level":"loud"}`)); err == nil {
		t.Fatal("unknown log level should fail")
	}
}

func TestLoadIgnoresUnknownKeys(t *testing.T) {
	cfg, err := Load(writeTemp(t, `{"appid":"1","secret":"s","future_option":true}`))
	if err != nil || cfg.AppID != "1" {
		t.Fatalf("Load = %+v, %v", cfg, err)
	}
}

func TestLoadRejectsInvalidPublicBaseURL(t *testing.T) {
	if _, err := Load(writeTemp(t, `{"appid":"1","secret":"s","public_base_url":"bot.example.com"}`)); err == nil {
		t.Fatal("scheme-less public_base_url should fail")
	}
	cfg, err := Load(writeTemp(t, `{"appid":"1","secret":"s","public_base_url":"https://bot.example.com/"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.PublicImageBase(); got != "https://bot.example.com" {
		t.Errorf("PublicImageBase() = %q", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil || !strings.Contains(err.Error(), "读取配置文件") {
		t.Fatalf("err = %v, want read error", err)
	}
}

func TestBindAndListenAddr(t *testing.T) {
	cfg, err := Load(writeTemp(t, `{"appid":"1","secret":"s","port":8443}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ListenAddr(); got != ":8443" {
		t.Errorf("ListenAddr() = %q, want :8443", got)
	}

	cfg, err = Load(writeTemp(t, `{"appid":"1","secret":"s","port":8443,"bind":"127.0.0.1"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ListenAddr(); got != "127.0.0.1:8443" {
		t.Errorf("ListenAddr() = %q, want 127.0.0.1:8443", got)
	}

	if _, err := Load(writeTemp(t, `{"appid":"1","secret":"s","bind":"not-an-ip"}`)); err == nil {
		t.Fatal("invalid bind should fail")
	}
}

func TestLoopbackBindAllowsAnyPort(t *testing.T) {
	cfg, err := Load(writeTemp(t, `{"appid":"1","secret":"s","port":18080,"bind":"127.0.0.1"}`))
	if err != nil {
		t.Fatalf("loopback bind with custom port should pass: %v", err)
	}
	if got := cfg.ListenAddr(); got != "127.0.0.1:18080" {
		t.Errorf("ListenAddr() = %q", got)
	}
	if _, err := Load(writeTemp(t, `{"appid":"1","secret":"s","port":18080,"bind":"0.0.0.0"}`)); err == nil {
		t.Fatal("public bind with non-platform port should fail")
	}
}
