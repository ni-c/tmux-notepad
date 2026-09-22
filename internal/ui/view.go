package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/ni-c/tmux-notepad/internal/notes"
)

// Colours are kept to the terminal's own 16, so the popup sits inside any
// theme the user has rather than fighting it.
var (
	styleHeader   = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleKey      = lipgloss.NewStyle().Bold(true)
)

// listMinWidth is the narrowest the entry list is allowed to get; below the
// split width the preview is dropped instead.
const (
	listMinWidth = 18
	listMaxWidth = 34
	splitWidth   = 60
)

// View renders the popup.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	width, height := m.width, m.height
	if width <= 0 {
		width = m.layout.W
	}
	if height <= 0 {
		height = m.layout.H
	}
	if width < 10 || height < 4 {
		return "too small"
	}

	header := m.renderHeader(width)
	footer := m.renderFooter(width)
	bodyHeight := height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var body string
	switch m.mode {
	case modeFiles:
		body = m.renderFiles(width, bodyHeight)
	case modeHelp:
		body = m.renderHelp(width, bodyHeight)
	default:
		body = m.renderEntries(width, bodyHeight)
	}
	return header + "\n" + body + "\n" + footer
}

// renderHeader shows the open note on the left and the target pane on the
// right.
func (m Model) renderHeader(width int) string {
	left := "notepad"
	if m.doc != nil {
		left = m.doc.Name
	}
	if m.mode == modeFiles {
		left = "choose a note"
	}
	right := "→ " + m.targetHint()

	left = styleHeader.Render(truncate(left, max(1, width-lipgloss.Width(right)-1)))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + styleDim.Render(right)
}

