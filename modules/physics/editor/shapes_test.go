package editor

import (
	"errors"
	"testing"

	"aego/contracts/shapes2d"
	"aego/core/mathx"
	"aego/editor/core/history/commands"
	"aego/engine"
	"aego/engine/module"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/scene"
)

// The geometry service is read by lighting in EditWorld as well as PlayWorld.
// Exercise actual History commands so Undo cannot leave a stale shadow contour.
func TestShapeProviderFollowsGizmoRemoveUndoRedoWithoutSolver(t *testing.T) {
	s, p := fixture(t)
	h := s.Create(s.Root(), "Box")
	*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
	s.Flush()
	w, m := engine.NewWorld("edit"), physics.New()
	ctx := &module.AttachContext{World: w, Scene: s, Editor: true}
	if err := m.AttachWorld(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.DetachWorld(ctx) })
	provider, ok := engine.ServiceOf[shapes2d.Provider](w, shapes2d.ServiceID)
	if !ok || physics.SimOf(w) != nil {
		t.Fatal("EditWorld must provide geometry without starting physics")
	}
	read := func() shapes2d.Buffer {
		t.Helper()
		var out shapes2d.Buffer
		if err := provider.Append(s.ID(h), components.NameBox, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := read()
	g := &colliderGizmo{p: p, kind: components.KindBox, td: p.types[components.KindBox]}
	hist := drag(t, s, h, g, 4, mathx.Vec2{X: 1.5})
	after := read()
	if hist.Cursor() != 1 || after.Contours[0].Revision <= before.Contours[0].Revision || after.Points[1].X != 1.5 {
		t.Fatal("gizmo transaction did not update local geometry")
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	undo := read()
	if undo.Contours[0].ID != before.Contours[0].ID || undo.Contours[0].Revision <= after.Contours[0].Revision || undo.Points[1] != before.Points[1] {
		t.Fatal("Undo retained modified geometry or changed its stable identity")
	}
	if err := hist.Redo(); err != nil {
		t.Fatal(err)
	}
	redo := read()
	if redo.Contours[0].Revision <= undo.Contours[0].Revision || redo.Points[1] != after.Points[1] {
		t.Fatal("Redo did not invalidate geometry")
	}
	cmd := commands.NewMutation("Remove collider", commands.EnvFor(s), commands.RemoveComp(s.ID(h), p.types[components.KindBox]))
	if err := hist.Exec(cmd); err != nil {
		t.Fatal(err)
	}
	var missing shapes2d.Buffer
	if err := provider.Append(s.ID(h), components.NameBox, &missing); !errors.Is(err, shapes2d.ErrMissingShape) || len(missing.Contours) != 0 {
		t.Fatalf("removed component retained geometry: %v", err)
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	restored := read()
	if restored.Contours[0].ID != before.Contours[0].ID || restored.Contours[0].Revision <= redo.Contours[0].Revision || restored.Points[1] != redo.Points[1] {
		t.Fatal("undoing removal restored a stale shape generation")
	}
}
