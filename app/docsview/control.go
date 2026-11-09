//go:build aego_editor

package docsview

import (
	"fmt"
	"strings"

	"aego/core/mathx"
	"aego/platform/input"
)

// ControlState lets the native/browser contract harness replay bounded input
// at fixed frame boundaries. Ordinary documentation sessions leave it disabled.
type ControlState struct {
	Enabled   bool `json:"enabled"`
	Remaining int  `json:"remaining"`
	keys      []input.Scancode
	started   bool
}

const maxControlFrames = 600

func controlKey(name string) (input.Scancode, bool) {
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		return input.KeyA + input.Scancode(name[0]-'A'), true
	}
	switch name {
	case "Space":
		return input.KeySpace, true
	case "Left":
		return input.KeyLeft, true
	case "Right":
		return input.KeyRight, true
	case "Up":
		return input.KeyUp, true
	case "Down":
		return input.KeyDown, true
	}
	return input.KeyUnknown, false
}

func (a *App) releaseControlKeys() {
	if a.control.started {
		for _, key := range a.control.keys {
			a.e.Input().ApplyKeyUp(key, 0, 0)
		}
	}
	a.control.keys = nil
	a.control.started = false
}

func (a *App) cancelControlAdvance() {
	a.control.Remaining = 0
	a.releaseControlKeys()
}

func (a *App) setControl(on bool) error {
	if a.control.Remaining != 0 {
		return fmt.Errorf("a controlled input sequence is still running")
	}
	a.releaseControlKeys()
	a.control.Enabled = on
	return nil
}

func (a *App) advanceControl(frames int, names []string) error {
	if !a.control.Enabled || a.control.Remaining != 0 {
		return fmt.Errorf("frame control is disabled or busy")
	}
	if frames < 1 || frames > maxControlFrames || len(names) > 8 {
		return fmt.Errorf("controlled input exceeds its frame or key budget")
	}
	keys := make([]input.Scancode, 0, len(names))
	seen := make(map[input.Scancode]bool, len(names))
	for _, name := range names {
		key, ok := controlKey(strings.TrimSpace(name))
		if !ok || seen[key] {
			return fmt.Errorf("unknown or duplicate gameplay key %q", name)
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if !a.session.State().Running || a.session.State().Paused {
		return fmt.Errorf("the Try world must be playing before advancing it")
	}
	a.releaseControlKeys()
	a.control.keys, a.control.Remaining = keys, frames
	return nil
}

func (a *App) tickSession(dt float64) {
	if !a.control.Enabled {
		a.session.Tick(dt)
		return
	}
	if a.control.Remaining == 0 {
		return
	}
	if !a.control.started {
		for _, key := range a.control.keys {
			a.e.Input().ApplyKeyDown(key, 0, 0, false)
		}
		a.control.started = true
	}
	if len(a.control.keys) != 0 {
		a.session.Controller().SetCapture(true, mathx.Vec2{}, mathx.Vec2{X: float32(a.opt.Width), Y: float32(a.opt.Height)})
	}
	a.session.Tick(1.0 / 60.0)
	a.control.Remaining--
	if a.control.Remaining == 0 {
		a.releaseControlKeys()
	}
}
