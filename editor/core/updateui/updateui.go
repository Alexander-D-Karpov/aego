// Package updateui is what the launcher and the editor show of Aego's updates and ask of them (D362).
package updateui

import (
	"time"

	"aego/update"
)

type Phase uint8

const (
	Idle Phase = iota
	Checking
	UpToDate
	// Available is a newer version not downloaded yet: Ask mode, or a copy that cannot update itself.
	Available
	Downloading
	// Ready is a downloaded version that installs on restart.
	Ready
	Failed
)

// State is the update as the banner and the status bar show it.
type State struct {
	Phase   Phase
	Version string
	Notes   string
	Err     string
	Done    int64
	Total   int64
	// Installable is false for a copy that was not installed by an installer: it can only send the user to the download page.
	Installable bool
	// Hidden is set after Later, so the banner waits for the next session.
	Hidden bool
	// RolledBack names a version that crashed right after its update and was undone.
	RolledBack string
}

// Host is the update service of the running copy; every method returns at once, work runs in the background.
type Host interface {
	State() State
	// Tick checks at start and every six hours of a long session; it costs nothing between checks.
	Tick(now time.Time)
	Check()
	Download()
	// Install puts the downloaded version in place; relaunch is false when an installer restarts the editor itself after it quits.
	Install() (relaunch bool, err error)
	Skip()
	Later()
	Policy() update.Policy
	SetMode(m update.Mode)
	SetChannel(c update.Channel)
	CanRestore() bool
	// Restore goes back to the previous version; relaunch is false as for Install.
	Restore() (relaunch bool, err error)
	// Page is where a copy that cannot update itself gets the new version.
	Page() string
}
