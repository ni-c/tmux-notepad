// Package ui is the popup itself: a list of entries on the left, a rendered
// preview on the right, and the keys that paste one into a pane.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ni-c/tmux-notepad/internal/config"
	"github.com/ni-c/tmux-notepad/internal/notes"
	"github.com/ni-c/tmux-notepad/internal/state"
	"github.com/ni-c/tmux-notepad/internal/tmuxio"
)

// mode is what the popup is currently doing.
type mode int

const (
	modeEntries mode = iota // browsing the entries of a note
	modeFiles               // choosing a note
	modeFilter              // typing a search term
	modePrompt              // typing a title for a new entry or note
	modeConfirm             // confirming a deletion
	modeHelp                // showing the key list
)

// promptKind says what a typed line will be used for.
type promptKind int

const (
	promptAppend promptKind = iota
	promptNewNote
)

// fileEntry is one selectable note file.
type fileEntry struct {
	Path   string
	Source string
	Label  string
}

// Model is the bubbletea model.
type Model struct {
	cfg        config.Config
	statePath  string
	configPath string
	target     string
	exePath    string

	sources []notes.Source
	files   []fileEntry
	doc     *notes.Doc

	mode    mode
	cursor  int
	fileCur int
	visible []int // indices into doc.Items, after filtering
	filter  string

	input      textinput.Model
	prompt     promptKind
	pendingDel int

	layout           state.Layout
	clientW, clientH int
	width, height    int

	status  string
	isError bool

	// quitting suppresses the final redraw when the popup is closing.
	quitting bool
}

// popupBorder is how many cells the popup border takes off each dimension.
const popupBorder = 2

// New builds the initial model.
func New(cfg config.Config, st state.State, statePath, configPath, target string, restored bool) Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 512

	m := Model{
		cfg:        cfg,
		statePath:  statePath,
		configPath: configPath,
		target:     target,
		filter:     st.Filter,
		input:      in,
		pendingDel: -1,
	}
	m.layout = initialLayout(st, restored)
	if exe, err := os.Executable(); err == nil {
		m.exePath = exe
	} else {
		m.exePath = "tmux-notepad"
	}

	m.clientW, m.clientH, _ = clientSize()
	// Keep the popup clear of the status line, otherwise the bottom rows of
	// the travel range are unreachable in practice.
	m.clientH -= tmuxio.StatusLines()
	// The layout is deliberately left unresolved when nothing was
	// remembered: the popup already has whatever size the key binding gave
	// it, and adopting that is better than forcing the configured size and
	// reopening on every single start.

	m.scan()
	m.openRemembered(st)
	return m
}

// initialLayout decides which geometry a starting popup gets.
//
// A remembered geometry is only carried across a reopen — the popup moving or
// resizing itself — never into a fresh open. A popup opened by the key binding
// always comes up at the size the binding gives it, which is wide enough to
// show the preview; remembering a size the user once shrank it to made it open
// too small to be useful.
func initialLayout(st state.State, restored bool) state.Layout {
	if restored {
		return st.Layout
	}
	return state.Layout{}
}

// clientSize returns the size of the tmux client, with a sane fallback.
func clientSize() (int, int, error) {
	w, h, err := tmuxio.ClientSize()
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24, err
	}
	return w, h, nil
}

// scan reads all configured sources.
func (m *Model) scan() {
	m.sources = nil
	m.files = nil
	for _, s := range m.cfg.ResolvedSources() {
		src := notes.Scan(s.Name, s.Path, s.Recursive, s.Exclude)
		m.sources = append(m.sources, src)
		for _, f := range src.Files {
			rel, err := filepath.Rel(src.Path, f)
			if err != nil {
				rel = filepath.Base(f)
			}
			m.files = append(m.files, fileEntry{
				Path:   f,
				Source: src.Name,
				Label:  strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel)),
			})
		}
	}
	sort.SliceStable(m.files, func(i, j int) bool {
		if m.files[i].Source != m.files[j].Source {
			return m.files[i].Source < m.files[j].Source
		}
		return strings.ToLower(m.files[i].Label) < strings.ToLower(m.files[j].Label)
	})
}

