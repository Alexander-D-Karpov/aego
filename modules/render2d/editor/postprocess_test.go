package editor

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"reflect"
	"testing"

	"aego/modules/render2d"
	"aego/render/postfx"
	"aego/scene"
	"aego/schema"
)

func TestPostInspectorParameterGestureAndUndo(t *testing.T) {
	s, ed := lightingFixture(t)
	h := s.Create(s.Root(), "Camera")
	*scene.Add[render2d.Camera2D](s, h) = render2d.DefaultCamera2D()
	c := scene.Add[render2d.PostProcess](s, h)
	c.Enabled = true
	s.Flush()
	reg := postfx.NewRegistry()
	desc, _ := reg.Lookup(postfx.VignetteID)
	td, _ := s.Registry().ByName(render2d.NamePostProcess)
	field, _ := td.Field("effects")
	w := postInspector{reg: reg, td: td, field: field, doc: ed.Doc(), node: s.ID(h)}
	c, _ = scene.Get[render2d.PostProcess](s, h)
	w.action(ed, c, func(n *render2d.PostProcess) error { return addEffect(reg, n, desc, "target") })
	fetch := func() *render2d.PostProcess { v, _ := scene.Get[render2d.PostProcess](s, h); return v }
	if len(fetch().Effects) != 1 {
		t.Fatal("Inspector did not add the selected descriptor")
	}
	w.tx = w.doc.History.Begin("one slider drag")
	for _, v := range []float32{.5, .6, .7} {
		draft := fetch().Clone()
		if err := render2d.SetEffectValue(&draft.Effects[0], &desc.Fields[0], schema.Float32(v)); err != nil {
			t.Fatal(err)
		}
		if err := w.write(&draft, w.tx); err != nil {
			t.Fatal(err)
		}
	}
	w.finish()
	if fetch().Effects[0].Parameters[0].Value.X != .7 || w.doc.History.Cursor() != 2 {
		t.Fatal("drag should create exactly one history entry")
	}
	if err := w.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if e := fetch().Effects[0]; e.InstanceID != "target" || e.Parameters[0].Value.X != .35 {
		t.Fatalf("Undo did not restore the original field: %+v", e)
	}
	if err := w.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if len(fetch().Effects) != 0 {
		t.Fatal("Undo did not remove the added effect")
	}
	if err := w.doc.History.Redo(); err != nil {
		t.Fatal(err)
	}
	if fetch().Effects[0].InstanceID != "target" {
		t.Fatal("redo changed the stable ID")
	}
}

func TestFactoryPostProcessPresetsUseOneUndo(t *testing.T) {
	s, ed := lightingFixture(t)
	h := s.Create(s.Root(), "Camera")
	*scene.Add[render2d.Camera2D](s, h) = render2d.DefaultCamera2D()
	reg := postfx.NewRegistry()
	b, _ := reg.NewInstance(postfx.BloomID, "authored")
	*scene.Add[render2d.PostProcess](s, h) = render2d.PostProcess{Enabled: true, Effects: []postfx.Instance{b}}
	s.Flush()
	td, _ := s.Registry().ByName(render2d.NamePostProcess)
	field, _ := td.Field("effects")
	w := postInspector{reg: reg, td: td, field: field, doc: ed.Doc(), node: s.ID(h)}
	current := func() *render2d.PostProcess { c, _ := scene.Get[render2d.PostProcess](s, h); return c }
	for index, want := range [][]string{{postfx.BloomID}, {postfx.BloomID}, {postfx.ColorGradeID}, {postfx.PixelateID, postfx.CRTID}, {postfx.CustomID}} {
		before := current().Clone()
		cursor := ed.Doc().History.Cursor()
		if err := w.applyFactoryPreset(current(), index, "apply preset"); err != nil {
			t.Fatal(index, err)
		}
		got := current()
		if got.Enabled != (index != 0) || len(got.Effects) != len(want) || ed.Doc().History.Cursor() != cursor+1 {
			t.Fatal(index, got)
		}
		for i, typ := range want {
			if got.Effects[i].TypeID != typ || got.Effects[i].InstanceID == "" {
				t.Fatal(index, got)
			}
		}
		if index == 1 && got.Effects[0].InstanceID != "authored" {
			t.Fatal("existing animation target lost")
		}
		if index == 4 && got.Effects[0].Material.ID != asset.BuiltinMaterialPostTint {
			t.Fatal("new project has no usable Custom material")
		}
		if err := ed.Doc().History.Undo(); err != nil || !reflect.DeepEqual(*current(), before) {
			t.Fatalf("undo %d: %+v != %+v (%v)", index, current(), before, err)
		}
	}
}

