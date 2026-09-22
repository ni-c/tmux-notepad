package notes

import "testing"

// FuzzParse throws arbitrary text at the note parser.
//
// Notes are written by whatever the user points the config at — an editor, a
// web UI, a sync that truncated a file mid-write — so the parser has to survive
// anything without panicking. Two promises are checked beyond that, and both
// exist because ticking an entry off rewrites the user's file in place:
//
//   - The indices have to be real. The UI slices Doc.Lines with them and
//     MarkDone rewrites the line at HeadingLine, so an index out of range here
//     would corrupt a note rather than merely crash.
//   - Rendering an item's heading back has to reproduce exactly what was parsed.
//     That is the round trip MarkDone performs, and a heading that does not
//     survive it would quietly change every time the entry is toggled.
func FuzzParse(f *testing.F) {
	f.Add("")
	f.Add("#")
	f.Add("# Title\n\nbody\n")
	f.Add("# ✓ Done\n")
	f.Add("# ✓ ✓ marker in the title\n")
	f.Add("```\n# not an entry\n```\n# real one\n")
	f.Add("---\ntitle: front matter\n---\n# after\n")
	f.Add("\r\n# CRLF\r\n\r\nbody\r\n")
	f.Add("# a\n## deeper\n# b\n")
	f.Add("~~~\n# tilde fence\n~~~\n")
	f.Add("#    spaced out\n")
	f.Add("# Größe 🌲\n")

	f.Fuzz(func(t *testing.T, src string) {
		doc := Parse(src)
		if doc == nil {
			t.Fatal("Parse returned nil")
		}

		prevEnd := -1
		for i, item := range doc.Items {
			if item.HeadingLine < 0 || item.HeadingLine >= len(doc.Lines) {
				t.Fatalf("item %d: HeadingLine %d outside 0..%d", i, item.HeadingLine, len(doc.Lines)-1)
			}
			if item.EndLine < 0 || item.EndLine >= len(doc.Lines) {
				t.Fatalf("item %d: EndLine %d outside 0..%d", i, item.EndLine, len(doc.Lines)-1)
			}
			if item.EndLine < item.HeadingLine {
				t.Fatalf("item %d: EndLine %d before HeadingLine %d", i, item.EndLine, item.HeadingLine)
			}
			if item.HeadingLine <= prevEnd {
				t.Fatalf("item %d starts at %d, inside the previous item ending at %d", i, item.HeadingLine, prevEnd)
			}
			prevEnd = item.EndLine

			// What MarkDone writes back, read again the way Parse read it.
			line := MarkTitle(item.Title, item.Done)
			heading, ok := headingTitle(line)
			if !ok {
				t.Fatalf("item %d: MarkTitle produced %q, which is not a heading", i, line)
			}
			title, done := splitDone(heading)
			if title != item.Title || done != item.Done {
				t.Fatalf("item %d: %q/%v rendered to %q and came back as %q/%v",
					i, item.Title, item.Done, line, title, done)
			}
		}
	})
}
