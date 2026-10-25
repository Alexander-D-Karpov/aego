//go:build aego_golden

package golden

import (
	"image"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/panels/projectsettings"
	"aego/engine"
	es "aego/engine/settings"
	"aego/format/aetext"
	settingsmod "aego/modules/settings"
	"aego/project"
	"aego/schema"
)

func settingsGoldenStore(t *testing.T) *es.Store {
	t.Helper()
	st := es.New()
	both := es.User | es.Project
	for _, d := range []es.Desc{
		{ID: "display.window.mode", Owner: "display", Kind: schema.KindEnum, Title: "Window Mode", Category: es.CategoryDisplay, Order: 10, Flags: both | es.Confirm,
			Choices: []es.Choice{
				{Name: "windowed", Label: "Windowed", Value: schema.Enum(0)},
				{Name: "borderless", Label: "Borderless", Value: schema.Enum(1)},
				{Name: "fullscreen", Label: "Fullscreen", Value: schema.Enum(2)},
			}},
		{ID: "display.frameLimit", Owner: "display", Kind: schema.KindInt64, Title: "Frame Limit", Unit: "fps", Category: es.CategoryDisplay, Order: 20,
			Flags: both, Min: 0, Max: 240, HasMin: true, HasMax: true, Step: 1},
		{ID: "display.uiScale", Owner: "display", Kind: schema.KindFloat64, Default: schema.Float64(1), Title: "UI Scale", Category: es.CategoryDisplay, Order: 30,
			Flags: both | es.Accessibility, Min: 0.5, Max: 2, HasMin: true, HasMax: true, Step: 0.05},
		{ID: "audio.bus.music.volume", Owner: "audio", Kind: schema.KindFloat64, Default: schema.Float64(1), Title: "Music Volume", Category: es.CategoryAudio,
			Flags: both, Min: 0, Max: 1, HasMin: true, HasMax: true, Step: 0.05},
		{ID: "audio.bus.music.mute", Owner: "audio", Kind: schema.KindBool, Title: "Mute Music", Category: es.CategoryAudio, Order: 1, Flags: both},
		{ID: "input.bind.Jump", Owner: "input", Kind: schema.KindList, Default: es.Strings("Key.Space", "Pad.A"), Title: "Jump",
			Category: es.CategoryInput, Flags: es.User | es.Controls},
	} {
		if _, err := st.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	st.Freeze()
	proj, err := aetext.Parse([]byte("defaults {\n  audio {\n    bus.music.volume=0.8\n  }\n}\n"), "project.aego")
	if err != nil {
		t.Fatal(err)
	}
	st.LoadProject(proj, "project.aego")
	if err := st.LoadProfile([]byte("profile version=1 {\n  display {\n    window.mode=fullscreen\n  }\n}\n"), "profile"); err != nil {
		t.Fatal(err)
	}
	if err := st.Override(st.Key("audio.bus.music.mute"), schema.Bool(true)); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestEditorSettingsRegistry(t *testing.T) {
	img, err := shotSettings(t, newEditorEnvSize(t, 720, 320))
	checkGolden(t, "editor_settings_registry", img, err)
}

func shotSettings(t *testing.T, env *editorEnv) (*image.NRGBA, error) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "SettingsGolden", false)
	if err != nil {
		t.Fatal(err)
	}
	panel := projectsettings.New(projectsettings.Deps{Project: p, Settings: settingsGoldenStore(t)})
	panel.Show("registry")
	img, err := env.draw(func(full mathx.Rect) { panel.Draw(&services.Frame{UI: env.ui}, full) })
	return img, err
}

func TestGameSettingsView(t *testing.T) {
	st := settingsGoldenStore(t)
	for _, tc := range []struct {
		name string
		tab  string
	}{
		{"game_settings_view", ""},
		{"game_settings_view_input", es.CategoryInput},
	} {
		env := newEditorEnvSize(t, 800, 520)
		v := settingsmod.NewView(st, engine.NewWorld("golden"))
		v.Open()
		if tc.tab != "" {
			v.Select(tc.tab)
		}
		img, err := env.draw(func(full mathx.Rect) { v.DrawUI(env.ui, mathx.Vec2{X: full.W, Y: full.H}) })
		t.Run(tc.name, func(t *testing.T) { checkGolden(t, tc.name, img, err) })
	}
}
