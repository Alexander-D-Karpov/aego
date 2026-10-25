//go:build aego_golden

package golden

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/services"
	animatorpanel "aego/editor/panels/animator"
	"aego/modules/animation"
	"aego/scene"
	"aego/script/bind"
	"aego/ui/dnd"
)

type fakeLive struct{ state string }

func (f fakeLive) State(ids.NodeID) (string, float32, bool) { return f.state, 0.6, true }
func (f fakeLive) Param(_ ids.NodeID, name string) (float64, bool) {
	switch name {
	case "Speed":
		return 4.5, true
	case "Grounded":
		return 1, true
	}
	return 0, true
}
func (fakeLive) SetParam(ids.NodeID, string, float64) {}

func TestEditorAnimatorPanel(t *testing.T) {
	env := newEditorEnvSize(t, 960, 360)
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	m := document.NewManager(document.Options{Root: t.TempDir(), Types: types})
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "Player")
	a := animation.DefaultAnimator()
	a.Controller = animation.ControllerRef{ID: asset.BuiltinPlatformerCtrl}
	*scene.Add[animation.Animator](s, h) = a
	s.Flush()
	d := m.New("main.aescene", s)
	text, _ := asset.BuiltinCtrlText(asset.BuiltinPlatformerCtrl)
	files := goldenFiles{"assets/anim/player.aectrl": []byte(text)}
	var live animatorpanel.Live
	p := animatorpanel.New(animatorpanel.Deps{
		Docs: animDocs{m: m, d: d}, Selection: animSel{id: s.ID(h)}, Notices: animNotices{}, Files: files,
		PathOf: func(id ids.AssetID) (string, bool) {
			return "assets/anim/player.aectrl", id == asset.BuiltinPlatformerCtrl
		},
		Clips: func() []animatorpanel.Clip { return []animatorpanel.Clip{{ID: asset.BuiltinRunClip, Name: "run"}} },
		Live:  func() animatorpanel.Live { return live },
	})
	draw := func(name string) {
		t.Helper()
		img, err := env.draw(func(full mathx.Rect) {
			p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 1}, full)
		})
		checkGolden(t, name, img, err)
	}
	draw("editor_animator_graph")
	p.SelectTransition(0)
	draw("editor_animator_transition")
	p.SelectState(1)
	live = fakeLive{state: "Run"}
	draw("editor_animator_play")
}
