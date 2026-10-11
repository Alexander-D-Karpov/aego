package edit

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	"aego/app"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/history/commands"
	"aego/engine"
	"aego/project"
	"aego/render2d"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/stdlib"
	"aego/sdk"
)

type Spin struct {
	Speed float32 `aego:"speed"`
}

type spinSystem struct{}

func (spinSystem) Run(ctx *engine.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	scene.Query1[Spin](s).Each(func(h scene.NodeHandle, sp *Spin) {
		t := s.Local(h)
		t.Rotation += sp.Speed * dt
		s.SetLocal(h, t)
	})
}

type spinModule struct{}

func (spinModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "test"}
}

func (spinModule) Register(r *sdk.Registry) error {
	if err := sdk.Component[Spin](r, schema.Name("test.Spin")).Err(); err != nil {
		return err
	}
	r.System(engine.SystemDesc{ID: "test.spin", Phase: engine.PhaseUpdate, System: spinSystem{}})
	return nil
}

func writeScene(t *testing.T, p *project.Project, rel string, build func(s *scene.Scene)) {
	t.Helper()
	types := app.NewTypes()
	if _, err := app.BuildModules(types, nil, stdlib.Options{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	build(s)
	s.Flush()
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), sceneio.Save(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newProject(t *testing.T) *project.Project {
	t.Helper()
	p, err := project.Create(t.TempDir(), "EditorTest", false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func headless() Config {
	return Config{Headless: true, NoWatch: true, Logger: log.Nop()}
}

func TestPlayLeavesEditSceneByteIdentical(t *testing.T) {
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
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	es := d.Scene()
	spinner := es.ID(es.Find(es.Root(), "Spinner"))
	td, ok := ctx.Types.Schema().ByGoType(reflect.TypeFor[Spin]())
	if !ok {
		t.Fatal("Spin is not registered")
	}
	speed, _ := td.Field("speed")
	add := &commands.AddComponent{
		Node:   spinner,
		Type:   td.ID,
		Name:   td.FullName,
		Values: []commands.FieldValue{{Field: speed.ID, Value: schema.Float32(3)}},
	}
	if err := d.History.Exec(add); err != nil {
		t.Fatal(err)
	}
	d.History.MarkSaved()

	before := sceneio.Save(d.Scene())
	pc := s.App.Play()
	if err := pc.Start(d); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		time.Sleep(time.Millisecond)
		s.Step()
	}
	ps := pc.Scene()
	if ps.Local(ps.Find(ps.Root(), "Spinner")).Rotation == 0 {
		t.Fatal("gameplay system did not run in the play world")
	}
	pc.Stop()
	if string(sceneio.Save(d.Scene())) != string(before) {
		t.Fatal("play mode changed the edit scene")
	}
	if d.Dirty() {
		t.Fatal("play mode dirtied the document")
	}
}

func TestHandoffRestoresUnsavedWork(t *testing.T) {
	p := newProject(t)
	rel := "scenes/main.aescene"
	writeScene(t, p, rel, func(s *scene.Scene) {
		s.Create(s.Root(), "Hero")
	})
	cfg := headless()
	cfg.Supervised = true

	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	sc := d.Scene()
	hero := sc.ID(sc.Find(sc.Root(), "Hero"))
	if err := d.Props.Rename(nil, hero, "Renamed"); err != nil {
		t.Fatal(err)
	}
	ctx.Select(hero)
	d.View.Camera.Zoom = 128
	if err := s.App.Handoff(); err != nil {
		t.Fatal(err)
	}
	for s.Step() {
	}
	if !s.App.Restarting() {
		t.Fatal("handoff must mark the session as restarting")
	}
	s.Close()

	again, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	again.Step()
	ctx2 := again.App.Context()
	d2 := ctx2.Doc()
	if d2 == nil || d2.Path != rel {
		t.Fatal("active scene was not restored")
	}
	if !d2.Dirty() {
		t.Fatal("unsaved changes must stay unsaved after the handoff")
	}
	s2 := d2.Scene()
	if s2.Find(s2.Root(), "Renamed").IsZero() {
		t.Fatal("unsaved rename lost")
	}
	if d2.View.Camera.Zoom != 128 {
		t.Fatalf("camera zoom = %v", d2.View.Camera.Zoom)
	}
	if sel := ctx2.Selected(); len(sel) != 1 || sel[0] != hero {
		t.Fatalf("selection = %v", sel)
	}
}

func TestTenThousandNodesIdleBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	p := newProject(t)
	rel := "scenes/big.aescene"
	var parents []ids.NodeID
	writeScene(t, p, rel, func(s *scene.Scene) {
		cam := s.Create(s.Root(), "Camera")
		*scene.Add[render2d.Camera2D](s, cam) = render2d.DefaultCamera2D()
		for i := 0; i < 100; i++ {
			g := s.Create(s.Root(), "Group "+strconv.Itoa(i))
			parents = append(parents, s.ID(g))
			for j := 0; j < 99; j++ {
				n := s.Create(g, "Node "+strconv.Itoa(j))
				s.SetPosition(n, mathx.Vec2{X: float32(j), Y: float32(i)})
				sr := scene.Add[render2d.SpriteRenderer](s, n)
				*sr = render2d.DefaultSpriteRenderer()
				sr.Size = mathx.Vec2{X: 1, Y: 1}
			}
		}
	})
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	d, err := s.App.Context().Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range parents {
		d.View.Expanded[id] = true
	}
	for i := 0; i < 30; i++ {
		s.Step()
	}
	const frames = 120
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	for i := 0; i < frames; i++ {
		s.Step()
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	perFrame := (after.TotalAlloc - before.TotalAlloc) / frames
	t.Logf("idle frame: %v, %d bytes allocated", elapsed/frames, perFrame)
	if perFrame > 64<<10 {
		t.Fatalf("idle frame allocates %d bytes, budget is 64 KB", perFrame)
	}
}
