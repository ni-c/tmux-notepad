package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSizes(t *testing.T) {
	cases := []struct {
		name             string
		w, h, x, y       string
		clientW, clientH int
		wantW, wantH     int
		wantX, wantY     int
	}{
		{"percent centred", "80%", "50%", "C", "C", 100, 40, 80, 20, 10, 10},
		{"cells", "30", "10", "5", "2", 100, 40, 30, 10, 5, 2},
		{"empty falls back", "", "", "", "", 100, 40, 80, 32, 10, 4},
		{"percent position", "50%", "50%", "10%", "10%", 100, 40, 50, 20, 10, 4},
		{"unknown keyword centres", "50%", "50%", "P", "W", 100, 40, 50, 20, 25, 10},
		{"garbage size falls back", "abc", "-5", "C", "C", 100, 40, 80, 32, 10, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.w, tc.h, tc.x, tc.y, tc.clientW, tc.clientH)
			if got.W != tc.wantW || got.H != tc.wantH || got.X != tc.wantX || got.Y != tc.wantY {
				t.Fatalf("got %+v, want {X:%d Y:%d W:%d H:%d}", got, tc.wantX, tc.wantY, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestResolveNeverLeavesTheScreen(t *testing.T) {
	// A popup asked to be bigger than the client must be cut down, not
	// pushed off the edge.
	got := Resolve("200%", "200%", "C", "C", 80, 24)
	if got.W != 80 || got.H != 24 || got.X != 0 || got.Y != 0 {
		t.Fatalf("got %+v, want the full client", got)
	}
}

func TestClampKeepsMinimumSize(t *testing.T) {
	got := Layout{X: 0, Y: 0, W: 1, H: 1}.Clamp(100, 40)
	if got.W != MinWidth || got.H != MinHeight {
		t.Fatalf("got %+v, want at least %dx%d", got, MinWidth, MinHeight)
	}
}

func TestClampOnATinyClient(t *testing.T) {
	// The client may be smaller than the minimum; the popup must still fit
	// rather than hang off the side.
	got := Layout{X: 5, Y: 5, W: 40, H: 20}.Clamp(10, 4)
	if got.W > 10 || got.H > 4 || got.X != 0 || got.Y != 0 {
		t.Fatalf("got %+v, want it to fit inside 10x4", got)
	}
}

func TestMoveStopsAtTheEdges(t *testing.T) {
	l := Layout{X: 10, Y: 5, W: 40, H: 20}

	if got := l.Move(-100, -100, 100, 40); got.X != 0 || got.Y != 0 {
		t.Fatalf("moving far up-left gave %+v, want 0,0", got)
	}
	if got := l.Move(100, 100, 100, 40); got.X != 60 || got.Y != 20 {
		t.Fatalf("moving far down-right gave %+v, want 60,20", got)
	}
	if got := l.Move(5, 3, 100, 40); got.X != 15 || got.Y != 8 {
		t.Fatalf("ordinary move gave %+v, want 15,8", got)
	}
	// Size must never change while moving.
	if got := l.Move(7, 7, 100, 40); got.W != l.W || got.H != l.H {
		t.Fatalf("move changed the size: %+v", got)
	}
}

func TestResizeBothDirections(t *testing.T) {
	l := Layout{X: 10, Y: 5, W: 40, H: 20}

	if got := l.Resize(10, 5, 100, 40); got.W != 50 || got.H != 25 {
		t.Fatalf("growing gave %+v", got)
	}
	if got := l.Resize(-10, -5, 100, 40); got.W != 30 || got.H != 15 {
		t.Fatalf("shrinking gave %+v", got)
	}
	// Shrinking past the minimum stops there.
	if got := l.Resize(-1000, -1000, 100, 40); got.W != MinWidth || got.H != MinHeight {
		t.Fatalf("over-shrinking gave %+v, want the minimum", got)
	}
	// Growing past the client edge pulls the window back in.
	got := l.Resize(1000, 1000, 100, 40)
	if got.W != 100 || got.H != 40 || got.X != 0 || got.Y != 0 {
		t.Fatalf("over-growing gave %+v, want the full client at 0,0", got)
	}
}

func TestValid(t *testing.T) {
	if (Layout{}).Valid() {
		t.Fatal("a zero layout must not count as resolved")
	}
	if !(Layout{W: MinWidth, H: MinHeight}).Valid() {
		t.Fatal("a minimum-size layout should be valid")
	}
}

func TestArgsConvertsYToTheBottomEdge(t *testing.T) {
	// Measured against tmux 3.7: -x is the popup's left edge, but -y is its
	// BOTTOM edge. Passing the top edge makes tmux clamp a tall popup to the
	// top of the screen, so the first steps downwards appear to do nothing.
	w, h, x, y := Layout{X: 1, Y: 2, W: 30, H: 10}.Args()
	if w != "30" || h != "10" || x != "1" {
		t.Fatalf("args = %q %q %q %q", w, h, x, y)
	}
	if y != "12" {
		t.Fatalf("y = %q, want %q (top edge 2 + height 10)", y, "12")
	}
}

func TestArgsAtTheTopOfTheScreen(t *testing.T) {
	// A popup flush against the top still has to report its bottom edge, or
	// tmux would place it differently than intended.
	_, _, _, y := Layout{X: 0, Y: 0, W: 30, H: 10}.Args()
	if y != "10" {
		t.Fatalf("y = %q, want %q", y, "10")
	}
}

func TestArgsAtTheBottomOfTheScreen(t *testing.T) {
	// Clamped against a 40-row client: top 30 + height 10 = 40, the last
	// usable row.
	l := Layout{X: 0, Y: 100, W: 30, H: 10}.Clamp(100, 40)
	_, _, _, y := l.Args()
	if y != "40" {
		t.Fatalf("y = %q, want %q", y, "40")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "session.json")
	want := State{
		NotePath:      "/notes/a.md",
		SelectedTitle: "Release notes",
		SelectedIndex: 3,
		Filter:        "re",
		Target:        "%12",
		Layout:        Layout{X: 1, Y: 2, W: 40, H: 20},
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("a missing state file must not be an error: %v", err)
	}
	if got != (State{}) {
		t.Fatalf("got %+v, want the zero state", got)
	}
}

func TestLoadCorruptFileIsEmpty(t *testing.T) {
	// A broken state file must never keep the popup from opening.
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != (State{}) {
		t.Fatalf("got %+v, want the zero state", got)
	}
}

func TestPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg-state")
	if got, want := Path(), filepath.Join("/xdg-state", "tmux-notepad", "session.json"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
