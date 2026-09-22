// Command tmux-notepad queues prompts and commands in a tmux popup and pastes
// one entry at a time into the pane it was opened from.
//
// It never presses Enter: the text is placed in the target pane's input as a
// bracketed paste and submitted by the user. That is what makes the same queue
// usable for a coding agent and for a plain shell alike.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ni-c/tmux-notepad/internal/config"
	"github.com/ni-c/tmux-notepad/internal/state"
	"github.com/ni-c/tmux-notepad/internal/tmuxio"
	"github.com/ni-c/tmux-notepad/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var (
		target      = flag.String("target", "", "tmux pane id to paste into (default: the active pane)")
		configPath  = flag.String("config", "", "path to config.toml")
		statePath   = flag.String("state", "", "path to the session state file")
		restore     = flag.Bool("restore", false, "restore the remembered selection and geometry")
		showVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("tmux-notepad", version)
		return
	}

	if err := run(*target, *configPath, *statePath, *restore); err != nil {
		fmt.Fprintln(os.Stderr, "tmux-notepad:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `tmux-notepad %s — a prompt and text queue for tmux

Usage:
  tmux-notepad [flags]

Bind it in tmux.conf. display-popup is called directly: run-shell -b has no
client and cannot show a popup at all, and without -b it blocks the client for
as long as the popup is open. The pane to paste into is resolved by
tmux-notepad itself, so no format expansion is needed here:

  bind n display-popup -E -w 80%% -h 80%% -T ' notepad ' 'tmux-notepad'

Flags:
`, version)
	flag.PrintDefaults()
}

func run(target, configPath, statePath string, restore bool) error {
	if !tmuxio.Available() {
		return fmt.Errorf("not running inside tmux")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		// A broken configuration must not keep the popup shut; say so and
		// carry on with the defaults.
		fmt.Fprintln(os.Stderr, "tmux-notepad:", err)
	}

	if statePath == "" {
		statePath = state.Path()
	}
	st, err := state.Load(statePath)
	if err != nil {
		st = state.State{}
	}
	if !restore {
		// A fresh open keeps the remembered geometry and note, but starts
		// without a stale search term.
		st.Filter = ""
	}

	if target == "" {
		if restore && st.Target != "" {
			target = st.Target
		} else if pane, err := tmuxio.CurrentPane(); err == nil {
			target = pane
		}
	}

	model := ui.New(cfg, st, statePath, configPath, target, restore)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	if err != nil {
		return err
	}
	// Leave at once. When the popup is being replaced by one at a new
	// geometry, anything lingering here would keep the old window on
	// screen and race the new one.
	os.Exit(0)
	return nil
}
