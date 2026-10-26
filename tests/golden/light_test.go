//go:build aego_golden

package golden

import (
	"image"
	"testing"

	"aego/app/docstool"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/ui/dnd"
	"aego/ui/theme"
)

// The light theme draws a representative set of the dark goldens with the same functions (D345).
func TestEditorLightGoldens(t *testing.T) {
	light := func(t *testing.T, w, h int) *editorEnv { return newEditorEnvTheme(t, w, h, theme.Light()) }
	for _, tc := range []struct {
		name string
		shot func(t *testing.T) (*image.NRGBA, error)
	}{
		{"editor_menu_popup", func(t *testing.T) (*image.NRGBA, error) { return shotMenu(t, light(t, editorW, editorH)) }},
		{"editor_inspector_refs", func(t *testing.T) (*image.NRGBA, error) { return shotInspector(t, light(t, editorW, editorH)) }},
		{"editor_hierarchy_prefab", func(t *testing.T) (*image.NRGBA, error) { return shotHierarchyPrefab(t, light(t, 320, 240)) }},
		{"editor_launcher", func(t *testing.T) (*image.NRGBA, error) { return shotLauncher(t, light(t, editorW, editorH)) }},
		{"editor_docs_coroutines", func(t *testing.T) (*image.NRGBA, error) {
			reg, err := docstool.Registry(nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			env := light(t, editorW, editorH)
			_, img, err := shotDocs(env, reg, &context.Editor{UI: env.ui}, "language/coroutines")
			return img, err
		}},
		{"editor_agent_card", func(t *testing.T) (*image.NRGBA, error) { return shotAgent(t, light(t, 420, 520), cardHost()) }},
		{"editor_agent_context", func(t *testing.T) (*image.NRGBA, error) { return shotAgent(t, light(t, 420, 680), contextHost()) }},
		{"editor_animation_normal", func(t *testing.T) (*image.NRGBA, error) {
			env := light(t, 960, 240)
			p, _, _, _ := animationPanel(t, env)
			return env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}}, full) })
		}},
		{"editor_settings_registry", func(t *testing.T) (*image.NRGBA, error) { return shotSettings(t, light(t, 720, 320)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img, err := tc.shot(t)
			checkGolden(t, tc.name+"_light", img, err)
		})
	}
}

// A theme switched in the middle of a session draws the same frame as a session started in that theme.
func TestEditorThemeSwitch(t *testing.T) {
	env := newEditorEnv(t)
	draw := inspectorPanel(t, env)
	if _, err := draw(); err != nil {
		t.Fatal(err)
	}
	env.ui.Theme = theme.Light()
	img, err := draw()
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		return
	}
	compare(t, "testdata/editor_inspector_refs_light.png", img, 8)
}
