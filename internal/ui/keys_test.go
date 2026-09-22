package ui

import (
	"fmt"
	"github.com/ni-c/tmux-notepad/internal/testenv"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ni-c/tmux-notepad/internal/config"
	"github.com/ni-c/tmux-notepad/internal/notes"
)

// key builds a KeyMsg for a single character.
func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// press sends a key to the model and returns the updated one.
func press(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	updated, _ := m.handleKey(msg)
	return updated.(Model)
}

// noteBody reads the note file back.
func noteBody(t *testing.T, m Model) string {
	t.Helper()
	raw, err := os.ReadFile(m.doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// tmuxTarget starts a throwaway tmux server with a pane running cat, points
// $TMUX at it, and returns that pane's id. Tests that paste need a real
// server, because the paste goes through tmux itself.
func tmuxTarget(t *testing.T) (socket, pane string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		testenv.Missing(t, "tmux not installed")
	}
	socket = "tmux-notepad-ui-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
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
	if out, err := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", "t",
		"-x", "80", "-y", "24", "cat").CombinedOutput(); err != nil {
		testenv.Missing(t, "cannot start tmux: %v: %s", err, out)
	}
	t.Cleanup(kill)

	env, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "t",
		"#{socket_path},#{pid},0").Output()
	if err != nil {
		testenv.Missing(t, "cannot query tmux: %v", err)
	}
	t.Setenv("TMUX", strings.TrimSpace(string(env)))

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("tmux", "-L", socket, "list-panes", "-a",
			"-F", "#{pane_id} #{pane_current_command}").Output()
		if err == nil && strings.Contains(string(out), "cat") {
			return socket, strings.Fields(string(out))[0]
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("pane never started cat")
	return "", ""
}

