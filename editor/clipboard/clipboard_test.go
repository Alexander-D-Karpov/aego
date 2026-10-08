package clipboard

import (
	"aego/core/ids"
	"testing"

	"aego/core/log"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/history/commands"
	"aego/editor/selection"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type Link struct {
	Target scene.NodeRef `aego:"target"`
}

type fakeClip struct {
	text string
	data map[string][]byte
}

func (f *fakeClip) Text() string {
	return f.text
}

func (f *fakeClip) SetText(s string) {
	f.text = s
	f.data = nil
}

func (f *fakeClip) SetData(mime string, data []byte, text string) bool {
	f.data = map[string][]byte{mime: data}
	f.text = text
	return true
}

func (f *fakeClip) Data(mime string) ([]byte, bool) {
	d, ok := f.data[mime]
	return d, ok
}

func setup(t *testing.T) (*context.Editor, *document.SceneDocument) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Link](types, schema.Name("test.Link")); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	a := s.Create(s.Root(), "A")
	b := s.Create(a, "B")
	c := s.Create(s.Root(), "C")
	scene.Add[Link](s, a).Target.Set(s, b)
	scene.Add[Link](s, c).Target.Set(s, b)
	s.Flush()
	m := document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()})
	d := m.New("scenes/t.aescene", s)
	ctx := &context.Editor{
		Types:     types,
		Docs:      m,
		Selection: selection.New(),
		Commands:  command.New(),
		Clipboard: &fakeClip{},
	}
	return ctx, d
}

func TestCopyPasteRemapsInternalRefs(t *testing.T) {
	ctx, d := setup(t)
	s := d.Scene()
	a := s.Find(s.Root(), "A")
	b := s.Find(s.Root(), "A/B")
	ctx.Select(s.ID(a))
	if err := Copy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Paste(ctx); err != nil {
		t.Fatal(err)
	}
	sel := ctx.Selected()
	if len(sel) != 1 {
		t.Fatalf("selection = %v", sel)
	}
	pasted := s.Handle(sel[0])
	if pasted == a || s.ChildCount(s.Root()) != 3 || commands.ChildIndex(s, pasted) != 1 {
		t.Fatal("paste must land right after the selected node")
	}
	child := s.Child(pasted, "B")
	if child.IsZero() || child == b {
		t.Fatal("pasted subtree must have its own child")
	}
	link, ok := scene.Get[Link](s, pasted)
	if !ok {
		t.Fatal("component lost")
	}
	if got, ok := link.Target.Resolve(s); !ok || got != child {
		t.Fatal("internal reference must follow the copy")
	}
	d.History.Undo()
	if s.ChildCount(s.Root()) != 2 {
		t.Fatal("paste must be one undo step")
	}
}

func TestExternalRefKeepsTarget(t *testing.T) {
	ctx, d := setup(t)
	s := d.Scene()
	c := s.Find(s.Root(), "C")
	b := s.Find(s.Root(), "A/B")
	ctx.Select(s.ID(c))
	Copy(ctx)
	if err := Paste(ctx); err != nil {
		t.Fatal(err)
	}
	pasted := s.Handle(ctx.Selected()[0])
	link, _ := scene.Get[Link](s, pasted)
	if got, ok := link.Target.Resolve(s); !ok || got != b {
		t.Fatal("reference outside the fragment must keep pointing at the original")
	}
}

func TestPasteFromPlainText(t *testing.T) {
	ctx, d := setup(t)
	s := d.Scene()
	data, _, err := Fragment(s, []ids.NodeID{s.ID(s.Find(s.Root(), "C"))})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Clipboard.SetText(string(data))
	if err := Paste(ctx); err != nil {
		t.Fatal(err)
	}
	if s.ChildCount(s.Root()) != 3 {
		t.Fatal("text clipboard paste failed")
	}
}

func TestDuplicateAndDeleteAreSingleSteps(t *testing.T) {
	ctx, d := setup(t)
	s := d.Scene()
	a := s.Find(s.Root(), "A")
	b := s.Find(s.Root(), "A/B")
	ctx.Select(s.ID(a), s.ID(b))
	if err := Duplicate(ctx); err != nil {
		t.Fatal(err)
	}
	if s.ChildCount(s.Root()) != 3 {
		t.Fatalf("duplicate must copy only the top-level node, root has %d", s.ChildCount(s.Root()))
	}
	d.History.Undo()
	if s.ChildCount(s.Root()) != 2 {
		t.Fatal("duplicate undo failed")
	}
	bID := s.ID(b)
	ctx.Select(s.ID(a), bID)
	if err := Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if s.ChildCount(s.Root()) != 1 {
		t.Fatal("delete failed")
	}
	d.History.Undo()
	if s.ChildCount(s.Root()) != 2 || s.Handle(bID).IsZero() {
		t.Fatal("one undo must restore the whole subtree")
	}
}
