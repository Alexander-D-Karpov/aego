package playdiff_test

import (
	"fmt"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/playdiff"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
)

func pair(t *testing.T) (*document.SceneDocument, *document.SceneDocument, ids.NodeID, *schema.TypeDescriptor) {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	td, ok := types.Schema().ByName("engine.SpriteRenderer")
	if !ok {
		t.Fatal("sprite renderer is not registered")
	}
	id := ids.NewNodeID()
	block := fmt.Sprintf("prefab version=%d {\n  node %q name=\"A\" {\n  }\n}\n", sceneio.FormatVersion, id.String())
	m := document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()})
	var docs [2]*document.SceneDocument
	for i, rel := range []string{"scenes/edit.aescene", "scenes/play.aescene"} {
		s := scene.New(types, nil, scene.Options{})
		if _, _, err := sceneio.LoadInto(s, s.Root(), []byte(block), "test", sceneio.LoadOptions{KeepIDs: true}); err != nil {
			t.Fatal(err)
		}
		s.Flush()
		d := m.New(rel, s)
		if err := d.History.Exec(commands.NewMutation("Add "+td.FullName, commands.EnvFor(s), commands.AddComp(s, id, td, nil))); err != nil {
			t.Fatal(err)
		}
		docs[i] = d
	}
	return docs[0], docs[1], id, td
}

func TestComputeMarksOnlyTouched(t *testing.T) {
	edit, play, id, td := pair(t)
	fd, _ := td.Field("tint")
	if err := play.Props.Set(nil, []ids.NodeID{id}, td.ID, fd.ID, schema.Color(mathx.Color{R: 1, A: 1})); err != nil {
		t.Fatal(err)
	}
	if err := play.Props.EditTransform(nil, []ids.NodeID{id}, func(tr *scene.Transform2D) { tr.Position.X = 5 }); err != nil {
		t.Fatal(err)
	}
	ps := play.Scene()
	ps.Create(ps.Root(), "Spawned")
	ps.Flush()
	touched := map[playdiff.Key]bool{{Node: id, Type: td.ID, Field: fd.ID}: true}
	r := playdiff.Compute(edit, play, touched)
	if len(r.Changes) != 2 || len(r.Created) != 1 || len(r.Removed) != 0 {
		t.Fatalf("changes=%d created=%d removed=%d", len(r.Changes), len(r.Created), len(r.Removed))
	}
	for _, c := range r.Changes {
		if c.Kind == playdiff.KindField && !c.Checked {
			t.Fatal("a field changed through the Inspector in Play must be checked")
		}
		if c.Kind == playdiff.KindTransform2D && c.Checked {
			t.Fatal("a transform moved by the game must stay unchecked")
		}
	}
}

func TestApplyIsOneStep(t *testing.T) {
	edit, play, id, td := pair(t)
	fd, _ := td.Field("tint")
	red := schema.Color(mathx.Color{R: 1, A: 1})
	if err := play.Props.Set(nil, []ids.NodeID{id}, td.ID, fd.ID, red); err != nil {
		t.Fatal(err)
	}
	play.Props.EditTransform(nil, []ids.NodeID{id}, func(tr *scene.Transform2D) { tr.Position.Y = 3 })
	r := playdiff.Compute(edit, play, nil)
	for i := range r.Changes {
		r.Changes[i].Checked = true
	}
	before := edit.History.Cursor()
	n, err := playdiff.Apply(edit, r.Changes)
	if err != nil || n != 2 {
		t.Fatalf("applied %d, err %v", n, err)
	}
	if edit.History.Cursor() != before+1 {
		t.Fatal("applying play changes must be one history step")
	}
	if v, _ := edit.Props.Get(id, td.ID, fd.ID); v.C != red.C {
		t.Fatal("tint did not move to the edit scene")
	}
	edit.History.Undo()
	if v, _ := edit.Props.Get(id, td.ID, fd.ID); v.C == red.C {
		t.Fatal("undo must restore the edit scene")
	}
}

func TestFromHistoryTakesValuesFromCommands(t *testing.T) {
	edit, play, id, td := pair(t)
	fd, _ := td.Field("tint")
	green := schema.Color(mathx.Color{G: 1, A: 1})
	if err := play.Props.Set(nil, []ids.NodeID{id}, td.ID, fd.ID, green); err != nil {
		t.Fatal(err)
	}
	if err := play.Props.EditTransform(nil, []ids.NodeID{id}, func(tr *scene.Transform2D) { tr.Position.X = 5 }); err != nil {
		t.Fatal(err)
	}
	ps := play.Scene()
	h := ps.Handle(id)
	tr := ps.Local(h)
	tr.Position.X = 9
	ps.SetLocal(h, tr)
	ps.Create(ps.Root(), "Spawned")
	ps.Flush()
	r := playdiff.FromHistory(edit, play)
	if len(r.Changes) != 2 || len(r.Created) != 0 {
		t.Fatalf("changes=%d created=%v, want the tint and the transform only", len(r.Changes), r.Created)
	}
	for _, c := range r.Changes {
		if !c.Checked || c.Gameplay || c.Name != "A" {
			t.Fatalf("history row %+v", c)
		}
		if c.Kind == playdiff.KindTransform2D && c.T2After.Position.X != 5 {
			t.Fatalf("transform after = %v, want the edited 5, not the game's 9", c.T2After.Position.X)
		}
		if c.Kind == playdiff.KindField && c.After.C != green.C {
			t.Fatal("tint after is not the value from the command")
		}
	}
	full := playdiff.Full(edit, play, r)
	if len(full.Changes) != 2 || len(full.Created) != 1 {
		t.Fatalf("full changes=%d created=%v", len(full.Changes), full.Created)
	}
}

func TestFullMarksGameChanges(t *testing.T) {
	edit, play, id, td := pair(t)
	fd, _ := td.Field("tint")
	if err := play.Props.Set(nil, []ids.NodeID{id}, td.ID, fd.ID, schema.Color(mathx.Color{G: 1, A: 1})); err != nil {
		t.Fatal(err)
	}
	ps := play.Scene()
	h := ps.Handle(id)
	tr := ps.Local(h)
	tr.Position.Y = 7
	ps.SetLocal(h, tr)
	r := playdiff.FromHistory(edit, play)
	full := playdiff.Full(edit, play, r)
	if len(r.Changes) != 1 || len(full.Changes) != 2 {
		t.Fatalf("history %d, full %d", len(r.Changes), len(full.Changes))
	}
	g := full.Changes[1]
	if g.Kind != playdiff.KindTransform2D || !g.Gameplay || g.Checked {
		t.Fatalf("game row %+v, want an unchecked transform from gameplay", g)
	}
}

func TestFromHistoryEmpty(t *testing.T) {
	edit, play, id, _ := pair(t)
	ps := play.Scene()
	h := ps.Handle(id)
	tr := ps.Local(h)
	tr.Position.X = 3
	ps.SetLocal(h, tr)
	r := playdiff.FromHistory(edit, play)
	if !r.Empty() || len(r.Created)+len(r.Removed) != 0 {
		t.Fatalf("a run with no edits produced %+v", r)
	}
}
