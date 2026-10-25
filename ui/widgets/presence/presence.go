// Package presence draws where someone else works: the agent's marker and its selection outline (11b5b).
package presence

import (
	"aego/core/mathx"
	"aego/ui/core"
	"aego/ui/widgets"
)

// Icon is the marker of the agent.
const Icon = "agent.presence"

// linger matches presence.Linger: the marker fades out over the last second of it.
const linger = 5.0

// Alpha is how strong a marker is: it pulses while someone is there and fades as the linger ends.
func Alpha(ui *core.Context, age float64) float32 {
	if age < 0 || age >= linger {
		return 0
	}
	a := 0.7 + 0.3*mathx.Sin(float32(ui.Time)*4)
	if left := float32(linger - age); left < 1 {
		a *= left
	}
	return a
}

// Marker draws the agent's avatar in r.
func Marker(ui *core.Context, r mathx.Rect, age float64) {
	a := Alpha(ui, age)
	if a <= 0 {
		return
	}
	col := ui.Theme.Agent.Presence
	col.A *= a
	widgets.IconAt(ui, r, Icon, col)
}

// Outline draws the second outline of the agent's selection, next to the user's own.
func Outline(ui *core.Context, r mathx.Rect) {
	ui.List().RectOutline(r, ui.Theme.Agent.Presence, 1)
}

// Light paints a changed row: the warn colour, strongest when it just changed.
func Light(ui *core.Context, r mathx.Rect, a float32) {
	if a <= 0 {
		return
	}
	col := ui.Theme.Warn
	col.A = 0.22 * a
	ui.RoundRect(r, ui.Theme.Radius.Button, col)
}