// renderEntries draws the list and, when there is room, the preview.
func (m Model) renderEntries(width, height int) string {
	if m.doc == nil {
		return m.renderEmpty(width, height, "no note open — press f to choose one")
	}
	if len(m.doc.Items) == 0 {
		return m.renderEmpty(width, height, "this note has no \"# heading\" entries yet — press a to add one")
	}
	if len(m.visible) == 0 {
		return m.renderEmpty(width, height, "nothing matches "+quote(m.filter))
	}

	if width < splitWidth {
		return m.renderStacked(width, height)
	}
	listWidth := clamp(width/3, listMinWidth, listMaxWidth)
	previewWidth := width - listWidth - 3

	list := pad(m.renderList(listWidth, height), listWidth, height)
	preview := pad(m.renderPreview(previewWidth, height), previewWidth, height)

	sep := styleDim.Render("│")
	var b strings.Builder
	listLines := strings.Split(list, "\n")
	previewLines := strings.Split(preview, "\n")
	for i := 0; i < height; i++ {
		b.WriteString(line(listLines, i))
		b.WriteString(" " + sep + " ")
		b.WriteString(line(previewLines, i))
		if i < height-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderStacked puts the preview below the list, for a window too narrow to
// hold both side by side. The preview is never dropped: seeing what will be
// pasted is the point of the thing.
func (m Model) renderStacked(width, height int) string {
	if height < 4 {
		return pad(m.renderList(width, height), width, height)
	}
	listHeight := len(m.visible)
	if maxList := height / 2; listHeight > maxList {
		listHeight = maxList
	}
	if listHeight < 1 {
		listHeight = 1
	}
	previewHeight := height - listHeight - 1

	var b strings.Builder
	b.WriteString(pad(m.renderList(width, listHeight), width, listHeight))
	b.WriteString("\n")
	b.WriteString(styleDim.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(pad(m.renderPreview(width, previewHeight), width, previewHeight))
	return b.String()
}

// renderList draws the entry titles.
func (m Model) renderList(width, height int) string {
	start := scrollStart(m.cursor, len(m.visible), height)
	var b strings.Builder
	for row := 0; row < height; row++ {
		i := start + row
		if i >= len(m.visible) {
			break
		}
		item := m.doc.Items[m.visible[i]]
		b.WriteString(m.renderListRow(item, i == m.cursor, width))
		if row < height-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) renderListRow(item notes.Item, selected bool, width int) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}
	title := item.Title
	if title == "" {
		title = "(untitled)"
	}
	if item.Done {
		title = notes.DoneMarker + " " + title
	}
	text := truncate(marker+title, width)
	text = text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
	switch {
	case selected:
		return styleSelected.Render(text)
	case item.Done:
		return styleDim.Render(text)
	default:
		return text
	}
}

// renderPreview renders the selected entry as Markdown.
func (m Model) renderPreview(width, height int) string {
	item, _, ok := m.current()
	if !ok {
		return ""
	}
	source := item.Body
	if source == "" {
		source = styleDim.Render("(no text below the heading — the title itself gets pasted)")
		return source
	}
	out, err := renderMarkdown(source, width)
	if err != nil {
		out = source
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
		if height > 0 {
			lines[height-1] = truncate(lines[height-1], max(1, width-1)) + styleDim.Render("…")
		}
	}
	return strings.Join(lines, "\n")
}

// rendererCache keeps one glamour renderer per width, since building one is
// not free and the width only changes when the popup is resized.
var rendererCache = map[int]*glamour.TermRenderer{}

func renderMarkdown(src string, width int) (string, error) {
	if width < 10 {
		width = 10
	}
	r, ok := rendererCache[width]
	if !ok {
		var err error
		r, err = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
			glamour.WithEmoji(),
		)
		if err != nil {
			return "", err
		}
		rendererCache[width] = r
	}
	return r.Render(src)
}

// renderFiles draws the note picker.
func (m Model) renderFiles(width, height int) string {
	if len(m.files) == 0 {
		return m.renderEmpty(width, height, m.sourceTrouble()+" — press n to create a note")
	}
	start := scrollStart(m.fileCur, len(m.files), height)
	var b strings.Builder
	for row := 0; row < height; row++ {
		i := start + row
		if i >= len(m.files) {
			break
		}
		f := m.files[i]
		marker := "  "
		if i == m.fileCur {
			marker = "▸ "
		}
		label := f.Label
		if len(m.sources) > 1 {
			label = f.Source + "/" + label
		}
		text := truncate(marker+label, width)
		text += strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
		if i == m.fileCur {
			text = styleSelected.Render(text)
		}
		b.WriteString(text)
		if row < height-1 {
			b.WriteString("\n")
		}
	}
	return pad(b.String(), width, height)
}

// renderHelp lists the keys.
func (m Model) renderHelp(width, height int) string {
	rows := [][2]string{
		{"enter", "paste into the pane and close"},
		{"tab", "paste and stay open"},
		{"x", "tick off / un-tick without pasting"},
		{"↑ ↓ / j k", "move the selection"},
		{"/", "search"},
		{"f", "choose another note"},
		{"n", "new note"},
		{"a", "add an entry"},
		{"d", "delete the entry"},
		{"e", "open the note in $EDITOR"},
		{"r", "reload"},
		{"alt+←→↑↓", "move the window"},
		{"shift+←→↑↓", "resize the window"},
		{"+ / -", "bigger / smaller"},
		{"0", "back to the default size"},
		{"q / esc", "close"},
	}
	var b strings.Builder
	for i, r := range rows {
		if i >= height {
			break
		}
		b.WriteString(truncate(styleKey.Render(fmt.Sprintf("%-12s", r[0]))+r[1], width))
		if i < len(rows)-1 && i < height-1 {
			b.WriteString("\n")
		}
	}
	return pad(b.String(), width, height)
}

// renderEmpty centres a message vertically.
func (m Model) renderEmpty(width, height int, msg string) string {
	lines := wrap(msg, width)
	top := max(0, (height-len(lines))/2)
	var b strings.Builder
	for i := 0; i < top; i++ {
		b.WriteString("\n")
	}
	b.WriteString(styleDim.Render(strings.Join(lines, "\n")))
	return pad(b.String(), width, height)
}

// renderFooter shows the prompt, the status message, or the key hints.
func (m Model) renderFooter(width int) string {
	switch m.mode {
	case modeFilter:
		return truncate(styleKey.Render("search: ")+m.input.View(), width)
	case modePrompt:
		label := "new entry: "
		if m.prompt == promptNewNote {
			label = "new note: "
		}
		return truncate(styleKey.Render(label)+m.input.View(), width)
	case modeConfirm:
		title := ""
		if m.pendingDel >= 0 && m.doc != nil && m.pendingDel < len(m.doc.Items) {
			title = m.doc.Items[m.pendingDel].Title
		}
		return truncate(styleError.Render("delete ")+quote(title)+styleError.Render("? [y/N]"), width)
	}

	if m.status != "" {
		style := styleOK
		if m.isError {
			style = styleError
		}
		return truncate(style.Render(m.status), width)
	}

	hints := [][2]string{
		{"enter", "paste"},
		{"tab", "paste·stay"},
		{"/", "search"},
		{"f", "note"},
		{"?", "keys"},
		{"q", "close"},
	}
	if m.mode == modeFiles {
		hints = [][2]string{
			{"enter", "open"},
			{"n", "new"},
			{"esc", "back"},
			{"q", "close"},
		}
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, styleKey.Render(h[0])+" "+styleDim.Render(h[1]))
	}
	return truncate(strings.Join(parts, styleDim.Render("  ")), width)
}

// --- small helpers -------------------------------------------------------

// scrollStart returns the first visible index so that cursor stays on screen.
func scrollStart(cursor, total, height int) int {
	if total <= height || height <= 0 {
		return 0
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	if start > total-height {
		start = total - height
	}
	return start
}

// line returns lines[i] or an empty string.
func line(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// pad makes a block exactly height lines of at most width columns.
func pad(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > width {
			lines[i] = truncate(l, width)
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, l := range lines {
		if gap := width - lipgloss.Width(l); gap > 0 {
			lines[i] = l + strings.Repeat(" ", gap)
		}
	}
	return strings.Join(lines, "\n")
}

// truncate cuts a string to width display cells, keeping escape sequences
// intact by measuring with lipgloss.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	// Cut rune by rune; styled strings are short enough that this is fine.
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > width {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// wrap breaks a message into lines of at most width cells.
func wrap(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	var cur string
	for _, word := range strings.Fields(s) {
		switch {
		case cur == "":
			cur = word
		case lipgloss.Width(cur+" "+word) <= width:
			cur += " " + word
		default:
			out = append(out, cur)
			cur = word
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func quote(s string) string { return "\"" + s + "\"" }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