func paneText(t *testing.T, socket, pane string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-t", pane).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// --- send ----------------------------------------------------------------

func TestSendPastesAndTicksOff(t *testing.T) {
	socket, pane := tmuxTarget(t)
	m := newTestModel(t, "# One\nthe body of one\n\n# Two\nbody two\n")
	m.target = pane

	updated, _ := m.send(false)
	m = updated.(Model)
	time.Sleep(400 * time.Millisecond)

	if got := paneText(t, socket, pane); !strings.Contains(got, "the body of one") {
		t.Fatalf("pane does not contain the entry body:\n%s", got)
	}
	if !strings.Contains(noteBody(t, m), "# ✓ One") {
		t.Fatalf("entry was not ticked off:\n%s", noteBody(t, m))
	}
	// Staying open advances to the next entry.
	if it, _, _ := m.current(); it.Title != "Two" {
		t.Fatalf("selection is %q, want it to have advanced to Two", it.Title)
	}
	if !strings.Contains(m.status, "One") {
		t.Fatalf("status = %q, want it to name what was pasted", m.status)
	}
}

func TestSendPastesTheTitleWhenThereIsNoBody(t *testing.T) {
	socket, pane := tmuxTarget(t)
	m := newTestModel(t, "# make test\n")
	m.target = pane

	if _, _ = m.send(false); true {
		time.Sleep(400 * time.Millisecond)
	}
	if got := paneText(t, socket, pane); !strings.Contains(got, "make test") {
		t.Fatalf("pane does not contain the title:\n%s", got)
	}
}

func TestSendWithIncludeTitle(t *testing.T) {
	socket, pane := tmuxTarget(t)
	m := newTestModel(t, "# One\nthe body\n")
	m.target = pane
	m.cfg.Behavior.IncludeTitle = true

	_, _ = m.send(false)
	time.Sleep(400 * time.Millisecond)

	got := paneText(t, socket, pane)
	if !strings.Contains(got, "One") || !strings.Contains(got, "the body") {
		t.Fatalf("pane should hold title and body:\n%s", got)
	}
}

func TestSendWithoutMarkDoneLeavesTheFileAlone(t *testing.T) {
	_, pane := tmuxTarget(t)
	m := newTestModel(t, "# One\nbody\n")
	m.target = pane
	m.cfg.Behavior.MarkDone = config.MarkDoneNone

	before := noteBody(t, m)
	_, _ = m.send(false)
	time.Sleep(300 * time.Millisecond)

	if after := noteBody(t, m); after != before {
		t.Fatalf("note was modified although mark_done is %q:\n%s", config.MarkDoneNone, after)
	}
}

func TestSendDoesNotTickOffAnAlreadyDoneEntry(t *testing.T) {
	_, pane := tmuxTarget(t)
	m := newTestModel(t, "# ✓ One\nbody\n")
	m.target = pane

	before := noteBody(t, m)
	_, _ = m.send(false)
	time.Sleep(300 * time.Millisecond)

	if after := noteBody(t, m); after != before {
		t.Fatalf("file changed for an entry that was already done:\n%s", after)
	}
}

func TestSendWithoutATarget(t *testing.T) {
	m := newTestModel(t, "# One\nbody\n")
	m.target = ""
	updated, cmd := m.send(false)
	if cmd != nil {
		t.Fatal("nothing should happen without a target")
	}
	if status := updated.(Model).status; !strings.Contains(status, "target") {
		t.Fatalf("status = %q, want it to mention the missing target", status)
	}
	if strings.Contains(noteBody(t, updated.(Model)), "✓") {
		t.Fatal("nothing was pasted, so nothing may be ticked off")
	}
}

func TestSendToAVanishedPane(t *testing.T) {
	tmuxTarget(t)
	m := newTestModel(t, "# One\nbody\n")
	m.target = "%999"

	updated, _ := m.send(false)
	if status := updated.(Model).status; !strings.Contains(status, "gone") {
		t.Fatalf("status = %q, want it to say the pane is gone", status)
	}
	if strings.Contains(noteBody(t, updated.(Model)), "✓") {
		t.Fatal("a failed paste must not tick the entry off")
	}
}

func TestSendWithNothingSelected(t *testing.T) {
	m := newTestModel(t, "")
	m.target = "%0"
	updated, _ := m.send(false)
	if status := updated.(Model).status; !strings.Contains(status, "nothing selected") {
		t.Fatalf("status = %q", status)
	}
}

func TestSendKeepsTheEntryWhenTheNoteChangedUnderneath(t *testing.T) {
	_, pane := tmuxTarget(t)
	m := newTestModel(t, "# One\nbody\n")
	m.target = pane

	// Someone rewrites the note between load and paste.
	other := "# One\nbody edited elsewhere\n"
	if err := os.WriteFile(m.doc.Path, []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(m.doc.Path, future, future); err != nil {
		t.Fatal(err)
	}

	updated, _ := m.send(false)
	m = updated.(Model)
	time.Sleep(300 * time.Millisecond)

	if got := noteBody(t, m); !strings.Contains(got, "edited elsewhere") {
		t.Fatalf("the other edit was clobbered:\n%s", got)
	}
	if !strings.Contains(m.status, "changed elsewhere") {
		t.Fatalf("status = %q, want it to report the conflict", m.status)
	}
}

// --- toggleDone ----------------------------------------------------------

func TestToggleDoneBothWays(t *testing.T) {
	m := newTestModel(t, "# One\nbody\n")

	updated, _ := m.toggleDone()
	m = updated.(Model)
	if !strings.Contains(noteBody(t, m), "# ✓ One") {
		t.Fatalf("not ticked off:\n%s", noteBody(t, m))
	}

	updated, _ = m.toggleDone()
	m = updated.(Model)
	if strings.Contains(noteBody(t, m), "✓") {
		t.Fatalf("not un-ticked:\n%s", noteBody(t, m))
	}
}

func TestToggleDoneWithNothingSelected(t *testing.T) {
	m := newTestModel(t, "")
	if _, cmd := m.toggleDone(); cmd != nil {
		t.Fatal("an empty note has nothing to toggle")
	}
}

// --- adding, deleting, creating -------------------------------------------

func TestAppendEntryThroughThePrompt(t *testing.T) {
	m := newTestModel(t, "# One\nbody\n")

	m = press(t, m, key('a'))
	if m.mode != modePrompt {
		t.Fatalf("mode = %v, want the prompt", m.mode)
	}
	m.input.SetValue("Fresh entry")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeEntries {
		t.Fatalf("mode = %v, want to be back in the list", m.mode)
	}
	if !strings.Contains(noteBody(t, m), "# Fresh entry") {
		t.Fatalf("entry not written:\n%s", noteBody(t, m))
	}
	if it, _, _ := m.current(); it.Title != "Fresh entry" {
		t.Fatalf("selection = %q, want the new entry", it.Title)
	}
}

func TestAppendEntryWithAnEmptyTitleDoesNothing(t *testing.T) {
	m := newTestModel(t, "# One\nbody\n")
	before := noteBody(t, m)

	m = press(t, m, key('a'))
	m.input.SetValue("   ")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if after := noteBody(t, m); after != before {
		t.Fatalf("file changed for an empty title:\n%s", after)
	}
}

func TestPromptCanBeCancelled(t *testing.T) {
	m := newTestModel(t, "# One\nbody\n")
	before := noteBody(t, m)

	m = press(t, m, key('a'))
	m.input.SetValue("Never written")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != modeEntries {
		t.Fatalf("mode = %v, want the list", m.mode)
	}
	if after := noteBody(t, m); after != before {
		t.Fatalf("cancelled prompt still wrote:\n%s", after)
	}
}

func TestDeleteNeedsConfirmation(t *testing.T) {
	m := newTestModel(t, "# One\nbody one\n\n# Two\nbody two\n")
	before := noteBody(t, m)

	m = press(t, m, key('d'))
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want the confirmation", m.mode)
	}
	// Anything other than y backs out.
	m = press(t, m, key('n'))
	if m.mode != modeEntries {
		t.Fatalf("mode = %v, want the list", m.mode)
	}
	if after := noteBody(t, m); after != before {
		t.Fatalf("declined deletion still wrote:\n%s", after)
	}

	m = press(t, m, key('d'))
	m = press(t, m, key('y'))
	got := noteBody(t, m)
	if strings.Contains(got, "# One") {
		t.Fatalf("entry was not deleted:\n%s", got)
	}
	if !strings.Contains(got, "# Two") {
		t.Fatalf("the wrong entry went missing:\n%s", got)
	}
}

func TestCreateNoteFromThePicker(t *testing.T) {
	m := newTestModel(t, "# One\n")
	dir := t.TempDir()
	m.sources = []notes.Source{{Name: "Notes", Path: dir, Available: true}}
	m.cfg.Sources = []config.Source{{Name: "Notes", Path: dir, Recursive: true}}

	m = press(t, m, key('n'))
	if m.mode != modePrompt {
		t.Fatalf("mode = %v, want the prompt", m.mode)
	}
	m.input.SetValue("Brand New")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.doc == nil || !strings.HasSuffix(m.doc.Path, "Brand New.md") {
		t.Fatalf("open note = %v, want the new file", m.doc)
	}
	if _, err := os.Stat(dir + "/Brand New.md"); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if len(m.doc.Items) != 0 {
		t.Fatalf("a new note should be empty, got %d entries", len(m.doc.Items))
	}
}

func TestCreateNoteWithNoReachableSource(t *testing.T) {
	m := newTestModel(t, "# One\n")
	m.sources = []notes.Source{{Name: "Gone", Path: "/nope", Available: false}}

	m = press(t, m, key('n'))
	m.input.SetValue("Nowhere")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m.status, "no reachable") {
		t.Fatalf("status = %q, want it to explain why nothing was created", m.status)
	}
}