// Drive the production Draw path through mouse down/up, including its combo pop-up.
// Widget IDs follow instances, so reordering changes only position.
func TestPostInspectorMouseWorkflowSaveReopen(t *testing.T) {
	t.Setenv("AEGO_LANG", "en")
	sc, ed := lightingFixture(t)
	camera := sc.Create(sc.Root(), "Camera")
	*scene.Add[render2d.Camera2D](sc, camera) = render2d.DefaultCamera2D()
	scene.Add[render2d.PostProcess](sc, camera).Enabled = true
	sc.Flush()
	node := sc.ID(camera)
	td, _ := sc.Registry().ByName(render2d.NamePostProcess)
	field, _ := td.Field("effects")
	doc := ed.Doc()
	ui := core.New(nil)
	ed.UI = ui
	w := postInspector{reg: postfx.NewRegistry(), td: td, field: field, doc: doc, node: node, closed: map[string]bool{}}
	var state input.State
	step := func(at mathx.Vec2, down, up bool) {
		state.BeginFrame()
		state.ApplyMouseMove(at, mathx.Vec2{})
		if down {
			state.ApplyMouseDown(at, input.MouseLeft)
		}
		if up {
			state.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(&state, mathx.V2(480, 1300), 1)
		ui.PushCursor(core.Cursor{X: 8, Y: 8, W: 450, RowH: 22, Space: 2})
		w.Draw(ed, []ids.NodeID{node}, func() {})
		ui.PopCursor()
		ui.End()
	}
	click := func(id core.ID) {
		t.Helper()
		for y := 6; y < 1200; y += 4 {
			at := mathx.V2(411, float32(y))
			step(at, false, false)
			if ui.Hot() == id {
				step(at, true, false)
				step(at, false, true)
				return
			}
		}
		t.Fatalf("inspector widget %x is unreachable", id)
	}
	current := func() *render2d.PostProcess { c, _ := scene.Get[render2d.PostProcess](sc, camera); return c }
	click(core.IDOf("Add effect"))
	if len(current().Effects) != 1 || current().Effects[0].TypeID != postfx.VignetteID {
		t.Fatal("Add did not insert the descriptor")
	}
	first := current().Effects[0].InstanceID
	click(core.IDOf(first + "Duplicate effect"))
	if len(current().Effects) != 2 {
		t.Fatal("duplicate didn't add a second Vignette")
	}
	second := current().Effects[1].InstanceID
	if second == first || second == "" {
		t.Fatal("duplicate reused the first track's ID")
	}
	click(core.IDOf(second + "Move up"))
	stack := current().Clone()
	if stack.Effects[0].InstanceID != second || stack.Effects[1].InstanceID != first {
		t.Fatal("reorder lost IDs")
	}
	preset := core.IDOf("post-preset")
	click(preset)
	if !ui.PopupOpen(preset) {
		t.Fatal("preset dropdown did not open")
	}
	cursor := doc.History.Cursor()
	click(core.IDSeq(preset, 4)) // Pixel/CRT
	if len(current().Effects) != 2 || current().Effects[1].TypeID != postfx.CRTID || doc.History.Cursor() != cursor+1 {
		t.Fatal("applying a preset in the Inspector did not create one undo action")
	}
	if err := doc.History.Undo(); err != nil || !reflect.DeepEqual(stack, *current()) {
		t.Fatal("undo lost the reordered stack", err)
	}
	if err := ed.Docs.Save(doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := ed.Docs.Close(doc.ID, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := ed.Docs.Open(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := scene.Get[render2d.PostProcess](reopened.Scene(), reopened.Scene().Handle(node))
	if !ok || !reflect.DeepEqual(stack, *c) {
		t.Fatal("save/open changed the authored stack")
	}
}

func TestReimportPhaseRepairKeepsUnknownSlotAndStableIDs(t *testing.T) {
	reg := postfx.NewRegistry()
	items := []postfx.Instance{{TypeID: postfx.VignetteID, InstanceID: "v"}, {TypeID: "module.unloaded", InstanceID: "keep"}, {TypeID: postfx.BloomID, InstanceID: "a"}, {TypeID: postfx.BloomID, InstanceID: "b"}, {TypeID: postfx.PixelateID, InstanceID: "p"}}
	phaseOf := func(e *postfx.Instance) (postfx.Phase, bool) {
		d, ok := reg.Lookup(e.TypeID)
		if !ok {
			return 0, false
		}
		return d.Phase, true
	}
	if !phaseOrderBroken(items, phaseOf) {
		t.Fatal("unnoticed phase error")
	}
	restorePhaseOrder(items, phaseOf)
	if phaseOrderBroken(items, phaseOf) {
		t.Fatal("phase boundary is still invalid")
	}
	for i, id := range []string{"a", "keep", "b", "v", "p"} {
		if items[i].InstanceID != id {
			t.Fatal("lost ordering", items)
		}
	}
}
