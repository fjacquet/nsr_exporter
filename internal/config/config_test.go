package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const validYAML = `
server:
  port: "9097"
collection:
  interval: 2m
  timeout: 30s
systems:
  - name: nsr-01
    host: "https://nw.local:9090"
    username: "${TEST_NSR_USER}"
    password: "${TEST_NSR_PASS}"
    insecureSkipVerify: true
`

func TestLoad_ExpandsEnvAndDefaults(t *testing.T) {
	t.Setenv("TEST_NSR_USER", "admin")
	t.Setenv("TEST_NSR_PASS", "s3cret")

	cfg, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Username != "admin" || cfg.Systems[0].Password != "s3cret" {
		t.Fatalf("env not expanded: %+v", cfg.Systems[0])
	}
	if cfg.Server.URI != "/metrics" {
		t.Fatalf("default URI not applied: %q", cfg.Server.URI)
	}
	if cfg.Collection.Interval != 2*time.Minute {
		t.Fatalf("interval = %v, want 2m", cfg.Collection.Interval)
	}
}

func TestLoad_FailFastOnUnsetEnv(t *testing.T) {
	_ = os.Unsetenv("TEST_NSR_USER")
	_ = os.Unsetenv("TEST_NSR_PASS")
	if _, err := Load(writeTemp(t, validYAML)); err == nil {
		t.Fatal("expected error for unset ${TEST_NSR_USER}/${TEST_NSR_PASS}, got nil")
	}
}

func TestLoad_RejectsDuplicateSystemNames(t *testing.T) {
	body := `
systems:
  - { name: dup, host: h1, username: u, password: p }
  - { name: dup, host: h2, username: u, password: p }
`
	if _, err := Load(writeTemp(t, body)); err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

func TestLoad_PasswordFile(t *testing.T) {
	dir := t.TempDir()
	pwFile := filepath.Join(dir, "secret")
	if err := os.WriteFile(pwFile, []byte("  filepass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `
systems:
  - name: nsr-01
    host: h
    username: u
    passwordFile: "` + pwFile + `"
`
	cfg, err := Load(writeTemp(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Password != "filepass" {
		t.Fatalf("passwordFile not loaded/trimmed: %q", cfg.Systems[0].Password)
	}
}
