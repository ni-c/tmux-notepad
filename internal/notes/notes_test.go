package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func titles(d *Doc) []string {
	out := make([]string, 0, len(d.Items))
	for _, it := range d.Items {
		out = append(out, it.Title)
	}
	return out
}

func TestParseEntries(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		titles []string
	}{
		{"empty", "", nil},
		{"only blank lines", "\n\n\n", nil},
		{"no heading at all", "just a note\nwith two lines\n", nil},
		{"single entry", "# One\nbody\n", []string{"One"}},
		{"two entries", "# One\na\n\n# Two\nb\n", []string{"One", "Two"}},
		{"heading without body", "# One\n", []string{"One"}},
		{"leading blank lines", "\n\n# One\nbody\n", []string{"One"}},
		{"no trailing newline", "# One\nbody", []string{"One"}},
		{"h2 is not an entry", "# One\n## Sub\ntext\n# Two\n", []string{"One", "Two"}},
		{"hashtag is not a heading", "#tag not a heading\n# Real\n", []string{"Real"}},
		{"bare hash is not a heading", "#\n# Real\n", []string{"Real"}},
		{"hash with only spaces", "#   \n# Real\n", []string{"Real"}},
		{"three spaces still a heading", "   # One\n", []string{"One"}},
		{"four spaces is code", "    # One\n# Two\n", []string{"Two"}},
		{"closing hashes trimmed", "# One ###\n", []string{"One"}},
		{"umlauts and emoji", "# Größe 🌲 prüfen\n", []string{"Größe 🌲 prüfen"}},
		{"crlf", "# One\r\nbody\r\n# Two\r\n", []string{"One", "Two"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := titles(Parse(tc.src))
			if len(got) != len(tc.titles) {
				t.Fatalf("got %q, want %q", got, tc.titles)
			}
			for i := range got {
				if got[i] != tc.titles[i] {
					t.Fatalf("got %q, want %q", got, tc.titles)
				}
			}
		})
	}
}

