package integration

import (
	"aego/editor/core/context"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/clipboard"
	"aego/editor/core/history/commands"
	"aego/editor/core/presets"
	"aego/schema"
)

type testEditor struct {
	s   any
	ctx *context.Editor
}

func spriteNodes(t *testing.T, n int) (*testEditor, []ids.NodeID, *schema.TypeDescriptor) {
	t.Helper()
	s, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/multi.aescene"); err != nil {
		t.Fatal(err)
	}
	d := ctx.Doc()
	sc := d.Scene()
	td, ok := sc.Registry().ByName("engine.SpriteRenderer")
	if !ok {
		t.Fatal("sprite renderer is not registered")
	}
	var nodes []ids.NodeID
	for i := 0; i < n; i++ {
		h := sc.Create(sc.Root(), "N")
		sc.Flush()
		id := sc.ID(h)
		if err := d.History.Exec(commands.NewMutation("Add "+td.FullName, commands.EnvFor(sc), commands.AddComp(sc, id, td, nil))); err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, id)
	}
	return &testEditor{s: s, ctx: ctx}, nodes, td
}

func TestMultiEditIsOneStep(t *testing.T) {
	te, nodes, td := spriteNodes(t, 3)
	d := te.ctx.Doc()
	fd, _ := td.Field("tint")
	red := schema.Color(mathx.Color{R: 1, A: 1})
	before := d.History.Cursor()
	if err := d.Props.Set(nil, nodes, td.ID, fd.ID, red); err != nil {
		t.Fatal(err)
	}
	if d.History.Cursor() != before+1 {
		t.Fatal("editing three nodes must be one history step")
	}
	if _, mixed, _ := d.Props.Common(nodes, td.ID, fd.ID); mixed {
		t.Fatal("all three nodes must carry the new value")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if v, _ := d.Props.Get(n, td.ID, fd.ID); v.C == red.C {
			t.Fatal("one undo must restore every node")
		}
	}
}

func TestComponentSetCopyPaste(t *testing.T) {
	te, nodes, td := spriteNodes(t, 1)
	ctx := te.ctx
	d := ctx.Doc()
	sc := d.Scene()
	empty := sc.ID(sc.Create(sc.Root(), "Empty"))
	sc.Flush()
	env := clipboard.EnvOf(ctx)
	if err := env.CopyComponents(sc, nodes[0], []schema.TypeID{td.ID}); err != nil {
		t.Fatal(err)
	}
	if env.Peek() != clipboard.KindComponents {
		t.Fatal("the clipboard must hold a component set")
	}
	if err := env.PasteComponents(d, []ids.NodeID{empty}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sc.GetByType(sc.Handle(empty), td.ID); !ok {
		t.Fatal("pasting the set must add the component")
	}
}

func TestPresetRoundTrip(t *testing.T) {
	te, nodes, td := spriteNodes(t, 2)
	ctx := te.ctx
	d := ctx.Doc()
	fd, _ := td.Field("tint")
	green := schema.Color(mathx.Color{G: 1, A: 1})
	if err := d.Props.Set(nil, nodes[:1], td.ID, fd.ID, green); err != nil {
		t.Fatal(err)
	}
	svc := presets.New(ctx)
	rel, err := svc.Save(d, nodes[0], td, "Green")
	if err != nil {
		t.Fatal(err)
	}
	list := svc.For(td.FullName)
	if len(list) != 1 || list[0].Path != rel {
		t.Fatalf("presets for %s = %+v", td.FullName, list)
	}
	if err := svc.Apply(d, nodes[1:], list[0].ID); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Props.Get(nodes[1], td.ID, fd.ID); v.C != green.C {
		t.Fatal("the preset must set the tint")
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Props.Get(nodes[1], td.ID, fd.ID); v.C == green.C {
		t.Fatal("undo must revert the preset")
	}
}
