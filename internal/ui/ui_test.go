package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ni-c/tmux-notepad/internal/config"
	"github.com/ni-c/tmux-notepad/internal/notes"
	"github.com/ni-c/tmux-notepad/internal/state"
)

// newTestModel builds a model around a note file, without touching tmux.
func newTestModel(t *testing.T, src string) Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := notes.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		cfg:        config.Default(),
		doc:        doc,
		input:      textinput.New(),
		pendingDel: -1,
		clientW:    120,
		clientH:    40,
		width:      100,
		height:     20,
		layout:     state.Layout{X: 10, Y: 5, W: 80, H: 24},
	}
	m.applyFilter()
	return m
}

func TestFilterMatchesTitleAndBody(t *testing.T) {
	m := newTestModel(t, "# Deploy\nstaging and production\n\n# Tests\nrun them\n\n# Other\nnothing\n")

	if len(m.visible) != 3 {
		t.Fatalf("unfiltered = %d entries, want 3", len(m.visible))
	}

	m.filter = "depl"
	m.applyFilter()
	if len(m.visible) != 1 || m.doc.Items[m.visible[0]].Title != "Deploy" {
		t.Fatalf("title match failed: %v", m.visible)
	}

	m.filter = "PRODUCTION" // body, and case-insensitive
	m.applyFilter()
	if len(m.visible) != 1 || m.doc.Items[m.visible[0]].Title != "Deploy" {
		t.Fatalf("body match failed: %v", m.visible)
	}

	m.filter = "zzz"
	m.applyFilter()
	if len(m.visible) != 0 {
		t.Fatalf("want no matches, got %v", m.visible)
	}

	m.filter = "   "
	m.applyFilter()
	if len(m.visible) != 3 {
		t.Fatalf("a blank filter should match everything, got %v", m.visible)
	}
}

func TestFilterPullsCursorIntoRange(t *testing.T) {
	m := newTestModel(t, "# A\n# B\n# C\n")
	m.cursor = 2
	m.filter = "A"
	m.applyFilter()
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after the list shrank", m.cursor)
	}
	if _, _, ok := m.current(); !ok {
		t.Fatal("current() must stay valid after filtering")
	}
}

func TestCurrentOnEmptyList(t *testing.T) {
	m := newTestModel(t, "")
	if _, _, ok := m.current(); ok {
		t.Fatal("current() should report no selection for an empty note")
	}
	m.cursor = -1
	if _, _, ok := m.current(); ok {
		t.Fatal("a negative cursor must not select anything")
	}
}

func TestOpenStartsAtTheFirstOpenEntry(t *testing.T) {
	m := newTestModel(t, "# ✓ done one\n# ✓ done two\n# open one\n# open two\n")
	if err := m.open(m.doc.Path); err != nil {
		t.Fatal(err)
	}
	it, _, ok := m.current()
	if !ok || it.Title != "open one" {
		t.Fatalf("selected %q, want the first entry that is not done", it.Title)
	}
}

func TestOpenAllDoneStartsAtTheTop(t *testing.T) {
	m := newTestModel(t, "# ✓ a\n# ✓ b\n")
	if err := m.open(m.doc.Path); err != nil {
		t.Fatal(err)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 when everything is done", m.cursor)
	}
}

func TestSnapshotAndRestoreSurviveAReopen(t *testing.T) {
	m := newTestModel(t, "# A\n# B\n# C\n")
	m.cursor = 2
	m.filter = "" // no filter, all visible
	st := m.snapshot()

	if st.SelectedTitle != "C" || st.Layout.W != 80 {
		t.Fatalf("snapshot = %+v", st)
	}

	// A fresh model restores the same entry.
	other := newTestModel(t, "# A\n# B\n# C\n")
	other.restoreSelection(st)
	it, _, ok := other.current()
	if !ok || it.Title != "C" {
		t.Fatalf("restored %q, want C", it.Title)
	}
}

func TestRestoreFallsBackToTheIndex(t *testing.T) {
	// The entry was renamed while the popup was reopening.
	st := state.State{SelectedTitle: "gone", SelectedIndex: 1}
	m := newTestModel(t, "# A\n# B\n# C\n")
	m.restoreSelection(st)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want the remembered index 1", m.cursor)
	}
}

