package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingFileYieldsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("a missing config must not be an error: %v", err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Path != "~/notes" {
		t.Fatalf("sources = %+v, want the default", cfg.Sources)
	}
	if !cfg.Behavior.CloseOnSend || !cfg.Behavior.Watch {
		t.Fatalf("behavior = %+v, want the defaults", cfg.Behavior)
	}
	if cfg.Path != "" {
		t.Fatalf("Path = %q, want empty for defaults", cfg.Path)
	}
}

func TestLoadOverridesAndKeepsUnsetDefaults(t *testing.T) {
	path := write(t, `
[[sources]]
name = "Prompts"
path = "/tmp/x"
recursive = false

[behavior]
mark_done = "none"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Name != "Prompts" {
		t.Fatalf("sources = %+v", cfg.Sources)
	}
	if cfg.Sources[0].Recursive {
		t.Fatal("recursive = true, want the value from the file")
	}
	if cfg.Behavior.MarkDone != MarkDoneNone {
		t.Fatalf("mark_done = %q", cfg.Behavior.MarkDone)
	}
	// Unset keys keep their defaults.
	if !cfg.Behavior.CloseOnSend {
		t.Fatal("close_on_send should still default to true")
	}
	if cfg.Window.Width != "80%" {
		t.Fatalf("window.width = %q, want the default", cfg.Window.Width)
	}
	if cfg.Path != path {
		t.Fatalf("Path = %q, want %q", cfg.Path, path)
	}
}

func TestExplicitFalseBeatsDefaultTrue(t *testing.T) {
	// The whole point of the raw-key check: a deliberate false must not be
	// mistaken for "unset".
	path := write(t, "[behavior]\nclose_on_send = false\nwatch = false\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Behavior.CloseOnSend {
		t.Fatal("close_on_send = true, want false from the file")
	}
	if cfg.Behavior.Watch {
		t.Fatal("watch = true, want false from the file")
	}
}

func TestInvalidTOMLIsReported(t *testing.T) {
	path := write(t, "this is not = = toml")
	if _, err := Load(path); err == nil {
		t.Fatal("want an error for malformed TOML")
	}
}

func TestUnknownMarkDoneIsRejected(t *testing.T) {
	path := write(t, "[behavior]\nmark_done = \"sometimes\"\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("want an error for an unknown mark_done value")
	}
	if !strings.Contains(err.Error(), "mark_done") {
		t.Fatalf("error should name the offending key: %v", err)
	}
}

func TestEmptySourcePathIsRejected(t *testing.T) {
	path := write(t, "[[sources]]\nname = \"x\"\npath = \"  \"\n")
	if _, err := Load(path); err == nil {
		t.Fatal("want an error for an empty source path")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	t.Setenv("NOTEPAD_TEST_DIR", "expanded")

	cases := map[string]string{
		"~":                     home,
		"~/notes":               filepath.Join(home, "notes"),
		"~/a b/c":               filepath.Join(home, "a b", "c"),
		"/absolute/path":        "/absolute/path",
		"$NOTEPAD_TEST_DIR/sub": "expanded/sub",
		"":                      "",
		// A tilde in the middle is a literal character, not a home
		// directory.
		"/tmp/~backup": "/tmp/~backup",
		"~notauser":    "~notauser",
	}
	for in, want := range cases {
		if got := ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvedSourcesFillsName(t *testing.T) {
	cfg := Config{Sources: []Source{{Path: "/tmp/some dir"}}}
	got := cfg.ResolvedSources()
	if len(got) != 1 || got[0].Name != "some dir" {
		t.Fatalf("resolved = %+v, want the name derived from the path", got)
	}
}

func TestEditorPrecedence(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	cfg := Default()
	if got := cfg.Editor(); got != "vi" {
		t.Fatalf("Editor() = %q, want the vi fallback", got)
	}

	t.Setenv("EDITOR", "nano")
	if got := cfg.Editor(); got != "nano" {
		t.Fatalf("Editor() = %q, want $EDITOR", got)
	}

	t.Setenv("VISUAL", "kate")
	if got := cfg.Editor(); got != "kate" {
		t.Fatalf("Editor() = %q, want $VISUAL to win over $EDITOR", got)
	}

	cfg.Behavior.Editor = "hx"
	if got := cfg.Editor(); got != "hx" {
		t.Fatalf("Editor() = %q, want the configured editor to win", got)
	}
}

func TestDefaultPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, want := DefaultPath(), filepath.Join("/xdg", "tmux-notepad", "config.toml"); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestHasKeyIgnoresComments(t *testing.T) {
	raw := "# watch = false\n[behavior]\nmark_done = \"none\"\n"
	if hasKey(raw, "watch") {
		t.Fatal("a commented-out key must not count as set")
	}
	if !hasKey(raw, "mark_done") {
		t.Fatal("mark_done should be found")
	}
}
