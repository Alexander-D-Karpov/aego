//go:build aego_golden

package golden

import (
	"testing"

	"aego/core/mathx"
	"aego/ui/dock"
)

func TestDockTabsGolden(t *testing.T) {
	env := newEditorEnv(t)
	tr := dock.Columns([]string{"hierarchy"}, nil, []string{"scene", "move", "move2"}, []string{"console"}, []string{"inspector"})
	titles := map[string]string{"hierarchy": "Hierarchy", "scene": "main", "move": "move — player", "move2": "move — enemy", "console": "Console", "inspector": "Inspector"}
	icons := map[string]string{"hierarchy": "panel.hierarchy", "scene": "file.scene", "move": "file.script", "move2": "file.script", "console": "panel.console", "inspector": "panel.inspector"}
	tr.Title = func(p string) string { return titles[p] }
	tr.Icon = func(p string) string { return icons[p] }
	tr.Dirty = func(p string) bool { return p == "scene" || p == "move2" }
	img, err := env.draw(func(full mathx.Rect) {
		tr.Draw(env.ui, full, func(p string, r mathx.Rect) {})
	})
	checkGolden(t, "editor_dock_tabs", img, err)
}
