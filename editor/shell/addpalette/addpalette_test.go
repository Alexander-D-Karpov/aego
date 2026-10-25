package addpalette

import (
	"strings"
	"testing"

	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/editor/ext/inspect"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type tBase struct {
	A float32 `aego:"a"`
}
type tMid struct {
	B float32 `aego:"b"`
}
type tTop struct {
	C float32 `aego:"c"`
}
type tSolo struct {
	S float32 `aego:"s"`
}
type tNeedsSolo struct {
	N float32 `aego:"n"`
}
type tShapeA struct {
	X float32 `aego:"x"`
}
type tShapeB struct {
	Y float32 `aego:"y"`
}
type tSensor struct {
	Z float32 `aego:"z"`
}
type tPlayer struct {
	P float32 `aego:"p"`
}
type tAnimator struct {
	Q float32 `aego:"q"`
}

type testModule struct{}

func (testModule) Info() module.Info { return module.Info{ID: "pt"} }

func (testModule) Register(r *module.Registry) error {
	for _, err := range []error{
		module.Component[tBase](r, schema.Name("pt.Base")).Err(),
		module.Component[tMid](r, schema.Name("pt.Mid")).Requires("pt.Base").Err(),
		module.Component[tTop](r, schema.Name("pt.Top")).Requires("pt.Mid").Err(),
		module.Component[tSolo](r, schema.Name("pt.Solo")).SceneUnique().Err(),
		module.Component[tNeedsSolo](r, schema.Name("pt.NeedsSolo")).Requires("pt.Solo").Err(),
		module.Component[tShapeA](r, schema.Name("pt.ShapeA")).Display("Shape A").Err(),
		module.Component[tShapeB](r, schema.Name("pt.ShapeB")).Display("Shape B").Err(),
		module.Component[tSensor](r, schema.Name("pt.Sensor")).RequiresOneOf("pt.ShapeA", "pt.ShapeB").Suggests("pt.Base").Err(),
		module.Component[tPlayer](r, schema.Name("pt.Player")).Display("Player").Err(),
		module.Component[tAnimator](r, schema.Name("pt.Animator")).Display("Animator").Conflicts("pt.Player").Err(),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

type fixture struct {
	s   *Service
	sc  *scene.Scene
	doc *document.SceneDocument
}

func setup(t *testing.T) *fixture {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, testModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	sc := scene.New(types, nil, scene.Options{})
	ctx := &context.Editor{
		Types:     types,
		Docs:      document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()}),
		Selection: selection.New(),
		Commands:  command.New(),
		Settings:  context.DefaultSettings(),
		Collapsed: map[string]bool{},
	}
	d := ctx.Docs.New("scenes/p.aescene", sc)
	meta := inspect.NewRegistry()
	if err := meta.Load(types.Schema()); err != nil {
		t.Fatal(err)
	}
	meta.SetEditor(ctx)
	return &fixture{s: New(Deps{Ctx: ctx, Meta: meta}), sc: sc, doc: d}
}

func (f *fixture) open(nodes ...scene.NodeHandle) {
	f.s.mode, f.s.doc, f.s.nodes = Components, f.doc, nil
	for _, h := range nodes {
		f.s.nodes = append(f.s.nodes, f.sc.ID(h))
	}
	f.s.collect()
	f.s.apply()
}

func (f *fixture) item(t *testing.T, name string) int {
	t.Helper()
	i := f.s.index("comp:" + name)
	if i < 0 {
		t.Fatalf("no item %s", name)
	}
	return i
}

func (f *fixture) has(h scene.NodeHandle, name string) bool {
	td, _ := f.sc.Registry().ByName(name)
	_, ok := f.sc.GetByType(h, td.ID)
	return ok
}

