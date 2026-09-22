// Package config loads the user's settings.
//
// Nothing here carries a personal default: the shipped example configuration
// points at ~/notes, and the real locations live only in the user's own file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Source is one directory that is searched for notes.
type Source struct {
	Name      string   `toml:"name"`
	Path      string   `toml:"path"`
	Recursive bool     `toml:"recursive"`
	Exclude   []string `toml:"exclude"`
}

// Behavior holds the settings that change what happens on a key press.
type Behavior struct {
	// MarkDone is "checkmark" to tick off an entry in the note after
	// pasting it, or "none" to leave the file alone.
	MarkDone string `toml:"mark_done"`
	// IncludeTitle sends the heading along with the body.
	IncludeTitle bool `toml:"include_title"`
	// CloseOnSend closes the popup after pasting, so the target pane has
	// the focus and the user can press Enter.
	CloseOnSend bool `toml:"close_on_send"`
	// Watch reloads a note when it changes on disk.
	Watch bool `toml:"watch"`
	// Editor overrides $EDITOR for the "open in editor" key.
	Editor string `toml:"editor"`
}

// Window holds the popup geometry used when no remembered one exists.
type Window struct {
	Width  string `toml:"width"`
	Height string `toml:"height"`
	X      string `toml:"x"`
	Y      string `toml:"y"`
}

// UI holds presentation settings.
type UI struct {
	// Theme is "auto", "dark", "light" or "notty".
	Theme string `toml:"theme"`
}

// Config is the whole file.
type Config struct {
	Sources  []Source `toml:"sources"`
	Behavior Behavior `toml:"behavior"`
	Window   Window   `toml:"window"`
	UI       UI       `toml:"ui"`

	// Path is where this configuration was read from; empty when defaults
	// were used.
	Path string `toml:"-"`
}

// Valid values for Behavior.MarkDone.
const (
	MarkDoneCheckmark = "checkmark"
	MarkDoneNone      = "none"
)

// Default returns the configuration used when the user has no file yet.
func Default() Config {
	return Config{
		Sources: []Source{{
			Name:      "Notes",
			Path:      "~/notes",
			Recursive: true,
			Exclude:   []string{"attachments"},
		}},
		Behavior: Behavior{
			MarkDone:     MarkDoneCheckmark,
			IncludeTitle: false,
			CloseOnSend:  true,
			Watch:        true,
		},
		Window: Window{Width: "80%", Height: "80%", X: "C", Y: "C"},
		UI:     UI{Theme: "auto"},
	}
}

// DefaultPath returns the location the configuration is read from.
func DefaultPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "tmux-notepad", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "tmux-notepad", "config.toml")
}

// Load reads the configuration at path. A missing file is not an error: the
// defaults are returned, so a fresh install starts up and can say what it is
// missing.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	// Start from a zero value so that unset fields can be told apart from
	// deliberate falses, then fill the gaps from the defaults.
	var file Config
	if _, err := toml.Decode(string(raw), &file); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	merged := merge(cfg, file, string(raw))
	merged.Path = path
	if err := merged.validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return merged, nil
}

// merge fills unset fields of file with the defaults. Booleans are only taken
// from the file when the key actually appears in it, which is why the raw text
// is consulted.
func merge(def, file Config, raw string) Config {
	out := def
	if len(file.Sources) > 0 {
		out.Sources = file.Sources
	}
	if file.Behavior.MarkDone != "" {
		out.Behavior.MarkDone = file.Behavior.MarkDone
	}
	if file.Behavior.Editor != "" {
		out.Behavior.Editor = file.Behavior.Editor
	}
	if hasKey(raw, "include_title") {
		out.Behavior.IncludeTitle = file.Behavior.IncludeTitle
	}
	if hasKey(raw, "close_on_send") {
		out.Behavior.CloseOnSend = file.Behavior.CloseOnSend
	}
	if hasKey(raw, "watch") {
		out.Behavior.Watch = file.Behavior.Watch
	}
	if file.Window.Width != "" {
		out.Window.Width = file.Window.Width
	}
	if file.Window.Height != "" {
		out.Window.Height = file.Window.Height
	}
	if file.Window.X != "" {
		out.Window.X = file.Window.X
	}
	if file.Window.Y != "" {
		out.Window.Y = file.Window.Y
	}
	if file.UI.Theme != "" {
		out.UI.Theme = file.UI.Theme
	}
	return out
}

// hasKey reports whether a bare TOML key appears in the source text.
func hasKey(raw, key string) bool {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, _, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(name) == key {
			return true
		}
	}
	return false
}

func (c Config) validate() error {
	switch c.Behavior.MarkDone {
	case MarkDoneCheckmark, MarkDoneNone:
	default:
		return fmt.Errorf("behavior.mark_done: unknown value %q (want %q or %q)",
			c.Behavior.MarkDone, MarkDoneCheckmark, MarkDoneNone)
	}
	for i, s := range c.Sources {
		if strings.TrimSpace(s.Path) == "" {
			return fmt.Errorf("sources[%d]: path must not be empty", i)
		}
	}
	return nil
}

// ExpandPath resolves a leading ~ and any environment variables.
func ExpandPath(path string) string {
	path = os.ExpandEnv(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	return path
}

// ResolvedSources returns the sources with their paths expanded, and a display
// name filled in where the user left it out.
func (c Config) ResolvedSources() []Source {
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		s.Path = ExpandPath(s.Path)
		if s.Name == "" {
			s.Name = filepath.Base(s.Path)
		}
		out = append(out, s)
	}
	return out
}

// Editor returns the command to edit a note with.
func (c Config) Editor() string {
	for _, candidate := range []string{c.Behavior.Editor, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return "vi"
}