// openRemembered opens the note the popup had last, falling back to the file
// picker.
func (m *Model) openRemembered(st state.State) {
	if st.NotePath != "" {
		if err := m.open(st.NotePath); err == nil {
			m.restoreSelection(st)
			return
		}
	}
	if len(m.files) == 1 {
		if err := m.open(m.files[0].Path); err == nil {
			return
		}
	}
	m.mode = modeFiles
	if len(m.files) == 0 {
		m.setStatus(m.sourceTrouble(), true)
	}
}

// sourceTrouble explains why no notes showed up.
func (m Model) sourceTrouble() string {
	var missing []string
	for _, s := range m.sources {
		if !s.Available {
			missing = append(missing, s.Path)
		}
	}
	if len(missing) > 0 {
		return "not reachable: " + strings.Join(missing, ", ")
	}
	if len(m.sources) == 0 {
		return "no sources configured — see " + config.DefaultPath()
	}
	return "no notes found"
}

// restoreSelection puts the cursor back where it was before a reopen.
func (m *Model) restoreSelection(st state.State) {
	m.applyFilter()
	if st.SelectedTitle != "" {
		for i, idx := range m.visible {
			if m.doc.Items[idx].Title == st.SelectedTitle {
				m.cursor = i
				return
			}
		}
	}
	if st.SelectedIndex < len(m.visible) {
		m.cursor = st.SelectedIndex
	}
}

// open loads a note file.
func (m *Model) open(path string) error {
	doc, err := notes.Load(path)
	if err != nil {
		m.setStatus(err.Error(), true)
		return err
	}
	m.doc = doc
	m.cursor = 0
	m.mode = modeEntries
	m.applyFilter()
	if open := doc.FirstOpen(); open >= 0 {
		for i, idx := range m.visible {
			if idx == open {
				m.cursor = i
				break
			}
		}
	}
	return nil
}

// applyFilter recomputes which entries are visible.
func (m *Model) applyFilter() {
	m.visible = nil
	if m.doc == nil {
		return
	}
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	for i, it := range m.doc.Items {
		if needle == "" || strings.Contains(strings.ToLower(it.Title), needle) ||
			strings.Contains(strings.ToLower(it.Body), needle) {
			m.visible = append(m.visible, i)
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

// current returns the highlighted entry.
func (m Model) current() (notes.Item, int, bool) {
	if m.doc == nil || m.cursor < 0 || m.cursor >= len(m.visible) {
		return notes.Item{}, -1, false
	}
	idx := m.visible[m.cursor]
	return m.doc.Items[idx], idx, true
}

func (m *Model) setStatus(msg string, isErr bool) {
	m.status = msg
	m.isError = isErr
}

// snapshot captures what has to survive a reopen.
func (m Model) snapshot() state.State {
	st := state.State{
		Filter: m.filter,
		Target: m.target,
		Layout: m.layout,
	}
	if m.doc != nil {
		st.NotePath = m.doc.Path
	}
	if it, _, ok := m.current(); ok {
		st.SelectedTitle = it.Title
		st.SelectedIndex = m.cursor
	}
	return st
}

// Init starts the file watcher.
func (m Model) Init() tea.Cmd {
	if !m.cfg.Behavior.Watch {
		return textinput.Blink
	}
	return tea.Batch(textinput.Blink, watchTick())
}

// Update handles a message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.layout.Valid() {
			// First open: adopt the size the popup actually has, and
			// derive the position from the configured placement. No
			// reopen, so nothing flickers on the way in.
			m.layout = state.Resolve(
				strconv.Itoa(msg.Width+popupBorder), strconv.Itoa(msg.Height+popupBorder),
				m.cfg.Window.X, m.cfg.Window.Y, m.clientW, m.clientH)
		}
		return m, nil

	case reloadMsg:
		return m.handleReload(), watchTick()

	case editorDoneMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		}
		m.reloadDoc()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey dispatches a key press to the active mode.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Window management works in every mode that is not taking text.
	if m.mode != modeFilter && m.mode != modePrompt {
		if handled, model, cmd := m.handleWindowKey(msg); handled {
			return model, cmd
		}
	}

	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modePrompt:
		return m.handlePromptKey(msg)
	case modeConfirm:
		return m.handleConfirmKey(msg)
	case modeFiles:
		return m.handleFilesKey(msg)
	case modeHelp:
		m.mode = modeEntries
		return m, nil
	default:
		return m.handleEntriesKey(msg)
	}
}

