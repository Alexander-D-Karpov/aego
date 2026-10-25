//go:build aego_golden

package golden

import (
	"bytes"
	"os"
	"testing"

	"strconv"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	animpanel "aego/editor/panels/animation"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/render/texture"
	"aego/scene"
	"aego/script/bind"
	"aego/ui/dnd"
)

const goldenClipRel = "assets/anim/hero.aeanim"

var goldenClipID = ids.AssetID{Hi: 0xa0a1, Lo: 0x51}

const goldenClip = `anim version=1 length=1 loop=loop fps=12 {
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[1, 1, 1, 1] interp=linear {}
    key t=0.5 v=[1, 0.2, 0.2, 1] {}
    key t=1 v=[1, 1, 1, 1] {}
  }
  track "" prop="transform.scale" kind=vec2 {
    key t=0 v=[1, 1] {}
    key t=0.25 v=[1.2, 0.8] {}
    key t=0.75 v=[0.9, 1.1] {}
  }
  track "" prop="SpriteRenderer.flipX" kind=bool {
    key t=0 v=false {}
    key t=0.5 v=true {}
  }
  event t=0.25 name="Footstep" mode=presentation {}
  event t=0.5 name="Hit" payload=1 {}
  event t=0.75 name="Mark" mode=preview {}
}
`

type goldenFiles map[string][]byte

func (m goldenFiles) Read(rel string) ([]byte, error) {
	b, ok := m[rel]
	if !ok {
		return nil, os.ErrNotExist
	}
	return bytes.Clone(b), nil
}

func (m goldenFiles) Write(rel string, data []byte) error {
	m[rel] = bytes.Clone(data)
	return nil
}

type animDocs struct {
	m *document.Manager
	d *document.SceneDocument
}

func (a animDocs) Doc() *document.SceneDocument { return a.d }
func (a animDocs) Documents() *document.Manager { return a.m }

type animSel struct{ id ids.NodeID }

func (s animSel) Select(...ids.NodeID)               {}
func (s animSel) AddToSelection(...ids.NodeID)       {}
func (s animSel) ToggleSelection(ids.NodeID)         {}
func (s animSel) IsSelected(n ids.NodeID) bool       { return n == s.id }
func (s animSel) Selected() []ids.NodeID             { return []ids.NodeID{s.id} }
func (s animSel) SelectionState() *selection.Service { return nil }

type animNotices struct{}

func (animNotices) Notify(context.Level, string) {}
func (animNotices) Ask(context.Question)         {}

func TestEditorAnimationPanel(t *testing.T) {
	env := newEditorEnvSize(t, 960, 240)
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
	*scene.Add[render2d.SpriteRenderer](s, h) = render2d.DefaultSpriteRenderer()
	pl := animation.DefaultPlayer()
	pl.Clip = animation.ClipRef{ID: goldenClipID}
	*scene.Add[animation.AnimationPlayer](s, h) = pl
	s.Flush()
	d := m.New("main.aescene", s)
	files := goldenFiles{goldenClipRel: []byte(goldenClip)}
	reg := animedit.NewRegistry(files, nil)
	p := animpanel.New(animpanel.Deps{
		Anim: reg, Docs: animDocs{m: m, d: d}, Selection: animSel{id: s.ID(h)}, Notices: animNotices{}, Files: files,
		PathOf: func(id ids.AssetID) (string, bool) { return goldenClipRel, id == goldenClipID },
		Events: func() []string { return []string{"Footstep", "Hit"} },
		Thumbs: whiteThumbs{reg: env.ui.Font().White},
	})
	draw := func(name string) {
		t.Helper()
		img, err := env.draw(func(full mathx.Rect) {
			p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}}, full)
		})
		checkGolden(t, name, img, err)
	}
	draw("editor_animation_normal")
	svc := reg.For(d)
	svc.SetMode(animedit.Preview)
	svc.SetTime(0.3)
	draw("editor_animation_preview")
	svc.SetMode(animedit.Record)
	p.Select(animedit.KeyRef{Track: 0, T: 0.5}, animedit.KeyRef{Track: 1, T: 0.25})
	draw("editor_animation_record")
	svc.SetMode(animedit.Normal)
	p.Select()
	p.ShowCurves(true, 1, 1)
	draw("editor_animation_curves")
	p.ShowCurves(false, -1, 0)
	files[goldenClipRel] = []byte(spriteClip())
	if err := svc.SetClip(goldenClipRel); err != nil {
		t.Fatal(err)
	}
	draw("editor_animation_thumbs")
}

type whiteThumbs struct{ reg texture.Region }

func (w whiteThumbs) ByID(ids.AssetID) (texture.Region, bool) { return w.reg, !w.reg.IsZero() }

func spriteClip() string {
	out := "anim version=1 length=1 loop=loop fps=12 {\n  track \"\" prop=\"SpriteRenderer.sprite\" kind=assetRef {\n"
	for i := range 4 {
		out += "    key t=" + strconv.FormatFloat(float64(i)*0.25, 'g', -1, 32) + " v=asset(\"" + asset.BuiltinRunner(i).String() + "\") {}\n"
	}
	return out + "  }\n  track \"\" prop=\"transform.position\" kind=vec2 {\n    key t=0 v=[0, 0] {}\n    key t=1 v=[1, 0] {}\n  }\n}\n"
}
