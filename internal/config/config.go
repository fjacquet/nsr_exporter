// Package config loads and validates the exporter's YAML configuration. config.yaml
// is the source of truth; ${ENV_VAR} references in host/username/password expand at
// load time (fail-fast on unset), and a passwordFile may supply secrets out-of-band.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// Config is the top-level exporter configuration.
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Collection    CollectionConfig    `yaml:"collection"`
	Systems       []SystemConfig      `yaml:"systems"`
	OpenTelemetry OpenTelemetryConfig `yaml:"opentelemetry"`
}

// OpenTelemetryConfig controls the optional OTLP push export path.
// OTEL_EXPORTER_OTLP_ENDPOINT env var takes precedence over Endpoint when set
// (standard OTEL SDK convention).
type OpenTelemetryConfig struct {
	// Endpoint is the OTLP/gRPC collector address (e.g. "otel-collector:4317").
	// An empty string disables OTLP push.
	Endpoint string `yaml:"endpoint"`
	// PushInterval is how often metrics are pushed to the collector. Default: 30s.
	PushInterval time.Duration `yaml:"pushInterval"`
	// Insecure allows plaintext gRPC (no TLS) for in-cluster collectors.
	Insecure bool `yaml:"insecure"`
	// Headers are optional gRPC metadata headers sent with every push.
	Headers map[string]string `yaml:"headers"`
}

// ServerConfig controls the exporter's own HTTP listener (not the NetWorker port).
type ServerConfig struct {
	Host    string `yaml:"host"`
	Port    string `yaml:"port"`
	URI     string `yaml:"uri"`
	LogName string `yaml:"logName"`
}

// CollectionConfig controls the background poll loop.
type CollectionConfig struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	// BackupWindow bounds the /backups sizing query (ADR-0010): only save sets with
	// saveTime within this lookback are fetched, so the full catalog is never pulled.
	BackupWindow time.Duration `yaml:"backupWindow"`
}

// SystemConfig is one NetWorker server target. A single exporter process serves
// many systems; every emitted metric carries system="<Name>".
type SystemConfig struct {
	Name               string `yaml:"name"`
	Host               string `yaml:"host"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	PasswordFile       string `yaml:"passwordFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads, expands, and validates the config at path.
func Load(path string) (*Config, error) {
	raw, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	expanded, err := expandEnv(string(raw))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.UnmarshalStrict([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// expandEnv replaces every ${VAR} with its environment value, failing fast on the
// first unset reference rather than silently producing an empty credential.
func expandEnv(s string) (string, error) {
	var missing []string
	out := envRef.ReplaceAllStringFunc(s, func(ref string) string {
		name := ref[2 : len(ref)-1]
		val, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return ref
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unset environment variables referenced in config: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func (c *Config) applyDefaults() error {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == "" {
		c.Server.Port = "9097"
	}
	if c.Server.URI == "" {
		c.Server.URI = "/metrics"
	}
	if c.Collection.Interval == 0 {
		c.Collection.Interval = 5 * time.Minute
	}
	if c.Collection.Timeout == 0 {
		c.Collection.Timeout = 60 * time.Second
	}
	if c.Collection.BackupWindow == 0 {
		c.Collection.BackupWindow = 24 * time.Hour
	}
	if c.OpenTelemetry.PushInterval == 0 {
		c.OpenTelemetry.PushInterval = 30 * time.Second
	}
	// Resolve passwordFile into Password where set.
	for i := range c.Systems {
		if c.Systems[i].PasswordFile != "" {
			b, err := readFile(c.Systems[i].PasswordFile)
			if err != nil {
				return fmt.Errorf("read passwordFile for system %q: %w", c.Systems[i].Name, err)
			}
			c.Systems[i].Password = strings.TrimSpace(string(b))
		}
	}
	return nil
}

func (c *Config) validate() error {
	if len(c.Systems) == 0 {
		return fmt.Errorf("config defines no systems")
	}
	seen := make(map[string]bool, len(c.Systems))
	for _, s := range c.Systems {
		switch {
		case s.Name == "":
			return fmt.Errorf("system with host %q has no name", s.Host)
		case seen[s.Name]:
			return fmt.Errorf("duplicate system name %q", s.Name)
		case s.Host == "":
			return fmt.Errorf("system %q has no host", s.Name)
		case s.Username == "":
			return fmt.Errorf("system %q has no username", s.Name)
		case s.Password == "":
			return fmt.Errorf("system %q has no password (set password or passwordFile)", s.Name)
		}
		seen[s.Name] = true
	}
	return nil
}
