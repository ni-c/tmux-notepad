// Package testenv decides whether a dependency the tests need, but that the
// machine may not have, skips a test or fails it.
package testenv

import (
	"os"
	"testing"
)

// RequireVar names the environment variable that turns a skip into a failure.
const RequireVar = "TMUX_NOTEPAD_REQUIRE_TMUX"

// Missing reports that something the test needs is not available — tmux is not
// installed, a throwaway server would not start, a client did not attach.
//
// On a developer machine that skips the test, so `go test ./...` still works
// without tmux. Where TMUX_NOTEPAD_REQUIRE_TMUX is set it fails instead: a CI
// run that silently skipped every tmux test would be green and prove nothing,
// which is worse than no CI at all.
func Missing(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv(RequireVar) != "" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}
