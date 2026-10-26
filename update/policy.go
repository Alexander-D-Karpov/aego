package update

import (
	"slices"
	"time"
)

// Mode is how a copy updates: download in the background and install on restart, ask first, or only when asked.
type Mode string

const (
	Auto Mode = "auto"
	Ask  Mode = "ask"
	Off  Mode = "off"
)

// Every is how often a long session checks again.
const Every = 6 * time.Hour

// Policy is the user's choice, kept in prefs.aetext for the editor and in the player's settings for a game.
type Policy struct {
	Mode      Mode
	Channel   Channel
	Skipped   []string
	LastCheck time.Time
}

// Due reports whether a check is due: at start, then every six hours; Off never checks on its own.
func (p *Policy) Due(now time.Time, started bool) bool {
	if p.Mode == Off {
		return false
	}
	return !started || now.Sub(p.LastCheck) >= Every
}

// Wants reports whether the release is offered: newer than the build, not skipped, and reachable from it.
func (p *Policy) Wants(r *Release, b Build) bool {
	return r != nil && r.Newer(b) && !slices.Contains(p.Skipped, r.Version)
}

// Skip stops offering a version; the next one is offered again.
func (p *Policy) Skip(version string) {
	if !slices.Contains(p.Skipped, version) {
		p.Skipped = append(p.Skipped, version)
	}
}

// ParseMode reads a mode, Auto for anything it does not know.
func ParseMode(s string) Mode {
	switch Mode(s) {
	case Ask, Off:
		return Mode(s)
	}
	return Auto
}

// ParseChannel reads a channel, Stable for anything it does not know.
func ParseChannel(s string) Channel {
	if Channel(s) == Nightly {
		return Nightly
	}
	return Stable
}