// How far one key press moves or resizes the popup. Rows are worth more than
// columns on a terminal, so vertical steps are smaller.
const (
	windowStepX = 8
	windowStepY = 3
)

// handleWindowKey deals with moving and resizing the popup.
func (m Model) handleWindowKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	l := m.layout
	var what string
	switch msg.String() {
	case "alt+left":
		l, what = l.Move(-windowStepX, 0, m.clientW, m.clientH), "left edge"
	case "alt+right":
		l, what = l.Move(windowStepX, 0, m.clientW, m.clientH), "right edge"
	case "alt+up":
		l, what = l.Move(0, -windowStepY, m.clientW, m.clientH), "top edge"
	case "alt+down":
		l, what = l.Move(0, windowStepY, m.clientW, m.clientH), "bottom edge"
	case "shift+left":
		l, what = l.Resize(-windowStepX, 0, m.clientW, m.clientH), "minimum width"
	case "shift+right":
		l, what = l.Resize(windowStepX, 0, m.clientW, m.clientH), "maximum width"
	case "shift+up":
		l, what = l.Resize(0, -windowStepY, m.clientW, m.clientH), "minimum height"
	case "shift+down":
		l, what = l.Resize(0, windowStepY, m.clientW, m.clientH), "maximum height"
	case "+", "=":
		l, what = l.Resize(windowStepX, windowStepY, m.clientW, m.clientH), "maximum size"
	case "-", "_":
		l, what = l.Resize(-windowStepX, -windowStepY, m.clientW, m.clientH), "minimum size"
	case "0":
		l, what = state.Resolve(m.cfg.Window.Width, m.cfg.Window.Height,
			m.cfg.Window.X, m.cfg.Window.Y, m.clientW, m.clientH), "default size"
	default:
		return false, m, nil
	}
	if l == m.layout {
		// Say so rather than appearing to ignore the key.
		m.setStatus("already at the "+what, false)
		return true, m, nil
	}
	m.layout = l
	return true, m, m.reopen()
}

// reopen closes this popup and opens a new one at the current geometry.
func (m Model) reopen() tea.Cmd {
	st := m.snapshot()
	if err := state.Save(m.statePath, st); err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	w, h, x, y := m.layout.Args()
	// Every flag this process was started with has to travel along, or the
	// reopened popup would fall back to the default configuration.
	args := []string{"--restore"}
	if m.target != "" {
		args = append(args, "--target", m.target)
	}
	if m.configPath != "" {
		args = append(args, "--config", m.configPath)
	}
	if m.statePath != "" {
		args = append(args, "--state", m.statePath)
	}
	if err := tmuxio.Reopen(tmuxio.Geometry{Width: w, Height: h, X: x, Y: y}, m.exePath, args...); err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	return tea.Quit
}

// handleEntriesKey handles the main list.
func (m Model) handleEntriesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m.quit()
	case "up", "k", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j", "ctrl+n":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
		}
		return m, nil
	case "home", "g":
		m.cursor = 0
		return m, nil
	case "end", "G":
		m.cursor = max(0, len(m.visible)-1)
		return m, nil
	case "enter":
		return m.send(m.cfg.Behavior.CloseOnSend)
	case "tab":
		return m.send(false)
	case "/":
		m.mode = modeFilter
		m.input.SetValue(m.filter)
		m.input.Focus()
		return m, textinput.Blink
	case "f":
		m.mode = modeFiles
		m.syncFileCursor()
		return m, nil
	case "n":
		return m.startPrompt(promptNewNote)
	case "a":
		if m.doc == nil {
			return m, nil
		}
		return m.startPrompt(promptAppend)
	case "d":
		if _, idx, ok := m.current(); ok {
			m.pendingDel = idx
			m.mode = modeConfirm
		}
		return m, nil
	case "e":
		return m, m.editCmd()
	case "r":
		m.scan()
		m.reloadDoc()
		m.setStatus("reloaded", false)
		return m, nil
	case "x":
		return m.toggleDone()
	case "?":
		m.mode = modeHelp
		return m, nil
	}
	return m, nil
}

