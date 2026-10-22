package integration

import (
	"slices"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/modules/gameplay"
	"aego/platform/input"
)

func TestProfilerGameplayAndCamera(t *testing.T) {
	s, pc := rewindSession(t)
	for range 60 {
		pc.Tick(1.0 / 60)
	}
	for range 3 {
		s.Step()
	}
	ui := s.App.Context().UI
	prof := s.App.Profiler()
	full := mathx.Rect{W: 900, H: 4000}
	draw := func() {
		ui.Begin(&input.State{}, mathx.Vec2{X: full.W, Y: full.H}, 1)
		ui.Region(full)
		prof.Draw(&services.Frame{UI: ui}, full)
		ui.PopCursor()
		ui.End()
	}
	draw()
	for _, name := range []string{"Gameplay", "Camera"} {
		if !slices.Contains(prof.Sections(), name) {
			t.Fatalf("profiler sections %v lack %s", prof.Sections(), name)
		}
	}
	st := gameplay.Of(pc.World()).Stats()
	if st.Steps == 0 || st.Controllers == 0 || st.Casts == 0 {
		t.Fatalf("gameplay counters %+v", st)
	}
	if st.Controllers < st.Steps {
		t.Fatalf("%d controller steps over %d fixed steps, the player alone runs every step", st.Controllers, st.Steps)
	}
	draw()
}
