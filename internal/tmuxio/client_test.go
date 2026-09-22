package tmuxio

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// attachedServer starts a throwaway tmux server with a real client attached to
// a pseudo-terminal, which is what the client-dependent calls need.
//
// The client's stdin must stay open: an attach whose input ends closes
// immediately and takes the session with it, so a sleep feeds the pipe.
func attachedServer(t *testing.T, width, height int) string {
	t.Helper()
	for _, bin := range []string{"tmux", "script"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	socket := "tmux-notepad-client-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
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

	size := strconv.Itoa
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "t",
		"-x", size(width), "-y", size(height)).CombinedOutput(); err != nil {
		t.Skipf("cannot start tmux: %v: %s", err, out)
	}
	t.Cleanup(kill)

	attach := exec.Command("sh", "-c",
		"sleep 60 | script -qfec \"sh -c 'stty rows "+size(height)+" cols "+size(width)+
			"; exec tmux -L "+socket+" attach -t t'\" /dev/null")
	attach.Stdout, attach.Stderr = nil, nil
	if err := attach.Start(); err != nil {
		t.Skipf("cannot attach a client: %v", err)
	}
	t.Cleanup(func() {
		_ = attach.Process.Kill()
		_, _ = attach.Process.Wait()
	})

	env, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "t",
		"#{socket_path},#{pid},0").Output()
	if err != nil {
		t.Skipf("cannot query tmux: %v", err)
	}
	t.Setenv("TMUX", strings.TrimSpace(string(env)))

	// Wait for the client to register, otherwise every client-dependent
	// call races the attach.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("tmux", "-L", socket, "list-clients", "-F", "#{client_tty}").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return socket
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Skip("no client attached in time")
	return socket
}

func TestClientSize(t *testing.T) {
	attachedServer(t, 132, 43)

	w, h, err := ClientSize()
	if err != nil {
		t.Fatal(err)
	}
	if w != 132 || h != 43 {
		t.Fatalf("ClientSize = %dx%d, want 132x43", w, h)
	}
}

func TestClientSizeWithoutAClient(t *testing.T) {
	// A detached server has no client; the caller must get an error rather
	// than a made-up size.
	socket := "tmux-notepad-client-detached"
	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "t").CombinedOutput(); err != nil {
		t.Skipf("cannot start tmux: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
		_ = os.Remove(fmt.Sprintf("/tmp/tmux-%d/%s", os.Getuid(), socket))
	})
	env, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "t",
		"#{socket_path},#{pid},0").Output()
	if err != nil {
		t.Skip("cannot query tmux")
	}
	t.Setenv("TMUX", strings.TrimSpace(string(env)))

	if _, _, err := ClientSize(); err == nil {
		t.Fatal("want an error when no client is attached")
	}
}

func TestCurrentPane(t *testing.T) {
	socket := attachedServer(t, 100, 30)

	got, err := CurrentPane()
	if err != nil {
		t.Fatal(err)
	}
	want, err := exec.Command("tmux", "-L", socket, "list-panes", "-t", "t", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("CurrentPane = %q, want %q", got, strings.TrimSpace(string(want)))
	}
}

func TestCurrentPaneWithoutTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if _, err := CurrentPane(); err != ErrNoServer {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
}

func TestStatusLines(t *testing.T) {
	socket := attachedServer(t, 100, 30)
	set := func(value string) {
		t.Helper()
		if err := exec.Command("tmux", "-L", socket, "set-option", "-g", "status", value).Run(); err != nil {
			t.Fatalf("set status %q: %v", value, err)
		}
	}

	cases := map[string]int{"on": 1, "off": 0, "2": 2, "3": 3}
	for value, want := range cases {
		set(value)
		if got := StatusLines(); got != want {
			t.Errorf("StatusLines with status=%q = %d, want %d", value, got, want)
		}
	}
}

func TestStatusLinesWithoutTmux(t *testing.T) {
	// Falling back to one row is the safe guess: it costs a row of travel
	// rather than putting the popup under the status line.
	t.Setenv("TMUX", "")
	if got := StatusLines(); got != 1 {
		t.Fatalf("StatusLines = %d, want the fallback 1", got)
	}
}

func TestSelectPane(t *testing.T) {
	socket := attachedServer(t, 100, 30)
	if err := exec.Command("tmux", "-L", socket, "new-window", "-t", "t", "-n", "second").Run(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	out, err := exec.Command("tmux", "-L", socket, "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	panes := strings.Fields(string(out))
	if len(panes) < 2 {
		t.Skip("expected two panes")
	}
	first := panes[0]

	if err := SelectPane(first); err != nil {
		t.Fatal(err)
	}
	active, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "#{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(active)) != first {
		t.Fatalf("active pane = %q, want %q", strings.TrimSpace(string(active)), first)
	}
}

func TestSelectMissingPane(t *testing.T) {
	attachedServer(t, 100, 30)
	if err := SelectPane("%999"); err == nil {
		t.Fatal("want an error for a pane that does not exist")
	}
}

func TestClosePopupWithNoPopupOpen(t *testing.T) {
	attachedServer(t, 100, 30)
	// Closing nothing is not an error; it is how the tool makes sure no
	// popup is left over.
	if err := ClosePopup(); err != nil {
		t.Fatalf("ClosePopup on an empty client: %v", err)
	}
}

func TestReopenOpensAPopupAtTheGivenGeometry(t *testing.T) {
	socket := attachedServer(t, 100, 30)
	marker := t.TempDir() + "/reopened"

	err := Reopen(Geometry{Width: "40", Height: "10", X: "5", Y: "20"},
		"sh", "-c", "echo up > "+marker+"; sleep 2")
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return // the popup really ran the command
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := exec.Command("tmux", "-L", socket, "list-clients", "-F", "#{client_tty}").Output()
	t.Fatalf("the reopened popup never ran; clients: %q", out)
}

func TestReopenRefusesAnEmptyCommand(t *testing.T) {
	// A blank command used to be passed to tmux anyway, which failed and
	// printed a run-shell error into whatever pane the user was in.
	attachedServer(t, 100, 30)
	for _, cmd := range []string{"", "   ", "\t"} {
		if err := Reopen(Geometry{Width: "40", Height: "10"}, cmd); err == nil {
			t.Errorf("Reopen with command %q returned no error", cmd)
		}
	}
}
