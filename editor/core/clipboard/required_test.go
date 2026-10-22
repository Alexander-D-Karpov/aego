package clipboard

import (
	"testing"

	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type reqBase struct {
	A float32 `aego:"a"`
}

type reqNeeds struct {
	B float32 `aego:"b"`
}

type reqModule struct{}

func (reqModule) Info() module.Info { return module.Info{ID: "reqtest"} }

func (reqModule) Register(r *module.Registry) error {
	if err := module.Component[reqBase](r, schema.Name("reqtest.Base")).Err(); err != nil {
		return err
	}
	return module.Component[reqNeeds](r, schema.Name("reqtest.Needs")).Requires("reqtest.Base").Err()
}

// Pasting a component adds what it requires in the same undo step (D144).
func TestPasteAddsRequiredComponents(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, reqModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	src := s.Create(s.Root(), "Src")
	dst := s.Create(s.Root(), "Dst")
	scene.Add[reqBase](s, src)
	scene.Add[reqNeeds](s, src).B = 4
	s.Flush()
	m := document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()})
	d := m.New("scenes/r.aescene", s)
	ctx := &context.Editor{Types: types, Docs: m, Selection: selection.New(), Commands: command.New(), Clipboard: &fakeClip{}}
	needs, _ := types.Schema().ByName("reqtest.Needs")
	if err := CopyComponent(ctx, s, s.ID(src), needs.ID); err != nil {
		t.Fatal(err)
	}
	if err := PasteComponent(ctx, d, []ids.NodeID{s.ID(dst)}); err != nil {
		t.Fatal(err)
	}
	if !scene.Has[reqBase](s, dst) || !scene.Has[reqNeeds](s, dst) {
		t.Fatal("paste did not add the required Base")
	}
	if n, _ := scene.Get[reqNeeds](s, dst); n.B != 4 {
		t.Fatalf("pasted value %v", n.B)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if scene.Has[reqBase](s, dst) || scene.Has[reqNeeds](s, dst) {
		t.Fatal("one Undo must remove both")
	}
}
