package config

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLoad throws arbitrary bytes at the config reader.
//
// config.toml is hand-edited, so a half-finished or mistyped file is the normal
// case rather than an attack. Every one of those has to come back as an error
// the popup can show, never as a panic and never as a Config with the defaults
// silently dropped — a zero Width or an empty MarkDone would reach the UI and
// break it far away from the cause.
func FuzzLoad(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("[behavior]\nmark_done = \"paste\"\n"))
	f.Add([]byte("[[sources]]\nname = \"Notes\"\npath = \"~/notes\"\nrecursive = true\n"))
	f.Add([]byte("[window]\nwidth = \"80%\"\n"))
	f.Add([]byte("[[sources]]\npath = \"\"\n"))
	f.Add([]byte("not toml at all"))
	f.Add([]byte("[behavior\n"))

	def := Default()

	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}

		cfg, err := Load(path)
		if err != nil {
			// A rejected file is a perfectly good outcome; it is reported to
			// the user. Nothing more to check.
			return
		}

		// Accepted: the parts the UI relies on must be filled in, either from
		// the file or from the defaults.
		if cfg.Behavior.MarkDone == "" {
			t.Fatalf("accepted config left Behavior.MarkDone empty (default is %q)", def.Behavior.MarkDone)
		}
		if cfg.Window.Width == "" || cfg.Window.Height == "" {
			t.Fatalf("accepted config left the window size empty: %+v", cfg.Window)
		}
		for i, s := range cfg.Sources {
			if s.Path == "" {
				t.Fatalf("source %d was accepted with an empty path", i)
			}
		}
	})
}