func TestRestoreWithAnOutOfRangeIndex(t *testing.T) {
	st := state.State{SelectedTitle: "gone", SelectedIndex: 99}
	m := newTestModel(t, "# A\n# B\n")
	m.restoreSelection(st)
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		t.Fatalf("cursor = %d is out of range", m.cursor)
	}
}

func TestReloadKeepsTheSelection(t *testing.T) {
	m := newTestModel(t, "# A\nbody a\n# B\nbody b\n")
	m.cursor = 1

	// Someone appends an entry at the top in another editor.
	if err := os.WriteFile(m.doc.Path, []byte("# New\nx\n# A\nbody a\n# B\nbody b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reloadDoc()

	it, _, ok := m.current()
	if !ok || it.Title != "B" {
		t.Fatalf("after reload the selection is %q, want B", it.Title)
	}
	if len(m.doc.Items) != 3 {
		t.Fatalf("want 3 entries after the reload, got %d", len(m.doc.Items))
	}
}

func TestReloadWhenTheSelectedEntryDisappeared(t *testing.T) {
	m := newTestModel(t, "# A\n# B\n")
	m.cursor = 1
	if err := os.WriteFile(m.doc.Path, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.reloadDoc()
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		t.Fatalf("cursor = %d is out of range after the entry vanished", m.cursor)
	}
}

func TestHandleReloadIgnoresAnUnchangedFile(t *testing.T) {
	m := newTestModel(t, "# A\nbody\n")
	before := m.doc
	got := m.handleReload()
	if got.doc != before {
		t.Fatal("an unchanged file should not be re-read")
	}
}

func TestCapitalise(t *testing.T) {
	cases := map[string]string{
		"toolbox":       "Toolbox",
		"my-project":    "My Project",
		"my_project":    "My Project",
		"OpenAPI":       "OpenAPI",
		"two words":     "Two Words",
		"":              "",
		"ärger":         "Ärger",
		"-leading-dash": "Leading Dash",
	}
	for in, want := range cases {
		if got := capitalise(in); got != want {
			t.Errorf("capitalise(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrollStart(t *testing.T) {
	cases := []struct{ cursor, total, height, want int }{
		{0, 3, 10, 0},   // everything fits
		{0, 100, 10, 0}, // at the top
		{50, 100, 10, 45},
		{99, 100, 10, 90}, // at the bottom, no overshoot
		{5, 100, 0, 0},    // degenerate height
		{0, 0, 10, 0},     // empty list
	}
	for _, tc := range cases {
		if got := scrollStart(tc.cursor, tc.total, tc.height); got != tc.want {
			t.Errorf("scrollStart(%d,%d,%d) = %d, want %d",
				tc.cursor, tc.total, tc.height, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hell"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"äöü", 2, "äö"},
		{"🌲🌲🌲", 2, "🌲"}, // wide runes count double
	}
	for _, tc := range cases {
		if got := truncate(tc.in, tc.width); got != tc.want {
			t.Errorf("truncate(%q,%d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestWrap(t *testing.T) {
	got := wrap("one two three four", 9)
	want := []string{"one two", "three", "four"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrap = %q, want %q", got, want)
	}
	if got := wrap("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("wrap of an empty string = %q", got)
	}
	if got := wrap("word", 0); len(got) != 1 {
		t.Fatalf("wrap with width 0 = %q", got)
	}
}

func TestPadMakesAnExactBlock(t *testing.T) {
	got := pad("a\nbb", 4, 4)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}
	for _, l := range lines {
		if len([]rune(l)) != 4 {
			t.Fatalf("line %q is not 4 cells wide", l)
		}
	}

	// Too many lines get cut.
	if n := len(strings.Split(pad("a\nb\nc\nd", 2, 2), "\n")); n != 2 {
		t.Fatalf("got %d lines, want 2", n)
	}
}

func TestViewDoesNotPanicOnOddSizes(t *testing.T) {
	m := newTestModel(t, "# A\nbody\n# B\n")
	for _, size := range [][2]int{{0, 0}, {1, 1}, {10, 4}, {40, 8}, {59, 10}, {60, 10}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		for _, mode := range []mode{modeEntries, modeFiles, modeHelp, modeFilter, modePrompt, modeConfirm} {
			m.mode = mode
			_ = m.View()
		}
	}
}

func TestViewOnAnEmptyNote(t *testing.T) {
	m := newTestModel(t, "no headings here\n")
	out := m.View()
	if !strings.Contains(out, "no \"# heading\" entries") {
		t.Fatalf("view should explain the empty note, got:\n%s", out)
	}
}

func TestViewWhenNothingMatches(t *testing.T) {
	m := newTestModel(t, "# A\n")
	m.filter = "zzz"
	m.applyFilter()
	if out := m.View(); !strings.Contains(out, "nothing matches") {
		t.Fatalf("view should say nothing matches, got:\n%s", out)
	}
}

func TestSourceTroubleNamesTheMissingDirectory(t *testing.T) {
	m := Model{sources: []notes.Source{{Name: "Notes", Path: "/gone", Available: false}}}
	if got := m.sourceTrouble(); !strings.Contains(got, "/gone") {
		t.Fatalf("sourceTrouble = %q, want it to name the path", got)
	}

	m = Model{sources: []notes.Source{{Name: "Notes", Path: "/here", Available: true}}}
	if got := m.sourceTrouble(); !strings.Contains(got, "no notes") {
		t.Fatalf("sourceTrouble = %q", got)
	}

	m = Model{}
	if got := m.sourceTrouble(); !strings.Contains(got, "no sources configured") {
		t.Fatalf("sourceTrouble = %q", got)
	}
}

func TestWindowKeysMoveInConfiguredSteps(t *testing.T) {
	m := newTestModel(t, "# A\n")
	m.exePath = "tmux-notepad"
	m.layout = state.Layout{X: 20, Y: 10, W: 40, H: 20}

	handled, model, _ := m.handleWindowKey(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	if !handled {
		t.Fatal("alt+right should be handled")
	}
	if got := model.(Model).layout.X; got != 20+windowStepX {
		t.Fatalf("x = %d, want %d (one horizontal step)", got, 20+windowStepX)
	}

	handled, model, _ = m.handleWindowKey(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	if !handled {
		t.Fatal("alt+down should be handled")
	}
	if got := model.(Model).layout.Y; got != 10+windowStepY {
		t.Fatalf("y = %d, want %d — moving down must work like the other directions", got, 10+windowStepY)
	}
}

func TestWindowKeyAtTheEdgeSaysSo(t *testing.T) {
	m := newTestModel(t, "# A\n")
	// Flush against the bottom already.
	m.layout = state.Layout{X: 0, Y: m.clientH - 20, W: 40, H: 20}

	handled, model, cmd := m.handleWindowKey(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	if !handled {
		t.Fatal("the key should still count as handled")
	}
	if cmd != nil {
		t.Fatal("nothing moved, so the popup must not be reopened")
	}
	if status := model.(Model).status; !strings.Contains(status, "bottom edge") {
		t.Fatalf("status = %q, want it to explain that the edge is reached", status)
	}
}

func TestUnrelatedKeyIsNotAWindowKey(t *testing.T) {
	m := newTestModel(t, "# A\n")
	if handled, _, _ := m.handleWindowKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}); handled {
		t.Fatal("j must not be treated as a window key")
	}
}

func TestNarrowWindowStillShowsThePreview(t *testing.T) {
	// The preview is the point of the tool: a window too narrow for two
	// columns stacks it below the list rather than dropping it.
	m := newTestModel(t, "# A\nthe body of entry A\n# B\nsomething else\n")
	m.width, m.height = 40, 16
	out := m.View()
	if !strings.Contains(out, "body of entry A") {
		t.Fatalf("narrow view lost the preview:\n%s", out)
	}
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Fatalf("narrow view lost the list:\n%s", out)
	}
}

func TestVeryShortWindowFallsBackToTheListAlone(t *testing.T) {
	m := newTestModel(t, "# A\nbody\n")
	m.width, m.height = 30, 3
	_ = m.View() // must not panic; there is no room for both
}

func TestPlainOpenIgnoresTheRememberedGeometry(t *testing.T) {
	// A popup opened by the key binding must come up at the size the
	// binding gave it, not at a size the user once shrank it to.
	remembered := state.Layout{X: 4, Y: 4, W: 30, H: 10}
	st := state.State{Layout: remembered}

	if got := initialLayout(st, false); got.Valid() {
		t.Fatalf("plain open got layout %+v, want none so the binding decides", got)
	}
	if got := initialLayout(st, true); got != remembered {
		t.Fatalf("restore got layout %+v, want the remembered %+v", got, remembered)
	}
}

func TestFirstWindowSizeAdoptsTheActualSize(t *testing.T) {
	// With no layout yet, the first size message settles it — and must not
	// trigger a reopen, or every open would flicker.
	m := newTestModel(t, "# A\n")
	m.layout = state.Layout{}
	m.clientW, m.clientH = 200, 50

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Fatal("adopting the initial size must not reopen the popup")
	}
	got := updated.(Model).layout
	if got.W != 100+popupBorder || got.H != 30+popupBorder {
		t.Fatalf("layout = %+v, want the popup's actual size", got)
	}
	if got.X != (200-got.W)/2 {
		t.Fatalf("x = %d, want it centred per the configured placement", got.X)
	}
}

func TestOpenRememberedReopensTheLastNote(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	for path, body := range map[string]string{
		first:  "# Alpha\nbody\n",
		second: "# Beta\nbody\n# Gamma\nmore\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := Model{cfg: config.Default(), input: textinput.New(), pendingDel: -1}
	m.files = []fileEntry{
		{Path: first, Source: "Notes", Label: "first"},
		{Path: second, Source: "Notes", Label: "second"},
	}
	m.sources = []notes.Source{{Name: "Notes", Path: dir, Available: true}}

	m.openRemembered(state.State{NotePath: second, SelectedTitle: "Gamma"})
	if m.doc == nil || m.doc.Path != second {
		t.Fatalf("open note = %v, want %q", m.doc, second)
	}
	if it, _, _ := m.current(); it.Title != "Gamma" {
		t.Fatalf("selection = %q, want the remembered Gamma", it.Title)
	}
	if m.mode != modeEntries {
		t.Fatalf("mode = %v, want the entry list", m.mode)
	}
}

func TestOpenRememberedFallsBackToThePickerWhenTheNoteIsGone(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("# X\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{cfg: config.Default(), input: textinput.New(), pendingDel: -1}
	m.files = []fileEntry{{Path: a, Label: "a"}, {Path: b, Label: "b"}}
	m.sources = []notes.Source{{Name: "Notes", Path: dir, Available: true}}

	m.openRemembered(state.State{NotePath: filepath.Join(dir, "deleted.md")})
	if m.mode != modeFiles {
		t.Fatalf("mode = %v, want the picker when the remembered note is gone", m.mode)
	}
}

func TestOpenRememberedOpensTheOnlyNoteStraightAway(t *testing.T) {
	dir := t.TempDir()
	only := filepath.Join(dir, "only.md")
	if err := os.WriteFile(only, []byte("# X\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Model{cfg: config.Default(), input: textinput.New(), pendingDel: -1}
	m.files = []fileEntry{{Path: only, Label: "only"}}
	m.sources = []notes.Source{{Name: "Notes", Path: dir, Available: true}}

	m.openRemembered(state.State{})
	if m.doc == nil || m.doc.Path != only {
		t.Fatalf("a single note should open without asking, got %v", m.doc)
	}
}

func TestOpenRememberedWithNoNotesAtAllExplains(t *testing.T) {
	m := Model{cfg: config.Default(), input: textinput.New(), pendingDel: -1}
	m.sources = []notes.Source{{Name: "Notes", Path: "/gone", Available: false}}

	m.openRemembered(state.State{})
	if m.mode != modeFiles {
		t.Fatalf("mode = %v, want the picker", m.mode)
	}
	if !strings.Contains(m.status, "/gone") {
		t.Fatalf("status = %q, want it to name the unreachable directory", m.status)
	}
}
