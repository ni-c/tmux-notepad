package tmuxio

import (
	"os"
	"testing"
)

// TestMain keeps the test binary away from the developer's own tmux server;
// every test that needs one starts a throwaway server and points $TMUX at it.
func TestMain(m *testing.M) {
	os.Unsetenv("TMUX")
	os.Exit(m.Run())
}
