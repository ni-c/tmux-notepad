package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// reloadMsg asks the model to check whether the open note changed on disk.
type reloadMsg struct{}

// editorDoneMsg reports that the external editor finished.
type editorDoneMsg struct{ err error }

// watchInterval is how often the open note is checked for changes.
//
// This is polling rather than inotify on purpose: notes usually live on a
// network share, written to by a web app or another machine, and inotify does
// not see writes made by another machine. A stat every two seconds is cheap
// and catches every case.
const watchInterval = 2 * time.Second

// watchTick schedules the next check.
func watchTick() tea.Cmd {
	return tea.Tick(watchInterval, func(time.Time) tea.Msg {
		return reloadMsg{}
	})
}
