package shell

import (
	"bytes"
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/selection"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/render/text"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/core"
)

// This uses the actual recovery confirmation with synthetic key input and a
// temporary project, not a physical window or the user's recovery files.
func TestRecoveryModalKeepsSelectedScene(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	opt := document.Options{Root: t.TempDir(), Types: types}
	makeScene := func(name string) *scene.Scene {
		sc := scene.New(types, nil, scene.Options{})
		sc.Create(sc.Root(), name)
		sc.Flush()
		return sc
	}
	old := document.NewManager(opt)
	selected := old.New("scenes/selected.aescene", makeScene("Hero"))
	if err := old.Save(selected.ID); err != nil {
		t.Fatal(err)
	}
	old.New("scenes/unknown.aescene", makeScene("Recovered"))
	if n, err := old.WriteRecovery(); err != nil || n != 1 {
		t.Fatalf("create recovery: %d, %v", n, err)
	}
	docs := document.NewManager(opt)
	active, err := docs.Open(selected.Path)
	if err != nil {
		t.Fatal(err)
	}
	recoveries := docs.FindRecovery()
	if len(recoveries) != 1 {
		t.Fatalf("expected one pending recovery: %+v", recoveries)
	}
	selectedNode := active.Scene().Find(active.Scene().Root(), "Hero")
	item := selection.Item{Doc: active.ID, Node: active.Scene().ID(selectedNode)}
	ed := &context.Editor{Docs: docs, Selection: selection.New()}
	ed.Selection.Set(item)
	before := sceneio.Save(active.Scene())
	font := text.NewFont(14, 1, 16, 12)
	for r := rune(32); r <= 0x04ff; r++ {
		font.SetGlyph(r, text.FontGlyph{Advance: 8})
	}
	ui, in := core.New(font), &input.State{}
	a := &App{ui: ui, ctx: ed, modals: []modal{{kind: modalRecovery, rec: recoveries[0]}}}
	draw := func() {
		ui.Begin(in, mathx.Vec2{X: 1000, Y: 700}, 1)
		a.drawModals()
		ui.End()
	}
	in.BeginFrame()
	draw()
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEnter, 0, 0, false)
	draw()
	restored := docs.Find("scenes/unknown.aescene")
	got, selectedOK := ed.Selection.Active()
	if len(a.modals) != 0 || restored == nil || !restored.Dirty() || docs.Active() != active || !selectedOK || got != item || !bytes.Equal(before, sceneio.Save(active.Scene())) {
		t.Fatal("confirming recovery did not preserve the active scene and selection")
	}
	notice, ok := ed.LastNotice()
	if !ok || notice.Level != context.LevelInfo || !strings.Contains(i18n.EnglishOf(notice.Text), restored.Path) {
		t.Fatalf("background recovery was not reported: %+v", notice)
	}
}