// Adding brings in what it requires, transitively, on every node, and one Undo takes all of it back.
func TestAddBringsRequired(t *testing.T) {
	f := setup(t)
	a := f.sc.Create(f.sc.Root(), "A")
	b := f.sc.Create(f.sc.Root(), "B")
	scene.Add[tBase](f.sc, b)
	f.open(a, b)
	i := f.item(t, "pt.Top")
	f.s.prepare(i)
	if f.s.desc.also != "Also adds: Base, Mid" {
		t.Fatalf("description %q", f.s.desc.also)
	}
	if err := f.s.add(f.doc, f.s.entries[i], nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, h := range []scene.NodeHandle{a, b} {
		for _, n := range []string{"pt.Base", "pt.Mid", "pt.Top"} {
			if !f.has(h, n) {
				t.Fatalf("%s lacks %s", f.sc.Name(h), n)
			}
		}
	}
	if err := f.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if f.has(a, "pt.Base") || f.has(a, "pt.Top") || f.has(b, "pt.Mid") || !f.has(b, "pt.Base") {
		t.Fatal("one Undo must take back exactly what Add put on the nodes")
	}
}

func TestDisabledReasons(t *testing.T) {
	f := setup(t)
	scene.Add[tSolo](f.sc, f.sc.Create(f.sc.Root(), "Holder"))
	n := f.sc.Create(f.sc.Root(), "N")
	scene.Add[tBase](f.sc, n)
	scene.Add[tPlayer](f.sc, n)
	f.sc.Flush()
	f.open(n)
	for name, want := range map[string]string{
		"pt.Base":      "already on the node",
		"pt.Solo":      "one per scene",
		"pt.NeedsSolo": "already has one",
		"pt.Animator":  "Animator and Player drive the same node",
	} {
		if got := f.s.items[f.item(t, name)].Disabled; !strings.Contains(got, want) {
			t.Errorf("%s: reason %q, want %q", name, got, want)
		}
	}
	if f.s.items[f.item(t, "pt.Top")].Disabled != "" {
		t.Error("Top must stay available")
	}
}

// A RequiresOneOf group offers its alternatives; Tab-like switching picks the second, and checked suggestions come along.
func TestAlternativesAndSuggestions(t *testing.T) {
	f := setup(t)
	n := f.sc.Create(f.sc.Root(), "N")
	f.open(n)
	i := f.item(t, "pt.Sensor")
	f.s.prepare(i)
	d := &f.s.desc
	if len(d.choices) != 1 || len(d.names[0]) != 2 || d.also != "Also adds: Shape A" {
		t.Fatalf("choices %v names %v also %q", d.choices, d.names, d.also)
	}
	d.picks[0] = 1
	f.s.alsoAdds()
	if d.also != "Also adds: Shape B" {
		t.Fatalf("after switching: %q", d.also)
	}
	if len(d.sugg) != 1 || d.sugg[0].on {
		t.Fatalf("suggestions %+v; they start unchecked", d.sugg)
	}
	d.sugg[0].on = true
	if err := f.s.add(f.doc, f.s.entries[i], d.picks, d.extras()); err != nil {
		t.Fatal(err)
	}
	if !f.has(n, "pt.Sensor") || !f.has(n, "pt.ShapeB") || f.has(n, "pt.ShapeA") || !f.has(n, "pt.Base") {
		t.Fatal("Add must put the picked alternative and the checked suggestion")
	}
	if err := f.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if f.has(n, "pt.Sensor") || f.has(n, "pt.ShapeB") || f.has(n, "pt.Base") {
		t.Fatal("one Undo must take back the component, the alternative and the suggestion")
	}
}

// With several nodes, a node that cannot take the component is skipped and named in the description.
func TestMultiSelectionSkips(t *testing.T) {
	f := setup(t)
	a := f.sc.Create(f.sc.Root(), "A")
	b := f.sc.Create(f.sc.Root(), "B")
	scene.Add[tPlayer](f.sc, b)
	f.sc.Flush()
	f.open(a, b)
	i := f.item(t, "pt.Animator")
	if f.s.items[i].Disabled != "" {
		t.Fatal("one node fits, the item must be available")
	}
	f.s.prepare(i)
	if !strings.Contains(f.s.desc.skip, "B (") {
		t.Fatalf("skip %q", f.s.desc.skip)
	}
	if err := f.s.add(f.doc, f.s.entries[i], nil, nil); err != nil {
		t.Fatal(err)
	}
	if !f.has(a, "pt.Animator") || f.has(b, "pt.Animator") {
		t.Fatal("Add must go only on the nodes that fit")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	f := setup(t)
	f.s.d.Root = t.TempDir()
	n := f.sc.Create(f.sc.Root(), "N")
	f.open(n)
	i := f.item(t, "pt.Top")
	f.s.used(i)
	f.s.star(f.item(t, "pt.Base"))
	g := New(Deps{Ctx: f.s.d.Ctx, Meta: f.s.d.Meta, Root: f.s.d.Root})
	g.load()
	st := g.store.modes[Components]
	if len(st.recent) != 1 || st.recent[0] != "comp:pt.Top" || len(st.favs) != 1 || g.store.uses["comp:pt.Top"] != 1 || len(st.open) != 1 {
		t.Fatalf("state %+v uses %v", st, g.store.uses)
	}
}
