// Package testenv decides whether a dependency the tests need, but that the
// machine may not have, skips a test or fails it.
package testenv

import (
	"os"
	"testing"
)

// Environment variables that turn a skip into a failure. They are separate
// because the two things they guard are available in different places.
const (
	// RequireTmux: tmux is installed and a throwaway server can be started.
	// True on any developer machine and on a CI runner that installs tmux, so
	// CI sets this and a silent skip there counts as a failure.
	RequireTmux = "TMUX_NOTEPAD_REQUIRE_TMUX"

	// RequireClient: a real tmux *client* can be attached to that server.
	// That goes through script(1) and a pseudo-terminal, and it does not work
	// on a hosted CI runner — every attach there times out. CI deliberately
	// leaves this unset; set it locally when working on internal/tmuxio.
	RequireClient = "TMUX_NOTEPAD_REQUIRE_CLIENT"
)

// Missing reports that something the test needs is not available — tmux is not
// installed, or a throwaway server would not start.
//
// On a developer machine without tmux that skips the test, so `go test ./...`
// still works. Under RequireTmux it fails instead: a CI run that silently
// skipped every tmux test would be green and prove nothing, which is worse
// than no CI at all.
func Missing(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv(RequireTmux) != "" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

// NeedsClient reports that no real tmux client could be attached.
//
// This skips by default, including under RequireTmux, because a hosted runner
// cannot attach one at all: the tests that need a client are the ones about
// client size, the current pane and reopening the popup, and on GitHub's
// runners every one of them times out waiting for the attach. Failing there
// would say nothing about the change under test.
//
// Set RequireClient on a machine that does have a terminal — that turns these
// into real failures, which is what you want while changing internal/tmuxio.
func NeedsClient(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv(RequireClient) != "" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}
