package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrConflict reports that the file changed on disk since it was read.
	// The note may be open in another editor, so the write is refused and
	// the caller should reload.
	ErrConflict = errors.New("note changed on disk since it was read")

	errNotDir = errors.New("not a directory")
)

// MarkDone sets or clears the done marker on the item at index idx and writes
// the file back.
//
// Only the heading line is rewritten; every other byte of the file is left
// untouched, so hand-made formatting survives.
func (d *Doc) MarkDone(idx int, done bool) error {
	if idx < 0 || idx >= len(d.Items) {
		return fmt.Errorf("item %d out of range", idx)
	}
	item := d.Items[idx]
	if item.Done == done {
		return nil
	}
	d.Lines[item.HeadingLine] = MarkTitle(item.Title, done)
	d.Items[idx].Done = done
	return d.save()
}

// Append adds a new entry with the given title and body at the end of the file
// and writes it back.
func (d *Doc) Append(title, body string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be empty")
	}
	if len(d.Lines) > 0 && strings.TrimSpace(d.Lines[len(d.Lines)-1]) != "" {
		d.Lines = append(d.Lines, "")
	}
	d.Lines = append(d.Lines, MarkTitle(title, false))
	if body = strings.TrimRight(body, "\n"); body != "" {
		d.Lines = append(d.Lines, strings.Split(body, "\n")...)
	}
	d.finalNewline = true
	return d.save()
}

// Delete removes the item at index idx, including its body, and writes the
// file back.
func (d *Doc) Delete(idx int) error {
	if idx < 0 || idx >= len(d.Items) {
		return fmt.Errorf("item %d out of range", idx)
	}
	item := d.Items[idx]
	end := item.EndLine
	if end >= len(d.Lines) {
		end = len(d.Lines) - 1
	}
	rest := append([]string{}, d.Lines[end+1:]...)
	d.Lines = append(d.Lines[:item.HeadingLine], rest...)
	return d.save()
}

// save writes the document atomically, refusing to clobber a concurrent
// change.
func (d *Doc) save() error {
	if d.Path == "" {
		return errors.New("document has no path")
	}
	if err := d.checkUnchanged(); err != nil {
		return err
	}

	dir := filepath.Dir(d.Path)
	tmp, err := os.CreateTemp(dir, ".tmux-notepad-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeded

	if _, err := tmp.WriteString(d.render()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		// Sync is best effort: some network filesystems refuse it.
		_ = err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Keep the permissions the note already had.
	if info, err := os.Stat(d.Path); err == nil {
		_ = os.Chmod(tmpName, info.Mode().Perm())
	}
	if err := os.Rename(tmpName, d.Path); err != nil {
		return err
	}

	reparsed := Parse(d.render())
	d.Items = reparsed.Items
	d.Lines = reparsed.Lines
	if info, err := os.Stat(d.Path); err == nil {
		d.ModTime = info.ModTime()
		d.Size = info.Size()
	}
	return nil
}

// checkUnchanged compares the file on disk against what was read.
func (d *Doc) checkUnchanged() error {
	info, err := os.Stat(d.Path)
	if err != nil {
		return err
	}
	if info.Size() != d.Size || !info.ModTime().Equal(d.ModTime) {
		return ErrConflict
	}
	return nil
}

// Create writes a new, empty note file. It refuses to overwrite an existing
// one.
func Create(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
