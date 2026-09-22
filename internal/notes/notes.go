// Package notes reads Markdown note files and splits them into entries.
//
// An entry starts at a top-level ATX heading ("# Title") at the beginning of a
// line and extends to the next one. The heading is only a label: what gets sent
// to a pane is the body below it. A heading whose title starts with the done
// marker is considered already sent.
package notes

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DoneMarker prefixes the title of an entry that has been sent.
const DoneMarker = "✓"

// Item is a single entry of a note.
type Item struct {
	// Title is the heading text without the leading "# " and without the
	// done marker.
	Title string
	// Done reports whether the heading carries the done marker.
	Done bool
	// Body is the text below the heading, stripped of surrounding blank
	// lines. It is what gets pasted into a pane.
	Body string
	// HeadingLine is the index into Doc.Lines of this item's heading.
	HeadingLine int
	// EndLine is the index of the last line belonging to this item,
	// inclusive.
	EndLine int
}

// Payload returns the text to paste into a pane. It is the body, or the title
// when the entry has no body — so that a one-liner like "# make test" works
// without a separate body line.
func (i Item) Payload() string {
	if i.Body != "" {
		return i.Body
	}
	return i.Title
}

// Doc is a parsed note file.
type Doc struct {
	Path  string
	Name  string
	Lines []string
	Items []Item

	// ModTime and Size are recorded at read time and are used to detect
	// concurrent modification before writing back.
	ModTime time.Time
	Size    int64

	// crlf reports whether the file used CRLF line endings, so writing back
	// preserves them.
	crlf bool
	// finalNewline reports whether the file ended with a line break.
	finalNewline bool
}

// Load reads and parses the note file at path.
func Load(path string) (*Doc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	doc := Parse(string(raw))
	doc.Path = path
	doc.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	doc.ModTime = info.ModTime()
	doc.Size = info.Size()
	return doc, nil
}

// Parse splits Markdown source into items.
func Parse(src string) *Doc {
	doc := &Doc{}
	doc.crlf = strings.Contains(src, "\r\n")
	if doc.crlf {
		src = strings.ReplaceAll(src, "\r\n", "\n")
	}
	doc.finalNewline = strings.HasSuffix(src, "\n")
	body := strings.TrimSuffix(src, "\n")
	if body == "" && !doc.finalNewline {
		doc.Lines = nil
	} else {
		doc.Lines = strings.Split(body, "\n")
	}

	start := skipFrontMatter(doc.Lines)
	var fence fenceState
	var items []Item
	for i := start; i < len(doc.Lines); i++ {
		line := doc.Lines[i]
		if fence.step(line) {
			continue
		}
		if fence.open {
			continue
		}
		title, ok := headingTitle(line)
		if !ok {
			continue
		}
		if n := len(items); n > 0 {
			items[n-1].EndLine = i - 1
		}
		clean, done := splitDone(title)
		items = append(items, Item{
			Title:       clean,
			Done:        done,
			HeadingLine: i,
			EndLine:     len(doc.Lines) - 1,
		})
	}
	for idx := range items {
		items[idx].Body = joinBody(doc.Lines, items[idx].HeadingLine+1, items[idx].EndLine)
	}
	doc.Items = items
	return doc
}

// headingTitle reports whether line is a top-level ATX heading and returns its
// title. Up to three leading spaces are allowed, as in CommonMark; a tab or a
// fourth space makes it an indented code block instead.
func headingTitle(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return "", false
	}
	rest := line[indent:]
	if !strings.HasPrefix(rest, "#") {
		return "", false
	}
	rest = rest[1:]
	if strings.HasPrefix(rest, "#") {
		return "", false // ## and deeper are structure inside an entry
	}
	if rest == "" {
		return "", false // a bare "#" is not an entry
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return "", false // "#hashtag" is not a heading
	}
	title := strings.TrimSpace(rest)
	// A closing sequence of hashes is decoration, not part of the title.
	title = strings.TrimRight(title, "#")
	title = strings.TrimSpace(title)
	if title == "" {
		return "", false
	}
	return title, true
}

