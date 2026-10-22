package inspector

import (
	"strings"
	"testing"

	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/editor/ext/inspect"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

type addBase struct {
	A float32 `aego:"a"`
}

type addMid struct {
	B float32 `aego:"b"`
}

type addTop struct {
	C float32 `aego:"c"`
}

type addSolo struct {
	S float32 `aego:"s"`
}

type addNeedsSolo struct {
	N float32 `aego:"n"`
}

type addModule struct{}

func (addModule) Info() module.Info {
	return module.Info{ID: "addtest"}
}

func (addModule) Register(r *module.Registry) error {
	if err := module.Component[addBase](r, schema.Name("addtest.Base")).Err(); err != nil {
		return err
	}
	if err := module.Component[addMid](r, schema.Name("addtest.Mid")).Requires("addtest.Base").Err(); err != nil {
		return err
	}
	if err := module.Component[addSolo](r, schema.Name("addtest.Solo")).SceneUnique().Err(); err != nil {
		return err
	}
	if err := module.Component[addNeedsSolo](r, schema.Name("addtest.NeedsSolo")).Requires("addtest.Solo").Err(); err != nil {
		return err
	}
	return module.Component[addTop](r, schema.Name("addtest.Top")).Requires("addtest.Mid").Err()
}

// Adding a component brings in what it requires, transitively, and one Undo takes all of it back.
func TestAddBringsRequiredComponents(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, addModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	a := s.Create(s.Root(), "A")
	b := s.Create(s.Root(), "B")
	scene.Add[addBase](s, b)
	ctx := &context.Editor{
		Types:     types,
		Docs:      document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()}),
		Selection: selection.New(),
		Commands:  command.New(),
		Settings:  context.DefaultSettings(),
		Collapsed: map[string]bool{},
	}
	d := ctx.Docs.New("scenes/add.aescene", s)
	reg := inspect.NewRegistry()
	if err := reg.Load(types.Schema()); err != nil {
		t.Fatal(err)
	}
	reg.SetEditor(ctx)
	p := New(reg, Deps{Docs: ctx, Selection: ctx, Commands: ctx.Commands, Notices: ctx, Nav: ctx, Problems: ctx, Clip: clipboard.EnvOf(ctx), Collapsed: ctx.Collapsed})
	top, _ := types.Schema().ByName("addtest.Top")
	if why := p.blocked(s, top); why != "" {
		t.Fatalf("Top is blocked: %s", why)
	}
	p.nodes = []ids.NodeID{s.ID(a), s.ID(b)}
	if got, _ := mutate.Required(s, top, a); len(got) != 2 || got[0].FullName != "addtest.Base" || got[1].FullName != "addtest.Mid" {
		t.Fatalf("chain %v", got)
	}
	p.add(d, top)
	has := func(h scene.NodeHandle, name string) bool {
		td, _ := types.Schema().ByName(name)
		_, ok := s.GetByType(h, td.ID)
		return ok
	}
	for _, h := range []scene.NodeHandle{a, b} {
		for _, name := range []string{"addtest.Base", "addtest.Mid", "addtest.Top"} {
			if !has(h, name) {
				t.Fatalf("%s lacks %s after Add", s.Name(h), name)
			}
		}
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if has(a, "addtest.Base") || has(a, "addtest.Mid") || has(a, "addtest.Top") || has(b, "addtest.Mid") || has(b, "addtest.Top") {
		t.Fatal("one Undo left added components behind")
	}
	if !has(b, "addtest.Base") {
		t.Fatal("Undo removed a component the node had before")
	}
}

// A requirement that is one per scene and already in the scene blocks the entry with the reason.
func TestAddBlockedBySceneUniqueRequirement(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, addModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	scene.Add[addSolo](s, s.Create(s.Root(), "Holder"))
	n := s.Create(s.Root(), "N")
	s.Flush()
	p := New(inspect.NewRegistry(), Deps{})
	p.nodes = []ids.NodeID{s.ID(n)}
	td, _ := types.Schema().ByName("addtest.NeedsSolo")
	why := p.blocked(s, td)
	if !strings.Contains(why, "already has one") {
		t.Fatalf("blocked = %q, want the scene-unique reason", why)
	}
}