// send pastes the selected entry into the target pane.
func (m Model) send(closeAfter bool) (tea.Model, tea.Cmd) {
	item, idx, ok := m.current()
	if !ok {
		m.setStatus("nothing selected", true)
		return m, nil
	}
	if m.target == "" {
		m.setStatus("no target pane", true)
		return m, nil
	}
	if !tmuxio.PaneExists(m.target) {
		m.setStatus("target pane "+m.target+" is gone", true)
		return m, nil
	}

	text := item.Payload()
	if m.cfg.Behavior.IncludeTitle && item.Body != "" {
		text = item.Title + "\n" + text
	}
	if err := tmuxio.Paste(m.target, text); err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}

	if m.cfg.Behavior.MarkDone == config.MarkDoneCheckmark && !item.Done {
		if err := m.doc.MarkDone(idx, true); err != nil {
			if err == notes.ErrConflict {
				m.reloadDoc()
				m.setStatus("pasted — note changed elsewhere, reloaded without ticking it off", true)
			} else {
				m.setStatus("pasted, but could not tick it off: "+err.Error(), true)
			}
			if closeAfter {
				return m.quitToPane()
			}
			return m, nil
		}
		m.applyFilter()
	}

	if closeAfter {
		return m.quitToPane()
	}
	m.setStatus("pasted: "+item.Title, false)
	if m.cursor < len(m.visible)-1 {
		m.cursor++
	}
	return m, nil
}

// toggleDone ticks an entry off, or un-ticks it, without pasting.
func (m Model) toggleDone() (tea.Model, tea.Cmd) {
	item, idx, ok := m.current()
	if !ok {
		return m, nil
	}
	if err := m.doc.MarkDone(idx, !item.Done); err != nil {
		if err == notes.ErrConflict {
			m.reloadDoc()
			m.setStatus("note changed elsewhere — reloaded", true)
		} else {
			m.setStatus(err.Error(), true)
		}
		return m, nil
	}
	m.applyFilter()
	return m, nil
}

// quitToPane closes the popup and puts the focus on the target pane so the
// user can press Enter.
func (m Model) quitToPane() (tea.Model, tea.Cmd) {
	if m.target != "" {
		_ = tmuxio.SelectPane(m.target)
	}
	return m.quit()
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	_ = state.Save(m.statePath, m.snapshot())
	m.quitting = true
	return m, tea.Quit
}

// handleFilesKey handles the note picker.
func (m Model) handleFilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m.quit()
	case "esc":
		if m.doc != nil {
			m.mode = modeEntries
		}
		return m, nil
	case "up", "k", "ctrl+p":
		if m.fileCur > 0 {
			m.fileCur--
		}
		return m, nil
	case "down", "j", "ctrl+n":
		if m.fileCur < len(m.files)-1 {
			m.fileCur++
		}
		return m, nil
	case "n":
		return m.startPrompt(promptNewNote)
	case "r":
		m.scan()
		return m, nil
	case "enter":
		if m.fileCur >= 0 && m.fileCur < len(m.files) {
			m.filter = ""
			_ = m.open(m.files[m.fileCur].Path)
		}
		return m, nil
	}
	return m, nil
}

// syncFileCursor points the picker at the currently open note.
func (m *Model) syncFileCursor() {
	if m.doc == nil {
		return
	}
	for i, f := range m.files {
		if f.Path == m.doc.Path {
			m.fileCur = i
			return
		}
	}
}

// handleFilterKey handles typing a search term.
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filter = ""
		m.applyFilter()
		m.mode = modeEntries
		m.input.Blur()
		return m, nil
	case "enter":
		m.mode = modeEntries
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filter = m.input.Value()
	m.applyFilter()
	return m, cmd
}

// startPrompt opens the single-line input.
func (m Model) startPrompt(kind promptKind) (tea.Model, tea.Cmd) {
	m.prompt = kind
	m.mode = modePrompt
	m.input.SetValue("")
	if kind == promptNewNote {
		m.input.SetValue(m.suggestNoteName())
	}
	m.input.CursorEnd()
	m.input.Focus()
	return m, textinput.Blink
}

// suggestNoteName proposes a name for a new note, taken from the project the
// target pane sits in.
func (m Model) suggestNoteName() string {
	if m.target == "" {
		return ""
	}
	dir := tmuxio.PanePath(m.target)
	if dir == "" {
		return ""
	}
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return ""
	}
	return capitalise(base) + " Prompts"
}

// capitalise upper-cases the first letter of every word, leaving the rest
// alone so that "OpenAPI" does not become "Openapi".
func capitalise(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, w := range words {
		r := []rune(w)
		if len(r) == 0 {
			continue
		}
		words[i] = string(unicode.ToUpper(r[0])) + string(r[1:])
	}
	return strings.Join(words, " ")
}

