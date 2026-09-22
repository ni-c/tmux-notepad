package ui

import (
	"os"
	"testing"
)

// TestMain makes sure the test binary can never reach the developer's own
// tmux server.
//
// Without this, any code path that calls tmux — a reopen triggered by a window
// key, or targetHint() during a render — talks to whatever $TMUX points at,
// which is the session the tests are being run from. That puts error messages
// on someone's screen and could open popups in their face. With $TMUX unset,
// those calls return ErrNoServer instead; tests that genuinely need a server
// set up a throwaway one and point $TMUX at it themselves.
func TestMain(m *testing.M) {
	os.Unsetenv("TMUX")
	os.Exit(m.Run())
}
