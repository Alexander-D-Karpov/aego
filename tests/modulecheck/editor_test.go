package modulecheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/assembly/editors"
	"aego/app/edit"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/services"
	"aego/editor/core/vectors"
	"aego/editor/panels/gameview"
	"aego/editor/panels/profiler"
	"aego/engine/module"
	"aego/gpu"
	"aego/modules/physics"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/project"
	"aego/render/rt"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/tests/modulecheck/fakephys"
	fpeditor "aego/tests/modulecheck/fakephys/editor"
)

type editorFixture struct {
	s    *edit.Session
	m    *fakephys.Module
	p    *fpeditor.Plugin
	doc  *document.SceneDocument
	body *schema.TypeDescriptor
}

func editorCatalog(m *fakephys.Module, p *fpeditor.Plugin) assembly.Catalog {
	c := editors.With(builtin.Default()).Without(physics.ID)
	return append(c, assembly.Entry{
		ID:     fakephys.ID,
		New:    func(assembly.Options) module.Module { return m },
		Editor: p.Hook,
	})
}

func openEditor(t *testing.T, build func(*scene.Scene)) *editorFixture {
	t.Helper()
	f := &editorFixture{m: &fakephys.Module{}, p: &fpeditor.Plugin{}}
	cat := editorCatalog(f.m, f.p)
	pr, err := project.Create(t.TempDir(), "FakeCheck", false)
	if err != nil {
		t.Fatal(err)
	}
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.Load(types, nil, assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	src := scene.New(types, nil, scene.Options{})
	cam := src.Create(src.Root(), "Camera")
	*scene.Add[render2d.Camera2D](src, cam) = render2d.DefaultCamera2D()
	build(src)
	src.Flush()
	rel := "scenes/main.aescene"
	if err := os.WriteFile(filepath.Join(pr.Root, filepath.FromSlash(rel)), sceneio.Save(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(pr, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop(), Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	ctx := s.App.Context()
	if f.doc, err = ctx.Docs.Open(rel); err != nil {
		t.Fatal(err)
	}
	td, ok := ctx.Types.Schema().ByGoType(reflect.TypeFor[fakephys.Body]())
	if !ok {
		t.Fatal("fakephys.Body is not registered")
	}
	f.s, f.body = s, td
	return f
}

func (f *editorFixture) node(name string) ids.NodeID {
	s := f.doc.Scene()
	return s.ID(s.Find(s.Root(), name))
}

func (f *editorFixture) bodyOf(t *testing.T, name string) fakephys.Body {
	t.Helper()
	s := f.doc.Scene()
	b, ok := scene.Get[fakephys.Body](s, s.Find(s.Root(), name))
	if !ok {
		t.Fatalf("%s has no fakephys.Body", name)
	}
	return *b
}

func TestFakeModuleReachesEditor(t *testing.T) {
	f := openEditor(t, func(*scene.Scene) {})
	ctx := f.s.App.Context()
	if _, ok := ctx.Sections.Desc(fakephys.ID); !ok {
		t.Fatal("project section is not loaded")
	}
	if !slices.ContainsFunc(ctx.Pages, func(p context.SettingsPage) bool { return p.Section == fakephys.ID }) {
		t.Fatal("settings page is not registered")
	}
	for _, id := range []string{fpeditor.CmdReset, vectors.Prefix + fpeditor.LayerImpulse} {
		if _, ok := ctx.Commands.Get(id); !ok {
			t.Fatalf("command %s is missing", id)
		}
	}
	if got := ctx.Sections.Names(fakephys.ID, fakephys.Lists); len(got) != 2 {
		t.Fatalf("names hint %v", got)
	}
	if _, ok := f.s.App.Templates().Get(fakephys.ID + ".body"); !ok {
		t.Fatal("template is not in the Create menu")
	}
	if _, ok := ctx.Types.Schema().ByName("physics.Body2D"); ok {
		t.Fatal("physics is still in the catalog")
	}
	if !f.s.App.Workspace().Has(fpeditor.PanelID) {
		t.Fatal("panel is not registered")
	}
	if _, ok := f.s.App.Extensions().Inspectors().Custom(f.body.ID); !ok {
		t.Fatal("custom inspector is not registered")
	}
	if d := f.s.App.Extensions().Disabled(); len(d) != 0 {
		t.Fatalf("extensions disabled: %v", d)
	}
	if mask, _ := f.body.Field("mask"); mask.Hints.Bitset.Section != fakephys.ID {
		t.Fatal("bitset hint is lost")
	}
	if speed, _ := f.body.Field("speed"); !speed.Hints.Runtime {
		t.Fatal("runtime hint is lost")
	}
}

func TestFakeComponentHooks(t *testing.T) {
	f := openEditor(t, func(s *scene.Scene) {
		n := s.Create(s.Root(), "Crate")
		sr := scene.Add[render2d.SpriteRenderer](s, n)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Size = mathx.Vec2{X: 2, Y: 3}
	})
	ctx := f.s.App.Context()
	d := f.doc
	crate := f.node("Crate")
	add := &commands.AddComponent{Node: crate, Type: f.body.ID, Name: f.body.FullName, OnAdd: ctx.OnAdd(f.body)}
	if err := d.History.Exec(add); err != nil {
		t.Fatal(err)
	}
	if b := f.bodyOf(t, "Crate"); b.Size != 2 {
		t.Fatalf("OnAdd did not fit the sprite, size %v", b.Size)
	}
	if f.m.Changed != 1 {
		t.Fatalf("OnChanged after add ran %d times", f.m.Changed)
	}
	size, _ := f.body.Field("size")
	if err := d.Props.Set(nil, []ids.NodeID{crate}, f.body.ID, size.ID, schema.Float32(5)); err != nil {
		t.Fatal(err)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if f.m.Changed != 3 {
		t.Fatalf("OnChanged after set and undo ran %d times, want 3", f.m.Changed)
	}
	if err := d.History.Exec(&commands.RemoveComponent{Node: crate, Type: f.body.ID, Name: f.body.FullName}); err != nil {
		t.Fatal(err)
	}
	if f.m.Removed != 1 || f.m.LastRemoved.Size != 2 {
		t.Fatalf("OnRemoving %d times, last %+v", f.m.Removed, f.m.LastRemoved)
	}
}

func TestFakeDiagnosticGatesPlay(t *testing.T) {
	f := openEditor(t, func(s *scene.Scene) {
		n := s.Create(s.Root(), "Bad")
		*scene.Add[fakephys.Body](s, n) = fakephys.Body{}
	})
	pc := f.s.App.Play()
	err := pc.Start(f.doc)
	if err == nil || !strings.Contains(err.Error(), "size must be positive") {
		t.Fatalf("Play was not blocked: %v", err)
	}
	size, _ := f.body.Field("size")
	if err := f.doc.Props.Set(nil, []ids.NodeID{f.node("Bad")}, f.body.ID, size.ID, schema.Float32(1)); err != nil {
		t.Fatal(err)
	}
	if err := pc.Start(f.doc); err != nil {
		t.Fatalf("fixed scene still blocked: %v", err)
	}
	for i := 0; i < 10; i++ {
		pc.Tick(1.0 / 60)
	}
	st := fakephys.StateOf(pc.World())
	if f.m.Attached != 1 || st == nil || st.Ticks == 0 {
		t.Fatalf("play world: attached %d state %+v", f.m.Attached, st)
	}
	pc.Stop()
	if f.m.Attached != 0 {
		t.Fatal("Stop did not detach the play world")
	}
}

func drawProfiler(f *editorFixture, prof *profiler.Panel) {
	ctx := f.s.App.Context()
	ui := ctx.UI
	full := mathx.Rect{W: 900, H: 4000}
	ui.Begin(&input.State{}, mathx.Vec2{X: full.W, Y: full.H}, 1)
	ui.Region(full)
	prof.Draw(&services.Frame{UI: ui}, full)
	ui.PopCursor()
	ui.End()
}

func TestFakeEditorExtensionsRun(t *testing.T) {
	f := openEditor(t, func(s *scene.Scene) {
		n := s.Create(s.Root(), "Good")
		*scene.Add[fakephys.Body](s, n) = fakephys.Body{Size: 1}
	})
	ctx := f.s.App.Context()
	ctx.Overlay.Set(gameview.CommandOverlays, true)
	ctx.Overlay.Set(vectors.CommandToggle, true)
	pc := f.s.App.Play()
	if err := pc.Start(f.doc); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		pc.Tick(1.0 / 60)
	}
	e := f.s.Engine
	pool := rt.New(e.GPU(), gpu.TexSRGBA8, "test.fakephys")
	target, err := pool.Resize(nil, 320, 180, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Release(target)
		pool.Close()
	}()
	frame := e.GPU().BeginFrame(320, 180)
	if err := f.s.Renderer.BeginFrame(frame, 320, 180, 1); err != nil {
		t.Fatal(err)
	}
	pc.Render(target)
	gameview.Overlays(f.s.App.Viewport(), pc)
	f.s.Renderer.EndFrame()
	e.GPU().EndFrame()
	if f.p.Overlays == 0 || f.p.Vectors == 0 {
		t.Fatalf("overlay ran %d times, vectors %d", f.p.Overlays, f.p.Vectors)
	}
	for i := 0; i < 3; i++ {
		f.s.Step()
	}
	prof := f.s.App.Profiler()
	drawProfiler(f, prof)
	if !slices.Contains(prof.Sections(), "Fake") || f.p.Profiled == 0 {
		t.Fatalf("profiler sections %v", prof.Sections())
	}
	if err := ctx.Commands.Run(fpeditor.CmdReset); err != nil || f.p.Resets != 1 {
		t.Fatalf("command: %v %d", err, f.p.Resets)
	}
	pc.Stop()
}

func TestNoCodeOutsideFakeModule(t *testing.T) {
	root := repoRoot(t)
	own := filepath.Join(root, "tests", "modulecheck") + string(filepath.Separator)
	filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if n := e.Name(); p != root && (strings.HasPrefix(n, ".") || n == "third_party" || n == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasPrefix(p, own) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "fakephys") {
			t.Errorf("%s mentions the fake module", p)
		}
		return nil
	})
}