// --- navigation and modes -------------------------------------------------

func TestListNavigationStopsAtBothEnds(t *testing.T) {
	m := newTestModel(t, "# A\n# B\n# C\n")
	m.cursor = 0

	m = press(t, m, key('k')) // up at the top
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want it to stay at 0", m.cursor)
	}
	m = press(t, m, key('j'))
	m = press(t, m, key('j'))
	m = press(t, m, key('j')) // down past the end
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want it to stop at the last entry", m.cursor)
	}
	m = press(t, m, key('g'))
	if m.cursor != 0 {
		t.Fatalf("g should jump to the top, cursor = %d", m.cursor)
	}
	m = press(t, m, key('G'))
	if m.cursor != 2 {
		t.Fatalf("G should jump to the end, cursor = %d", m.cursor)
	}
}

func TestSearchModeFiltersAndEscapeClearsIt(t *testing.T) {
	m := newTestModel(t, "# Alpha\n# Beta\n")

	m = press(t, m, key('/'))
	if m.mode != modeFilter {
		t.Fatalf("mode = %v, want the filter", m.mode)
	}
	m = press(t, m, key('b'))
	if len(m.visible) != 1 {
		t.Fatalf("visible = %v, want only Beta", m.visible)
	}
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeEntries || m.filter != "b" {
		t.Fatalf("enter should keep the filter and leave the mode: %v %q", m.mode, m.filter)
	}

	m = press(t, m, key('/'))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.filter != "" || len(m.visible) != 2 {
		t.Fatalf("escape should clear the filter: %q %v", m.filter, m.visible)
	}
}