// splitDone separates the done marker from a title.
func splitDone(title string) (string, bool) {
	rest, ok := strings.CutPrefix(title, DoneMarker)
	if !ok {
		return title, false
	}
	trimmed := strings.TrimLeft(rest, " \t")
	if trimmed == "" {
		return "", true
	}
	return trimmed, true
}

// MarkTitle renders a title with or without the done marker.
func MarkTitle(title string, done bool) string {
	if done {
		return "# " + DoneMarker + " " + title
	}
	return "# " + title
}

// joinBody returns lines[from..to] with surrounding blank lines removed.
func joinBody(lines []string, from, to int) string {
	if from > to || from >= len(lines) {
		return ""
	}
	if to >= len(lines) {
		to = len(lines) - 1
	}
	for from <= to && strings.TrimSpace(lines[from]) == "" {
		from++
	}
	for to >= from && strings.TrimSpace(lines[to]) == "" {
		to--
	}
	if from > to {
		return ""
	}
	return strings.Join(lines[from:to+1], "\n")
}

// skipFrontMatter returns the index of the first line after a YAML front
// matter block, or 0 when there is none.
func skipFrontMatter(lines []string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if t := strings.TrimSpace(lines[i]); t == "---" || t == "..." {
			return i + 1
		}
	}
	return 0 // unterminated: treat the whole file as content
}

// fenceState tracks fenced code blocks so that a "# comment" inside a shell
// snippet is not mistaken for an entry.
type fenceState struct {
	open   bool
	char   byte
	length int
}

// step consumes a line and reports whether it was a fence delimiter.
func (f *fenceState) step(line string) bool {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	rest := line[indent:]
	if len(rest) < 3 {
		return false
	}
	ch := rest[0]
	if ch != '`' && ch != '~' {
		return false
	}
	n := 0
	for n < len(rest) && rest[n] == ch {
		n++
	}
	if n < 3 {
		return false
	}
	info := strings.TrimSpace(rest[n:])
	if !f.open {
		// An opening backtick fence may not carry a backtick in its info
		// string.
		if ch == '`' && strings.Contains(info, "`") {
			return false
		}
		f.open, f.char, f.length = true, ch, n
		return true
	}
	// Closing fences must match the opening character, be at least as long,
	// and carry no info string.
	if ch != f.char || n < f.length || info != "" {
		return false
	}
	f.open, f.char, f.length = false, 0, 0
	return true
}

// FirstOpen returns the index of the first item that is not done, or -1.
func (d *Doc) FirstOpen() int {
	for i, it := range d.Items {
		if !it.Done {
			return i
		}
	}
	return -1
}

// render reassembles the document, preserving the original line endings.
func (d *Doc) render() string {
	out := strings.Join(d.Lines, "\n")
	if d.finalNewline && out != "" {
		out += "\n"
	}
	if d.crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out
}

// List finds note files below the given roots.
//
// A root that does not exist is not an error: it is reported through the
// returned Source so the UI can say so instead of failing.
type Source struct {
	Name      string
	Path      string
	Available bool
	Err       error
	Files     []string
}

// Scan collects Markdown files under root. Names in exclude are matched
// against each path element, so "attachments" hides that whole directory.
func Scan(name, root string, recursive bool, exclude []string) Source {
	src := Source{Name: name, Path: root}
	info, err := os.Stat(root)
	if err != nil {
		src.Err = err
		return src
	}
	if !info.IsDir() {
		src.Err = errNotDir
		return src
	}
	src.Available = true

	skip := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		skip[strings.Trim(e, "/")] = true
	}

	walk := filepath.WalkDir
	err = walk(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corners are skipped, not fatal
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if skip[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			if !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if skip[rel] {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			src.Files = append(src.Files, path)
		}
		return nil
	})
	if err != nil {
		src.Err = err
	}
	sort.Strings(src.Files)
	return src
}
