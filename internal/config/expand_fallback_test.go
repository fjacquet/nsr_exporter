package config

import (
	"os"
	"testing"
)

// TestExpandFallback pins the ${VAR:-default} form ported from pscale_exporter: it falls
// back when the variable is unset OR exported empty (shell / docker-compose semantics),
// prefers a real value, and never errors — while a bare ${VAR} must keep failing loudly,
// which is what stops an UNSET variable from silently resolving to an empty string.
func TestExpandFallback(t *testing.T) {
	unsetForTest(t, "NSR_FALLBACK_TEST_UNSET")
	t.Setenv("NSR_FALLBACK_TEST_SET", "real")
	t.Setenv("NSR_FALLBACK_TEST_EMPTY", "")
	for _, tc := range []struct{ name, in, want string }{
		{"unset falls back", "${NSR_FALLBACK_TEST_UNSET:-false}", "false"},
		{"set wins over default", "${NSR_FALLBACK_TEST_SET:-false}", "real"},
		{"exported empty falls back", "${NSR_FALLBACK_TEST_EMPTY:-other}", "other"},
		{"empty default allowed", "${NSR_FALLBACK_TEST_UNSET:-}", ""},
		{"mixed with literal text", "a${NSR_FALLBACK_TEST_UNSET:-b}c", "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandEnv(tc.in)
			if err != nil {
				t.Fatalf("expandEnv(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("expandEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	if _, err := expandEnv("${NSR_FALLBACK_TEST_UNSET}"); err == nil {
		t.Error("a bare reference to an unset variable must still fail")
	}
}

// unsetForTest clears name for the duration of the test and restores whatever was there —
// value and set/unset state alike. Tests that assert on an *unset* variable are otherwise
// at the mercy of whatever the developer or CI runner happens to export.
func unsetForTest(t *testing.T, name string) {
	t.Helper()
	old, had := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, old)
			return
		}
		_ = os.Unsetenv(name)
	})
}
