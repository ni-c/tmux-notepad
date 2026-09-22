// Package state remembers what the popup was showing and where it sat.
//
// It exists because a tmux popup cannot move or resize itself: the window is
// closed and reopened at the new geometry, and everything the user had on
// screen has to survive that jump.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// State is what is carried across a reopen.
type State struct {
	// NotePath is the note that was open.
	NotePath string `json:"note_path,omitempty"`
	// SelectedTitle identifies the highlighted entry by title, which
	// survives edits better than an index.
	SelectedTitle string `json:"selected_title,omitempty"`
	// SelectedIndex is the fallback when the title no longer matches.
	SelectedIndex int `json:"selected_index,omitempty"`
	// Filter is the search term that was typed.
	Filter string `json:"filter,omitempty"`
	// Target is the pane entries get pasted into.
	Target string `json:"target,omitempty"`
	// Layout is the popup geometry in cells.
	Layout Layout `json:"layout"`
}

// Layout is a popup geometry in terminal cells.
type Layout struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Minimum usable popup size, in cells.
const (
	MinWidth  = 24
	MinHeight = 6
)

// Valid reports whether the layout has been resolved to real cell values.
func (l Layout) Valid() bool {
	return l.W >= MinWidth && l.H >= MinHeight
}

// Resolve converts a configured geometry — cells, a percentage such as "80%",
// or the keyword "C" for centred — into cell values for a client of the given
// size.
func Resolve(width, height, x, y string, clientW, clientH int) Layout {
	w := resolveSize(width, clientW, 80)
	h := resolveSize(height, clientH, 80)
	l := Layout{W: w, H: h}
	l.X = resolvePos(x, clientW, w)
	l.Y = resolvePos(y, clientH, h)
	return l.Clamp(clientW, clientH)
}

// resolveSize turns "80%", "40" or "" into a cell count.
func resolveSize(spec string, total, defaultPercent int) int {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return percentOf(total, defaultPercent)
	}
	if pct, ok := strings.CutSuffix(spec, "%"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(pct))
		if err != nil {
			return percentOf(total, defaultPercent)
		}
		return percentOf(total, n)
	}
	n, err := strconv.Atoi(spec)
	if err != nil || n <= 0 {
		return percentOf(total, defaultPercent)
	}
	return n
}

func percentOf(total, pct int) int {
	if pct < 0 {
		pct = 0
	}
	return int(math.Round(float64(total) * float64(pct) / 100))
}

// resolvePos turns "C", "20" or "" into a cell offset. Anything tmux
// understands but this does not — "P", "M", "W" and friends — is treated as
// centred, which is the safe reading for a first placement.
func resolvePos(spec string, total, size int) int {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "", "C", "c":
		return (total - size) / 2
	}
	if pct, ok := strings.CutSuffix(spec, "%"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(pct)); err == nil {
			return percentOf(total, n)
		}
		return (total - size) / 2
	}
	n, err := strconv.Atoi(spec)
	if err != nil {
		return (total - size) / 2
	}
	return n
}

// Clamp keeps a layout inside a client of the given size.
func (l Layout) Clamp(clientW, clientH int) Layout {
	if clientW > 0 {
		if l.W > clientW {
			l.W = clientW
		}
		if l.W < MinWidth {
			l.W = min(MinWidth, clientW)
		}
		if l.X > clientW-l.W {
			l.X = clientW - l.W
		}
	}
	if clientH > 0 {
		if l.H > clientH {
			l.H = clientH
		}
		if l.H < MinHeight {
			l.H = min(MinHeight, clientH)
		}
		if l.Y > clientH-l.H {
			l.Y = clientH - l.H
		}
	}
	if l.X < 0 {
		l.X = 0
	}
	if l.Y < 0 {
		l.Y = 0
	}
	return l
}

// Move shifts the layout by dx, dy and keeps it on screen.
func (l Layout) Move(dx, dy, clientW, clientH int) Layout {
	l.X += dx
	l.Y += dy
	return l.Clamp(clientW, clientH)
}

// Resize grows or shrinks the layout, keeping the top-left corner put.
func (l Layout) Resize(dw, dh, clientW, clientH int) Layout {
	l.W += dw
	l.H += dh
	return l.Clamp(clientW, clientH)
}

// Args renders the layout as tmux display-popup arguments.
//
// tmux is asymmetric here, which is measurable and easy to get wrong: -x is
// the LEFT edge of the popup, but -y is its BOTTOM edge. A -y smaller than the
// popup's height is clamped to the top of the screen, so passing the top edge
// makes a tall popup ignore the first several steps downwards and only start
// moving once y exceeds its own height.
func (l Layout) Args() (w, h, x, y string) {
	return strconv.Itoa(l.W), strconv.Itoa(l.H), strconv.Itoa(l.X), strconv.Itoa(l.Y + l.H)
}

// Path returns the file the state is stored in.
func Path() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "tmux-notepad-state.json"
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "tmux-notepad", "session.json")
}

// Load reads the remembered state. A missing file yields a zero State.
func Load(path string) (State, error) {
	if path == "" {
		path = Path()
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		// A corrupt state file must never keep the popup from opening.
		return State{}, nil
	}
	return s, nil
}

// Save writes the state atomically.
func Save(path string, s State) error {
	if path == "" {
		path = Path()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
