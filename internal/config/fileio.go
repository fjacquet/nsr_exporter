package config

import (
	"os"
	"path/filepath"
)

// readFile centralizes every read of an operator-supplied path (the --config file
// and any passwordFile). Reading a variable path is the intended behavior here —
// the operator chooses what to load — so the single audited choke point lives in
// this one function rather than being scattered (and rather than per-call inline
// linter suppressions, which the family standard forbids). The .golangci.yml
// path-scopes the gosec G304 exclusion to this file.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Clean(path))
}
