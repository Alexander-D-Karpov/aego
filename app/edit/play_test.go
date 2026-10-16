package edit

import (
	"reflect"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/templates"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type spinFixture struct {
	s       *Session
	doc     *document.SceneDocument
	spinner ids.NodeID
	typ     schema.TypeID
	speed   schema.FieldID
}

func openSpinner(t *testing.T) *spinFixture {
	t.Helper()
	p := newProject(t)
	rel := "scenes/play.aescene"
	writeScene(t, p, rel, func(s *scene.Scene) {
		cam := s.Create(s.Root(), "Camera")
		*scene.Add[render2d.Camera2D](s, cam) = render2d.DefaultCamera2D()
		s.Create(s.Root(), "Spinner")
	})
	s, err := Open(p, headless(), spinModule{})
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	es := d.Scene()
	f := &spinFixture{s: s, doc: d, spinner: es.ID(es.Find(es.Root(), "Spinner"))}
	td, ok := ctx.Types.Schema().ByGoType(reflect.TypeFor[Spin]())
	if !ok {
		t.Fatal("Spin is not registered")
	}
	fd, _ := td.Field("speed")
	f.typ, f.speed = td.ID, fd.ID
	add := &commands.AddComponent{
		Node:   f.spinner,
		Type:   td.ID,
		Name:   td.FullName,
		Values: []commands.FieldValue{{Field: fd.ID, Value: schema.Float32(3)}},
	}
	if err := d.History.Exec(add); err != nil {
		t.Fatal(err)
	}
	d.History.MarkSaved()
	return f
}

func speedIn(t *testing.T, s *scene.Scene, id ids.NodeID) float32 {
	t.Helper()
	sp, ok := scene.Get[Spin](s, s.Handle(id))
	if !ok {
		t.Fatal("Spin missing")
	}
	return sp.Speed
}

func TestPlayEditsGoToPlayScene(t *testing.T) {
	f := openSpinner(t)
	defer f.s.Close()
	ctx := f.s.App.Context()
	before := sceneio.Save(f.doc.Scene())
	pc := f.s.App.Play()
	if err := pc.Start(f.doc); err != nil {
		t.Fatal(err)
	}
	if ctx.Doc() == f.doc || !ctx.Docs.InPlay() {
		t.Fatal("the active document must be the play view")
	}
	if err := ctx.Props().Set(nil, []ids.NodeID{f.spinner}, f.typ, f.speed, schema.Float32(9)); err != nil {
		t.Fatal(err)
	}
	if v := speedIn(t, pc.Scene(), f.spinner); v != 9 {
		t.Fatalf("play speed = %v", v)
	}
	if f.doc.Dirty() {
		t.Fatal("play edit dirtied the document")
	}
	pc.Stop()
	if string(sceneio.Save(f.doc.Scene())) != string(before) {
		t.Fatal("edit scene changed")
	}
	if f.doc.Dirty() || ctx.Doc() != f.doc {
		t.Fatal("stop must return to a clean edit document")
	}
}

func TestPlayCreatedNodesVanishOnStop(t *testing.T) {
	f := openSpinner(t)
	defer f.s.Close()
	ctx := f.s.App.Context()
	pc := f.s.App.Play()
	pc.Start(f.doc)
	empty, _ := f.s.App.Templates().Get("engine.empty")
	created, err := templates.Instantiate(ctx, empty, ids.NodeID{}, -1, nil)
	if err != nil || len(created) != 1 {
		t.Fatalf("create in play: %v %v", created, err)
	}
	if len(ctx.Selected()) != 1 {
		t.Fatal("created node must be selected")
	}
	pc.Stop()
	if !f.doc.Scene().Handle(created[0]).IsZero() {
		t.Fatal("play-created node leaked into the edit scene")
	}
	if len(ctx.Selected()) != 0 {
		t.Fatal("selection must drop nodes that no longer exist")
	}
}

func TestCopyValuesToEditScene(t *testing.T) {
	f := openSpinner(t)
	defer f.s.Close()
	ctx := f.s.App.Context()
	pc := f.s.App.Play()
	pc.Start(f.doc)
	ctx.Props().Set(nil, []ids.NodeID{f.spinner}, f.typ, f.speed, schema.Float32(7))
	if !pc.CanCopyToEdit([]ids.NodeID{f.spinner}, f.typ) {
		t.Fatal("copy must be allowed for a node that exists in both scenes")
	}
	if err := pc.CopyToEdit([]ids.NodeID{f.spinner}, f.typ); err != nil {
		t.Fatal(err)
	}
	if !f.doc.Dirty() || speedIn(t, f.doc.Scene(), f.spinner) != 7 {
		t.Fatal("copy did not reach the edit scene")
	}
	pc.Stop()
	if err := f.doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if v := speedIn(t, f.doc.Scene(), f.spinner); v != 3 {
		t.Fatalf("undo left speed %v", v)
	}
}

func TestUndoInsidePlay(t *testing.T) {
	f := openSpinner(t)
	defer f.s.Close()
	ctx := f.s.App.Context()
	pc := f.s.App.Play()
	pc.Start(f.doc)
	ctx.Props().Set(nil, []ids.NodeID{f.spinner}, f.typ, f.speed, schema.Float32(9))
	if err := ctx.Undo(); err != nil {
		t.Fatal(err)
	}
	if v := speedIn(t, pc.Scene(), f.spinner); v != 3 {
		t.Fatalf("play undo left %v", v)
	}
	if f.doc.Dirty() {
		t.Fatal("play undo touched the document")
	}
}

func TestPlayMouseFollowsPanelOrigin(t *testing.T) {
	f := openSpinner(t)
	defer f.s.Close()
	pc := f.s.App.Play()
	if err := pc.Start(f.doc); err != nil {
		t.Fatal(err)
	}
	cursor := mathx.Vec2{X: 500, Y: 300}
	f.s.Engine.Input().ApplyMouseMove(cursor, mathx.Vec2{})
	for _, origin := range []mathx.Vec2{{X: 100, Y: 50}, {X: 320, Y: 200}} {
		pc.SetCapture(true, origin, mathx.Vec2{X: 1000, Y: 1000})
		pc.Tick(1.0 / 60)
		if got, want := pc.World().Input.MousePos(), cursor.Sub(origin); got != want {
			t.Fatalf("origin %v: game mouse %v, want %v", origin, got, want)
		}
	}
}

func TestClickOutsideGameReleasesBeforeGameSeesIt(t *testing.T) {
	f := openSpinner(t)
	defer f.s.Close()
	pc := f.s.App.Play()
	if err := pc.Start(f.doc); err != nil {
		t.Fatal(err)
	}
	pc.SetCapture(true, mathx.Vec2{X: 100, Y: 50}, mathx.Vec2{X: 400, Y: 300})
	in := f.s.Engine.Input()
	in.BeginFrame()
	in.ApplyMouseDown(mathx.Vec2{X: 700, Y: 20}, input.MouseLeft)
	pc.Tick(1.0 / 60)
	if pc.Captured() {
		t.Fatal("a click outside the game view must release the capture")
	}
	if pc.World().Input.ButtonPressed(input.MouseLeft) {
		t.Fatal("the releasing click leaked into the game")
	}
}