func TestParseSkipsCodeFences(t *testing.T) {
	src := "# Real\n" +
		"```sh\n" +
		"# this is a shell comment, not an entry\n" +
		"make test\n" +
		"```\n" +
		"~~~\n" +
		"# neither is this\n" +
		"~~~\n" +
		"# Also real\n"
	got := titles(Parse(src))
	want := []string{"Real", "Also real"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseUnterminatedFence(t *testing.T) {
	// An unterminated fence swallows the rest of the file; that is what a
	// Markdown renderer does too.
	src := "# Real\n```\n# hidden\n"
	if got := titles(Parse(src)); len(got) != 1 || got[0] != "Real" {
		t.Fatalf("got %q, want [Real]", got)
	}
}

func TestParseFenceNeedsMatchingClose(t *testing.T) {
	// A shorter run of backticks does not close a longer fence.
	src := "# Real\n````\n# hidden\n```\n# still hidden\n````\n# Back\n"
	got := titles(Parse(src))
	want := []string{"Real", "Back"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseRejectsATitleItCouldNotWriteBack(t *testing.T) {
	// "# # #" is a heading whose closing sequence leaves "#" as the title. That
	// title cannot be rendered again: MarkTitle would produce "# #", where the
	// trailing hash reads as a closing sequence once more and nothing is left.
	// Accepting it would mean the entry disappears the first time it is ticked
	// off and back on. Found by FuzzParse.
	for _, src := range []string{
		"# # #\n",     // title would be "#", which renders back to nothing
		"#   #   #\n", // the same with more spaces
		"# ## #\n",
		"# 0 # #\n", // title would be "0 #", which renders back to "0"
		"# a #  #\n",
		"# ✓#\n", // done marker off leaves "#", which renders back to ""
	} {
		if got := titles(Parse(src)); len(got) != 0 {
			t.Fatalf("Parse(%q) gave %q, want no entries", src, got)
		}
	}

	// The neighbouring cases still parse. A closing sequence only counts where
	// a space sets it off, so a title that ends in a hash keeps it: "# C#" used
	// to parse as "C", and the entry was renamed on the next write-back.
	for _, tc := range []struct{ src, want string }{
		{"# C#\n", "C#"},
		{"# C# notes #\n", "C# notes"},
		{"# #tag here\n", "#tag here"},
		{"# Real #\n", "Real"},
		{"# Real\n", "Real"},
		{"# a # b #\n", "a # b"},
	} {
		got := titles(Parse(tc.src))
		if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("Parse(%q) gave %q, want [%q]", tc.src, got, tc.want)
		}
	}
}

func TestParseFrontMatter(t *testing.T) {
	src := "---\ntitle: notes\ntags: [a]\n---\n# One\nbody\n"
	if got := titles(Parse(src)); len(got) != 1 || got[0] != "One" {
		t.Fatalf("got %q, want [One]", got)
	}

	// A heading inside front matter must not become an entry.
	src = "---\n# not an entry\n---\n# One\n"
	if got := titles(Parse(src)); len(got) != 1 || got[0] != "One" {
		t.Fatalf("got %q, want [One]", got)
	}

	// Unterminated front matter: fall back to treating everything as body.
	src = "---\n# One\n"
	if got := titles(Parse(src)); len(got) != 1 || got[0] != "One" {
		t.Fatalf("unterminated front matter: got %q, want [One]", got)
	}
}

func TestBodyBoundaries(t *testing.T) {
	src := "# One\n\n  first\n\nsecond\n\n\n# Two\nbody two\n"
	doc := Parse(src)
	if len(doc.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(doc.Items))
	}
	if want := "  first\n\nsecond"; doc.Items[0].Body != want {
		t.Fatalf("body = %q, want %q", doc.Items[0].Body, want)
	}
	if want := "body two"; doc.Items[1].Body != want {
		t.Fatalf("body = %q, want %q", doc.Items[1].Body, want)
	}
}

func TestPayloadFallsBackToTitle(t *testing.T) {
	doc := Parse("# make test\n\n# With body\ntext\n")
	if got := doc.Items[0].Payload(); got != "make test" {
		t.Fatalf("payload = %q, want %q", got, "make test")
	}
	if got := doc.Items[1].Payload(); got != "text" {
		t.Fatalf("payload = %q, want %q", got, "text")
	}
}

func TestDoneMarker(t *testing.T) {
	doc := Parse("# ✓ Sent\nbody\n# Open\n# ✓\n")
	if len(doc.Items) != 3 {
		t.Fatalf("want 3 items, got %d: %q", len(doc.Items), titles(doc))
	}
	if !doc.Items[0].Done || doc.Items[0].Title != "Sent" {
		t.Fatalf("item 0 = %+v, want done with title Sent", doc.Items[0])
	}
	if doc.Items[1].Done {
		t.Fatalf("item 1 should be open")
	}
	if !doc.Items[2].Done || doc.Items[2].Title != "" {
		t.Fatalf("item 2 = %+v, want done with empty title", doc.Items[2])
	}
	if got := doc.FirstOpen(); got != 1 {
		t.Fatalf("FirstOpen = %d, want 1", got)
	}
}

func TestFirstOpenWhenAllDone(t *testing.T) {
	doc := Parse("# ✓ a\n# ✓ b\n")
	if got := doc.FirstOpen(); got != -1 {
		t.Fatalf("FirstOpen = %d, want -1", got)
	}
}

func TestMarkTitleRoundTrip(t *testing.T) {
	for _, title := range []string{"Release notes", "Größe 🌲", "a # b"} {
		line := MarkTitle(title, true)
		got, ok := headingTitle(line)
		if !ok {
			t.Fatalf("%q is not a heading", line)
		}
		clean, done := splitDone(got)
		if !done || clean != title {
			t.Fatalf("round trip of %q gave (%q, %v)", title, clean, done)
		}
	}
}

// writeNote creates a note file and returns its path.
func writeNote(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMarkDoneRewritesOnlyTheHeading(t *testing.T) {
	src := "# One\n  indented body  \n\ttab line\n\n# Two\nbody\n"
	path := writeNote(t, src)
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.MarkDone(0, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# ✓ One\n  indented body  \n\ttab line\n\n# Two\nbody\n"
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if !doc.Items[0].Done {
		t.Fatal("in-memory item not updated")
	}

	// And back again.
	if err := doc.MarkDone(0, false); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != src {
		t.Fatalf("after undo file =\n%q\nwant\n%q", got, src)
	}
}

func TestMarkDoneIsIdempotent(t *testing.T) {
	path := writeNote(t, "# ✓ One\nbody\n")
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.MarkDone(0, true); err != nil {
		t.Fatalf("marking an already-done item should be a no-op: %v", err)
	}
}

func TestMarkDoneOutOfRange(t *testing.T) {
	path := writeNote(t, "# One\n")
	doc, _ := Load(path)
	if err := doc.MarkDone(5, true); err == nil {
		t.Fatal("want an error for an out-of-range index")
	}
	if err := doc.MarkDone(-1, true); err == nil {
		t.Fatal("want an error for a negative index")
	}
}

func TestMarkDonePreservesCRLF(t *testing.T) {
	path := writeNote(t, "# One\r\nbody\r\n")
	doc, _ := Load(path)
	if err := doc.MarkDone(0, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := "# ✓ One\r\nbody\r\n"; string(got) != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestMarkDonePreservesMissingFinalNewline(t *testing.T) {
	path := writeNote(t, "# One\nbody")
	doc, _ := Load(path)
	if err := doc.MarkDone(0, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := "# ✓ One\nbody"; string(got) != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestConcurrentChangeIsRefused(t *testing.T) {
	path := writeNote(t, "# One\nbody\n")
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Someone edits the note in another app while the popup is open.
	other := "# One\nbody edited elsewhere\n"
	if err := os.WriteFile(path, []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Now().Add(time.Second), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	err = doc.MarkDone(0, true)
	if err != ErrConflict {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != other {
		t.Fatalf("the other edit was clobbered: %q", got)
	}
}

func TestConcurrentChangeSameSizeIsRefused(t *testing.T) {
	// A same-length edit must still be caught, via the timestamp.
	path := writeNote(t, "# One\nbody\n")
	doc, _ := Load(path)
	if err := os.WriteFile(path, []byte("# One\nBODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if err := doc.MarkDone(0, true); err != ErrConflict {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestAppend(t *testing.T) {
	path := writeNote(t, "# One\nbody\n")
	doc, _ := Load(path)
	if err := doc.Append("Two", "new body\nsecond line"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "# One\nbody\n\n# Two\nnew body\nsecond line\n"
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if len(doc.Items) != 2 || doc.Items[1].Title != "Two" {
		t.Fatalf("items not reparsed: %q", titles(doc))
	}
}

func TestAppendToEmptyFile(t *testing.T) {
	path := writeNote(t, "")
	doc, _ := Load(path)
	if err := doc.Append("First", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := "# First\n"; string(got) != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestAppendRejectsEmptyTitle(t *testing.T) {
	path := writeNote(t, "# One\n")
	doc, _ := Load(path)
	if err := doc.Append("   ", "body"); err == nil {
		t.Fatal("want an error for an empty title")
	}
}

func TestDelete(t *testing.T) {
	path := writeNote(t, "# One\nbody one\n\n# Two\nbody two\n\n# Three\nbody three\n")
	doc, _ := Load(path)
	if err := doc.Delete(1); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "# One\nbody one\n\n# Three\nbody three\n"
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestDeleteLastAndOnly(t *testing.T) {
	path := writeNote(t, "# Only\nbody\n")
	doc, _ := Load(path)
	if err := doc.Delete(0); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if len(strings.TrimSpace(string(got))) != 0 {
		t.Fatalf("file should be empty, got %q", got)
	}
	if len(doc.Items) != 0 {
		t.Fatalf("items = %q, want none", titles(doc))
	}
}

func TestDeletePreservesPreamble(t *testing.T) {
	// Text before the first heading is not part of any entry and must stay.
	path := writeNote(t, "intro line\n\n# One\nbody\n")
	doc, _ := Load(path)
	if err := doc.Delete(0); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := "intro line\n\n"; string(got) != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.md")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

func TestCreateRefusesExisting(t *testing.T) {
	path := writeNote(t, "# One\n")
	if err := Create(path); err == nil {
		t.Fatal("want an error when the note already exists")
	}
	fresh := filepath.Join(filepath.Dir(path), "fresh.md")
	if err := Create(fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		t.Helper()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("a.md")
	mk("b.MD")
	mk("not-a-note.txt")
	mk("sub/c.md")
	mk("attachments/skip.md")
	mk(".hidden/skip.md")

	src := Scan("Notes", root, true, []string{"attachments"})
	if !src.Available || src.Err != nil {
		t.Fatalf("source not available: %v", src.Err)
	}
	var names []string
	for _, f := range src.Files {
		rel, _ := filepath.Rel(root, f)
		names = append(names, filepath.ToSlash(rel))
	}
	want := []string{"a.md", "b.MD", "sub/c.md"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("files = %q, want %q", names, want)
	}

	shallow := Scan("Notes", root, false, nil)
	for _, f := range shallow.Files {
		if strings.Contains(filepath.ToSlash(f), "/sub/") {
			t.Fatalf("non-recursive scan descended into sub: %q", shallow.Files)
		}
	}
}

func TestScanMissingRootIsNotFatal(t *testing.T) {
	src := Scan("Gone", filepath.Join(t.TempDir(), "nope"), true, nil)
	if src.Available {
		t.Fatal("a missing root must not be reported as available")
	}
	if src.Err == nil {
		t.Fatal("want an error describing why the source is unavailable")
	}
	if len(src.Files) != 0 {
		t.Fatalf("want no files, got %q", src.Files)
	}
}

func TestScanFileAsRoot(t *testing.T) {
	path := writeNote(t, "# One\n")
	src := Scan("File", path, true, nil)
	if src.Available || src.Err == nil {
		t.Fatal("a file as root must be reported as unavailable")
	}
}