// handlePromptKey handles typing a title.
func (m Model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeEntries
		if m.doc == nil {
			m.mode = modeFiles
		}
		m.input.Blur()
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		m.input.Blur()
		if value == "" {
			m.mode = modeEntries
			return m, nil
		}
		switch m.prompt {
		case promptAppend:
			return m.appendEntry(value)
		case promptNewNote:
			return m.createNote(value)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// appendEntry adds a new entry to the open note.
func (m Model) appendEntry(title string) (tea.Model, tea.Cmd) {
	if m.doc == nil {
		m.mode = modeFiles
		return m, nil
	}
	if err := m.doc.Append(title, ""); err != nil {
		if err == notes.ErrConflict {
			m.reloadDoc()
			m.setStatus("note changed elsewhere — reloaded, nothing added", true)
		} else {
			m.setStatus(err.Error(), true)
		}
		m.mode = modeEntries
		return m, nil
	}
	m.applyFilter()
	for i, idx := range m.visible {
		if m.doc.Items[idx].Title == title {
			m.cursor = i
		}
	}
	m.mode = modeEntries
	m.setStatus("added: "+title, false)
	return m, nil
}

// createNote creates a new note file in the first writable source.
func (m Model) createNote(name string) (tea.Model, tea.Cmd) {
	dir := ""
	for _, s := range m.sources {
		if s.Available {
			dir = s.Path
			break
		}
	}
	if dir == "" {
		m.setStatus("no reachable note directory to create it in", true)
		m.mode = modeFiles
		return m, nil
	}
	name = strings.TrimSuffix(name, ".md")
	path := filepath.Join(dir, name+".md")
	if err := notes.Create(path); err != nil {
		m.setStatus(err.Error(), true)
		m.mode = modeFiles
		return m, nil
	}
	m.scan()
	m.filter = ""
	_ = m.open(path)
	m.setStatus("created: "+name, false)
	return m, nil
}

// handleConfirmKey handles the delete confirmation.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		idx := m.pendingDel
		m.pendingDel = -1
		m.mode = modeEntries
		if m.doc == nil || idx < 0 {
			return m, nil
		}
		title := ""
		if idx < len(m.doc.Items) {
			title = m.doc.Items[idx].Title
		}
		if err := m.doc.Delete(idx); err != nil {
			if err == notes.ErrConflict {
				m.reloadDoc()
				m.setStatus("note changed elsewhere — reloaded, nothing deleted", true)
			} else {
				m.setStatus(err.Error(), true)
			}
			return m, nil
		}
		m.applyFilter()
		m.setStatus("deleted: "+title, false)
		return m, nil
	default:
		m.pendingDel = -1
		m.mode = modeEntries
		return m, nil
	}
}

// editCmd opens the note in the user's editor, inside the popup.
func (m Model) editCmd() tea.Cmd {
	if m.doc == nil {
		return nil
	}
	editor := m.cfg.Editor()
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return nil
	}
	args := append(parts[1:], m.doc.Path)
	cmd := exec.Command(parts[0], args...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{err: err}
	})
}

// reloadDoc re-reads the open note, keeping the selected entry where possible.
func (m *Model) reloadDoc() {
	if m.doc == nil {
		return
	}
	var selected string
	if it, _, ok := m.current(); ok {
		selected = it.Title
	}
	doc, err := notes.Load(m.doc.Path)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.doc = doc
	m.applyFilter()
	if selected != "" {
		for i, idx := range m.visible {
			if m.doc.Items[idx].Title == selected {
				m.cursor = i
				break
			}
		}
	}
}

// handleReload reacts to the watcher tick.
func (m Model) handleReload() Model {
	if m.doc == nil {
		return m
	}
	info, err := os.Stat(m.doc.Path)
	if err != nil {
		return m
	}
	if info.Size() == m.doc.Size && info.ModTime().Equal(m.doc.ModTime) {
		return m
	}
	m.reloadDoc()
	return m
}

// targetHint describes the pane entries go to.
func (m Model) targetHint() string {
	if m.target == "" {
		return "no target"
	}
	if cmd := tmuxio.PaneCommand(m.target); cmd != "" {
		return fmt.Sprintf("%s (%s)", m.target, cmd)
	}
	return m.target
}
