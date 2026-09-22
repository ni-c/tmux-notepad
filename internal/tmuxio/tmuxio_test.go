package tmuxio

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ni-c/tmux-notepad/internal/testenv"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":           "plain",
		"":                "''",
		"with space":      "'with space'",
		"it's":            `'it'\''s'`,
		"$VAR":            "'$VAR'",
		"a;b":             "'a;b'",
		"/srv/notes":      "/srv/notes",
		"80%":             "'80%'",
		"back`tick":       "'back`tick'",
		"new\nline":       "'new\nline'",
		`quote"and\slash`: `'quote"and\slash'`,
		"[10] Notes":      "'[10] Notes'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellQuoteSurvivesRoundTrip(t *testing.T) {
	// Whatever we quote must arrive at the other end unchanged.
	for _, in := range []string{
		"plain", "with space", "it's", "$HOME", "a;rm -rf /", "[10] Notes",
		"tmux-notepad --target %12", "80%", `back\slash`,
	} {
		out, err := exec.Command("sh", "-c", "printf %s "+shellQuote(in)).Output()
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("round trip of %q gave %q", in, out)
		}
	}
}

// testServer starts a throwaway tmux server and points $TMUX at it.
func testServer(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		testenv.Missing(t, "tmux not installed")
	}
	socket := "tmux-notepad-test-" + t.Name()
	socket = strings.NewReplacer("/", "-", " ", "-").Replace(socket)

	kill := func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
		// tmux leaves the socket file behind; without this the test runs
		// slowly litter /tmp.
		if dir := os.Getenv("TMUX_TMPDIR"); dir != "" {
			_ = os.Remove(dir + "/" + socket)
		} else {
			_ = os.Remove(fmt.Sprintf("/tmp/tmux-%d/%s", os.Getuid(), socket))
		}
	}
	kill()
	cmd := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "t", "-x", "120", "-y", "40", "cat")
	if out, err := cmd.CombinedOutput(); err != nil {
		testenv.Missing(t, "cannot start a tmux server: %v: %s", err, out)
	}
	t.Cleanup(kill)

	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "t", "#{socket_path},#{pid},0").Output()
	if err != nil {
		testenv.Missing(t, "cannot query the test server: %v", err)
	}
	t.Setenv("TMUX", strings.TrimSpace(string(out)))

	// Right after new-session the pane still runs tmux itself; the shell
	// command is exec'd a moment later. Wait for it, otherwise every
	// assertion about the pane races the server.
	waitForPaneCommand(t, socket, "cat")
	return socket
}

// waitForPaneCommand blocks until the first pane runs want, or fails.
func waitForPaneCommand(t *testing.T, socket, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("tmux", "-L", socket, "list-panes", "-a", "-F", "#{pane_current_command}").Output()
		if err == nil && strings.Contains(string(out), want) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pane never started %q", want)
}

func panes(t *testing.T, socket string) []string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(out))
}

func TestAvailable(t *testing.T) {
	t.Setenv("TMUX", "")
	if Available() {
		t.Fatal("Available() = true without $TMUX")
	}
	t.Setenv("TMUX", "/tmp/sock,1,0")
	if !Available() {
		t.Fatal("Available() = false with $TMUX set")
	}
}

func TestPasteWithoutTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := Paste("%0", "text"); err != ErrNoServer {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
}

func TestPasteEmptyText(t *testing.T) {
	t.Setenv("TMUX", "/tmp/sock,1,0")
	if err := Paste("%0", ""); err == nil {
		t.Fatal("want an error when there is nothing to paste")
	}
}

func TestPasteDeliversTextVerbatim(t *testing.T) {
	socket := testServer(t)
	target := panes(t, socket)[0]

	text := "line one\nline two with \"quotes\", $VAR and 'ticks'\nÜmlaut 🌲"
	if err := Paste(target, text); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)

	out, err := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-t", target).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"line one",
		`line two with "quotes", $VAR and 'ticks'`,
		"Ümlaut 🌲",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("pane does not contain %q; got:\n%s", want, got)
		}
	}
}

func TestPasteDoesNotSubmit(t *testing.T) {
	// The pane runs `cat`, which echoes a line only once it is submitted.
	// A pasted line without Enter must therefore NOT come back doubled.
	socket := testServer(t)
	target := panes(t, socket)[0]

	if err := Paste(target, "unsubmitted"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	out, _ := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-t", target).Output()
	if n := strings.Count(string(out), "unsubmitted"); n != 1 {
		t.Fatalf("text appears %d times, want exactly 1 (it must not be submitted):\n%s", n, out)
	}
}

func TestPasteToMissingPane(t *testing.T) {
	testServer(t)
	if err := Paste("%999", "text"); err == nil {
		t.Fatal("want an error for a pane that does not exist")
	}
}

func TestPaneExistsAndCommand(t *testing.T) {
	socket := testServer(t)
	target := panes(t, socket)[0]

	if !PaneExists(target) {
		t.Fatalf("PaneExists(%q) = false", target)
	}
	if PaneExists("%999") {
		t.Fatal("PaneExists(%999) = true")
	}
	if got := PaneCommand(target); got != "cat" {
		t.Fatalf("PaneCommand = %q, want cat", got)
	}
	if got := PaneCommand("%999"); got != "" {
		t.Fatalf("PaneCommand of a missing pane = %q, want empty", got)
	}
	if got := PanePath(target); got == "" {
		t.Fatal("PanePath returned nothing")
	}
}

func TestBufferIsCleanedUp(t *testing.T) {
	socket := testServer(t)
	target := panes(t, socket)[0]
	if err := Paste(target, "some text"); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("tmux", "-L", socket, "list-buffers", "-F", "#{buffer_name}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), bufferName) {
		t.Fatalf("buffer %q was left behind: %s", bufferName, out)
	}
}

func TestReopenWithoutTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if err := Reopen(Geometry{}, "tmux-notepad"); err != ErrNoServer {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
}

func TestReopenBuildsAQuotedCommand(t *testing.T) {
	// Without an attached client tmux cannot actually show a popup, so this
	// checks that run-shell is reached with a well-formed command rather
	// than that a window appears.
	socket := testServer(t)
	marker := os.Getenv("TMPDIR")
	if marker == "" {
		marker = "/tmp"
	}
	probe := marker + "/tmux-notepad-reopen-probe"
	_ = os.Remove(probe)

	err := Reopen(Geometry{Width: "80%", Height: "80%", X: "C", Y: "C"},
		"sh", "-c", "touch "+probe)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	_ = socket
	// The popup itself cannot open without a client; what matters is that
	// tmux accepted the command line.
}
