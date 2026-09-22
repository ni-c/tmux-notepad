// Package tmuxio talks to the tmux server.
//
// Two things here are less obvious than they look, both established by probing
// tmux 3.7:
//
//   - display-popup does NOT expand format strings such as #{pane_id} in the
//     command it runs, and $TMUX_PANE inside a popup is inherited from whatever
//     environment the client had. The originating pane therefore has to be
//     passed in explicitly by the key binding (which uses run-shell, where
//     formats do expand), with the active pane as a fallback.
//   - A popup cannot move or resize itself: when display-popup runs inside an
//     existing popup, tmux ignores -x, -y, -w and -h. Moving means closing and
//     reopening from outside, which Reopen does via run-shell -b.
package tmuxio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNoServer reports that tmux is not running or not reachable.
var ErrNoServer = errors.New("not running inside tmux")

// Available reports whether this process is running under tmux.
func Available() bool {
	return os.Getenv("TMUX") != ""
}

// run executes a tmux command and returns its trimmed standard output.
func run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// CurrentPane returns the active pane of the attached client. Inside a popup
// this is the pane the popup was opened over, which is what a key binding
// without an explicit target should act on.
func CurrentPane() (string, error) {
	if !Available() {
		return "", ErrNoServer
	}
	return run("display-message", "-p", "#{pane_id}")
}

// paneField looks up one format field of a pane.
//
// It goes through list-panes rather than "display-message -t", because the
// latter resolves against the attached client and reports that client's own
// context when there is no client — which is exactly the situation in tests
// and in detached sessions.
func paneField(pane, format string) string {
	out, err := run("list-panes", "-a", "-F", "#{pane_id}\t"+format)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		id, value, found := strings.Cut(line, "\t")
		if found && id == pane {
			return value
		}
	}
	return ""
}

// PaneExists reports whether the given pane id is still around.
func PaneExists(pane string) bool {
	out, err := run("list-panes", "-a", "-F", "#{pane_id}")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == pane {
			return true
		}
	}
	return false
}

// PaneCommand returns the command currently running in a pane, such as "zsh"
// or "claude".
func PaneCommand(pane string) string {
	return paneField(pane, "#{pane_current_command}")
}

// PanePath returns the working directory of a pane.
func PanePath(pane string) string {
	return paneField(pane, "#{pane_current_path}")
}

// bufferName is the tmux buffer the text is staged in. A fixed name keeps the
// paste stack from filling up.
const bufferName = "tmux-notepad"

// Paste puts text into the given pane as a bracketed paste.
//
// No Enter is sent: multi-line text arrives as one input that the user submits
// themselves, which is what makes this safe to use against a shell as well as
// against an agent.
func Paste(pane, text string) error {
	if !Available() {
		return ErrNoServer
	}
	if text == "" {
		return errors.New("nothing to paste")
	}
	load := exec.Command("tmux", "load-buffer", "-b", bufferName, "-")
	load.Stdin = strings.NewReader(text)
	var errb bytes.Buffer
	load.Stderr = &errb
	if err := load.Run(); err != nil {
		return fmt.Errorf("load-buffer: %s", strings.TrimSpace(errb.String()))
	}
	// -p wraps the text in bracketed paste markers, -d drops the buffer
	// afterwards.
	if _, err := run("paste-buffer", "-p", "-d", "-b", bufferName, "-t", pane); err != nil {
		return err
	}
	return nil
}

// SelectPane moves the focus to a pane, so the user can press Enter right
// after the popup closes.
func SelectPane(pane string) error {
	if _, err := run("select-window", "-t", pane); err != nil {
		// A pane in the current window needs no window switch.
		_ = err
	}
	_, err := run("select-pane", "-t", pane)
	return err
}

// Geometry describes where a popup sits. Width and Height may be given in
// cells or as a percentage such as "80%"; X and Y additionally accept tmux
// position keywords such as "C" for centred.
type Geometry struct {
	Width  string
	Height string
	X      string
	Y      string
}

// StatusLines returns how many rows the status line occupies, so callers can
// keep a popup clear of it. The "status" option is off, on, or a count.
func StatusLines() int {
	out, err := run("show-options", "-gv", "status")
	if err != nil {
		return 1
	}
	switch strings.TrimSpace(out) {
	case "off", "0":
		return 0
	case "on", "":
		return 1
	}
	if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil && n >= 0 {
		return n
	}
	return 1
}

// ClientSize returns the size of the attached client in cells.
func ClientSize() (width, height int, err error) {
	out, err := run("display-message", "-p", "#{client_width} #{client_height}")
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Fields(out)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected client size %q", out)
	}
	width, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	height, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

// Reopen closes the current popup and opens a new one at the given geometry
// running command with args.
//
// It has to be run detached (run-shell -b) because the calling process is
// inside the popup that is about to be replaced; tmux would otherwise ignore
// the geometry flags.
func Reopen(g Geometry, command string, args ...string) error {
	if !Available() {
		return ErrNoServer
	}
	// An empty command would still be handed to tmux, which runs it, fails,
	// and puts a run-shell error on the user's screen — in whatever pane
	// they happen to be working in.
	if strings.TrimSpace(command) == "" {
		return errors.New("no command to reopen with")
	}
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, shellQuote(command))
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	inner := strings.Join(quoted, " ")

	// tmux allows only one popup per client, and the current one is still
	// on screen while this process shuts down — opening the replacement
	// straight away silently does nothing. Waiting briefly lets the caller
	// exit and its popup disappear on its own. Closing it forcibly with
	// "display-popup -C" would work too, but it pulls the terminal out from
	// under the old process, which then lingers.
	popup := fmt.Sprintf("sleep %s; tmux display-popup -E -w %s -h %s -x %s -y %s %s",
		reopenDelay,
		shellQuote(g.Width), shellQuote(g.Height),
		shellQuote(g.X), shellQuote(g.Y), shellQuote(inner))

	_, err := run("run-shell", "-b", popup)
	return err
}

// reopenDelay is how long to wait between closing and reopening, as a value
// for sleep(1). Long enough for tmux to drop the old popup, short enough not
// to be seen.
const reopenDelay = "0.25"

// ClosePopup closes any popup on the client.
func ClosePopup() error {
	_, err := run("display-popup", "-C")
	return err
}

// shellQuote wraps s so that a POSIX shell sees it as one word.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`*?[]{}()<>|&;#~=%!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
