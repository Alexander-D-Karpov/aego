//go:build aego_golden

package golden

import (
	"fmt"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	pd "aego/editor/core/playdiff"
	"aego/editor/core/selection"
	"aego/editor/panels/playdiff"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
)

func TestEditorPlayDiff(t *testing.T) {
	env := newEditorEnv(t)
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}, prefabModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	td, _ := types.Schema().ByName("golden.Stats")
	speed, _ := td.Field("speed")
	tint, _ := td.Field("tint")
	id := ids.NodeID{Hi: 0x51, Lo: 0x1}
	spawned := ids.NodeID{Hi: 0x51, Lo: 0x2}
	block := fmt.Sprintf("prefab version=%d {\n  node %q name=\"Slime\" {\n  }\n}\n", sceneio.FormatVersion, id.String())
	m := document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()})
	var docs [2]*document.SceneDocument
	for i, rel := range []string{"scenes/level.aescene", "scenes/level-play.aescene"} {
		s := scene.New(types, nil, scene.Options{})
		if _, _, err := sceneio.LoadInto(s, s.Root(), []byte(block), "golden", sceneio.LoadOptions{KeepIDs: true}); err != nil {
			t.Fatal(err)
		}
		s.Flush()
		d := m.New(rel, s)
		if err := d.History.Exec(commands.NewMutation("Add "+td.FullName, commands.EnvFor(s), commands.AddComp(s, id, td, nil))); err != nil {
			t.Fatal(err)
		}
		docs[i] = d
	}
	edit, play := docs[0], docs[1]
	one := []ids.NodeID{id}
	play.Props.Set(nil, one, td.ID, speed.ID, schema.Float32(7.5))
	play.Props.Set(nil, one, td.ID, tint.ID, schema.Color(mathx.Color{R: 0.9, G: 0.2, B: 0.2, A: 1}))
	ps := play.Scene()
	h := ps.Handle(id)
	tr := ps.Local(h)
	tr.Position = mathx.Vec2{X: 3.25, Y: -1}
	ps.SetLocal(h, tr)
	ps.CreateWithID(ps.Root(), "Coin", spawned)
	ps.Flush()
	hist := pd.FromHistory(edit, play)
	full := pd.Full(edit, play, hist)

	ctx := &context.Editor{UI: env.ui, Types: types, Docs: m, Selection: selection.New(), Settings: context.DefaultSettings()}
	dlg := playdiff.New(playdiff.Deps{Docs: ctx, Notices: ctx, ShowAll: func() bool { return true }})
	dlg.Open(env.ui, edit.ID, hist, full)
	img, err := env.draw(func(full mathx.Rect) { dlg.Draw(env.ui) })
	checkGolden(t, "editor_playdiff", img, err)
}