func TestHelpOpensAndAnyKeyCloses(t *testing.T) {
	m := newTestModel(t, "# A\n")
	m = press(t, m, key('?'))
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want help", m.mode)
	}
	m = press(t, m, key('x'))
	if m.mode != modeEntries {
		t.Fatalf("mode = %v, want back in the list", m.mode)
	}
}

func TestFilePickerNavigation(t *testing.T) {
	m := newTestModel(t, "# A\n")
	m.files = []fileEntry{
		{Path: "/n/a.md", Source: "Notes", Label: "a"},
		{Path: "/n/b.md", Source: "Notes", Label: "b"},
	}
	m = press(t, m, key('f'))
	if m.mode != modeFiles {
		t.Fatalf("mode = %v, want the picker", m.mode)
	}
	m = press(t, m, key('j'))
	if m.fileCur != 1 {
		t.Fatalf("fileCur = %d, want 1", m.fileCur)
	}
	m = press(t, m, key('k'))
	m = press(t, m, key('k')) // past the top
	if m.fileCur != 0 {
		t.Fatalf("fileCur = %d, want it to stop at 0", m.fileCur)
	}
	// Escape returns to the entries of the open note.
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeEntries {
		t.Fatalf("mode = %v, want the list", m.mode)
	}
}

func TestPickerOpensTheSelectedNote(t *testing.T) {
	m := newTestModel(t, "# A\n")
	other := t.TempDir() + "/other.md"
	if err := os.WriteFile(other, []byte("# Elsewhere\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.files = []fileEntry{{Path: other, Source: "Notes", Label: "other"}}
	m.filter = "leftover"

	m = press(t, m, key('f'))
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.doc.Path != other {
		t.Fatalf("open note = %q, want %q", m.doc.Path, other)
	}
	if m.filter != "" {
		t.Fatalf("filter = %q, want it cleared when switching notes", m.filter)
	}
}

func TestTextModesIgnoreWindowKeys(t *testing.T) {
	// While typing, "+" and "-" are characters, not resize commands.
	m := newTestModel(t, "# A\n")
	m = press(t, m, key('/'))
	before := m.layout
	m = press(t, m, key('-'))
	if m.layout != before {
		t.Fatal("typing in the search box must not resize the window")
	}
	if !strings.Contains(m.filter, "-") {
		t.Fatalf("filter = %q, want the character to be typed", m.filter)
	}
}

func TestSuggestNoteNameWithoutATarget(t *testing.T) {
	m := newTestModel(t, "# A\n")
	m.target = ""
	if got := m.suggestNoteName(); got != "" {
		t.Fatalf("suggestNoteName = %q, want empty without a target", got)
	}
}
