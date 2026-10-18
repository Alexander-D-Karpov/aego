//go:build aego_golden

package golden

import (
	"testing"

	"aego/ui/draw"
)

func TestEditorDrawsBalancedClips(t *testing.T) {
	s, _ := playSession(t, 1400, 900, cameraScene)
	ws := s.App.Workspace()
	check := func(what string) {
		for range 2 {
			s.Step()
		}
		if d := s.App.UIList().ClipDepth(); d != [draw.Layers]int{} {
			t.Errorf("%s leaves clips open %v", what, d)
		}
	}
	for _, pg := range []string{"project", "engine", "render2d", "input", "physics", "debug", "defaults/display", "defaults/audio", "defaults/accessibility", "registry"} {
		s.App.Context().ShowSettings(pg)
		check("settings page " + pg)
	}
	for _, p := range ws.Panels() {
		ws.Show(p.ID())
		check("panel " + p.ID())
	}
}
