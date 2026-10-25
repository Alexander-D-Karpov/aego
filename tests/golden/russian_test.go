//go:build aego_golden

package golden

import (
	"image"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/editor/panels/problems"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

type goldenProblems []context.ScriptProblem

func (g goldenProblems) ScriptProblems(dst []context.ScriptProblem) ([]context.ScriptProblem, uint64) {
	return append(dst[:0], g...), 1
}

func (g goldenProblems) FirstError(string) (context.ScriptProblem, int, bool) {
	return context.ScriptProblem{}, 0, false
}

type goldenNav struct{}

func (goldenNav) Open(context.Location) {}

func shotProblems(t *testing.T, env *editorEnv) (*image.NRGBA, error) {
	t.Helper()
	p := problems.New(goldenProblems{
		{Package: "game", Path: "scripts/game/player.aes", Line: 12, Col: 5, Code: "AES2001", Error: true, Message: "unknown name speed"},
		{Package: "game", Path: "scripts/game/player.aes", Line: 30, Col: 9, Code: "AES3004", Message: "the value is never used"},
		{Category: context.ProblemScene, Path: "scenes/main.aescene", Code: "SCN010", Message: "Player has two cameras"},
	}, goldenNav{})
	return env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui}, full) })
}

func russian(t *testing.T) {
	t.Helper()
	i18n.Default().SetLanguage(i18n.Russian)
	widgets.Tr = i18n.Lookup
	t.Setenv("AEGO_LANG", i18n.Russian)
	t.Cleanup(func() { i18n.Default().SetLanguage(i18n.English) })
}

// The editor in Russian: the panels the author reviews first, drawn with the ru catalog (D348).
func TestEditorRussianGoldens(t *testing.T) {
	for _, tc := range []struct {
		name string
		shot func(t *testing.T) (*image.NRGBA, error)
	}{
		{"editor_inspector_refs", func(t *testing.T) (*image.NRGBA, error) { return shotInspector(t, newEditorEnv(t)) }},
		{"editor_hierarchy_prefab", func(t *testing.T) (*image.NRGBA, error) { return shotHierarchyPrefab(t, newEditorEnvSize(t, 320, 240)) }},
		{"editor_problems", func(t *testing.T) (*image.NRGBA, error) { return shotProblems(t, newEditorEnvSize(t, 640, 200)) }},
		{"editor_launcher", func(t *testing.T) (*image.NRGBA, error) { return shotLauncher(t, newEditorEnv(t)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			russian(t)
			img, err := tc.shot(t)
			checkGolden(t, tc.name+"_ru", img, err)
		})
	}
}

func TestPaletteRussianGolden(t *testing.T) {
	russian(t)
	s := paletteSession(t)
	ctx := s.App.Context()
	s.App.AddPalette().AddComponent(ctx.Selected(), nil)
	paletteShot(t, s, "editor_palette_recent_ru")
}

// A long tooltip wraps at tooltipMax instead of running off the window (D348).
func TestTooltipWrapRussian(t *testing.T) {
	russian(t)
	env := newEditorEnvSize(t, 520, 200)
	id := core.IDOf("golden.tip")
	img, err := env.draw(func(full mathx.Rect) {
		widgets.TooltipAfter(env.ui, id, i18n.Tr("editor.command.play.release_input.tip", "Return keyboard and mouse to the editor; the game keeps Escape")+". "+i18n.Tr("editor.shell.tip_click_a_problem_in", "Tip: click a problem in the Problems panel to jump to its line, F2 walks through errors"), 0)
	})
	checkGolden(t, "editor_tooltip_wrap_ru", img, err)
}
