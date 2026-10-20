package integration

import (
	"testing"

	"aego/core/ids"
	"aego/editor/panels/sceneview"
)

func TestIsolationKeepsSubtrees(t *testing.T) {
	_, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/iso.aescene"); err != nil {
		t.Fatal(err)
	}
	s := ctx.Doc().Scene()
	a := s.Create(s.Root(), "A")
	a1 := s.Create(a, "A1")
	b := s.Create(s.Root(), "B")
	s.Flush()
	vis := sceneview.Visible(s, map[ids.NodeID]bool{s.ID(a): true})
	if !vis[a] || !vis[a1] || vis[b] {
		t.Fatalf("visible a=%v a1=%v b=%v", vis[a], vis[a1], vis[b])
	}
	before := len(ctx.Docs.Docs())
	ctx.Select(s.ID(a))
	if err := ctx.Commands.Run(sceneview.CommandIsolate); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Doc().View.Isolated) != 1 || len(ctx.Docs.Docs()) != before || ctx.Doc().Dirty() {
		t.Fatal("isolation is view state: it must not touch the scene")
	}
	ctx.Commands.Run(sceneview.CommandIsolate)
	if len(ctx.Doc().View.Isolated) != 0 {
		t.Fatal("the second Shift+H shows everything again")
	}
}
