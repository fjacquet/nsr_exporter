package config

import (
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadDotEnv loads a .env file (CWD first, then the config file's directory) into
// the process environment BEFORE config interpolation runs. godotenv never
// overrides already-set variables, so real injected secrets always win over .env
// defaults. ".env is nice, config.yaml is the way" — this is a quickstart
// convenience, never a replacement for config.yaml. Missing files are ignored.
func LoadDotEnv(cfgPath string) {
	// CWD .env first (does not override real env).
	_ = godotenv.Load()
	// Then a .env sitting next to the config file.
	if cfgPath != "" {
		_ = godotenv.Load(filepath.Join(filepath.Dir(cfgPath), ".env"))
	}
}
