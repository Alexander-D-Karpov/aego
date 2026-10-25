package animedit_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/modules/render2d"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
)

const clipRel = "assets/anim/hero.aeanim"

const clipSrc = `anim version=1 length=1 loop=loop {
  track "" prop="SpriteRenderer.tint" kind=color {
    key t=0 v=[1, 0, 0, 1] interp=linear {}
    key t=1 v=[0, 0, 1, 1] {}
  }
  track "" prop="transform.position" kind=vec2 {
    key t=0 v=[0, 0] interp=linear {}
    key t=1 v=[2, 0] {}
  }
  event t=0.5 name="Mark" mode=preview {}
  event t=0.5 name="Hit" payload=1 {}
}
`

type memFiles map[string][]byte

func (m memFiles) Read(rel string) ([]byte, error) {
	b, ok := m[rel]
	if !ok {
		return nil, os.ErrNotExist
	}
	return bytes.Clone(b), nil
}

func (m memFiles) Write(rel string, data []byte) error {
	m[rel] = bytes.Clone(data)
	return nil
}

type rig struct {
	t     *testing.T
	root  string
	m     *document.Manager
	d     *document.SceneDocument
	s     *scene.Scene
	hero  scene.NodeHandle
	files memFiles
	reg   *animedit.Registry
	svc   *animedit.Service
	td    *schema.TypeDescriptor
}

func newRig(t *testing.T) *rig {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m := document.NewManager(document.Options{Root: root, Types: types})
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "Hero")
	*scene.Add[render2d.SpriteRenderer](s, h) = render2d.DefaultSpriteRenderer()
	s.Flush()
	d := m.New("main.aescene", s)
	files := memFiles{clipRel: []byte(clipSrc)}
	reg := animedit.NewRegistry(files, nil)
	m.SetSaveGuard(reg.SaveGuard)
	svc := reg.For(d)
	if err := svc.SetClip(clipRel); err != nil {
		t.Fatal(err)
	}
	svc.SetTarget(s.ID(h))
	td, _ := s.Registry().ByName("engine.SpriteRenderer")
	return &rig{t: t, root: root, m: m, d: d, s: s, hero: h, files: files, reg: reg, svc: svc, td: td}
}

func (r *rig) tint() mathx.Color {
	sr, _ := scene.Get[render2d.SpriteRenderer](r.s, r.hero)
	return sr.Tint
}

func (r *rig) set(field string, v schema.Value) {
	r.t.Helper()
	fd, _ := r.td.Field(field)
	if err := r.d.Props.Set(nil, []ids.NodeID{r.s.ID(r.hero)}, r.td.ID, fd.ID, v); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) clip() *mutate.AnimDocument {
	r.t.Helper()
	d, err := mutate.LoadAnim(r.files[clipRel], clipRel)
	if err != nil {
		r.t.Fatal(err)
	}
	return d
}

func near(a, b float32) bool {
	d := a - b
	return d < 1e-4 && d > -1e-4
}

func TestPreviewLeavesTheSceneAndTheSaveAlone(t *testing.T) {
	r := newRig(t)
	orig := r.tint()
	before := sceneio.Save(r.s)
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.5)
	if c := r.tint(); !near(c.R, 0.5) || !near(c.B, 0.5) {
		t.Fatalf("preview tint %+v", c)
	}
	if p, ok := r.s.PresentationLocal(r.hero); !ok || !near(p.Position.X, 1) || r.s.Local(r.hero).Position.X != 0 {
		t.Fatalf("pose %v %v, local %v", p, ok, r.s.Local(r.hero))
	}
	if err := r.m.Save(r.d.ID); err != nil {
		t.Fatal(err)
	}
	disk, _ := os.ReadFile(filepath.Join(r.root, "main.aescene"))
	if !bytes.Equal(disk, before) {
		t.Fatalf("saving during Preview wrote the sampled values:\n%s", disk)
	}
	if c := r.tint(); !near(c.R, 0.5) {
		t.Fatalf("Preview did not come back after the save: %+v", c)
	}
	if !r.svc.Driven(r.s.ID(r.hero), r.td.ID, mustField(r.td, "tint")) || r.svc.Driven(r.s.ID(r.hero), r.td.ID, mustField(r.td, "order")) {
		t.Fatal("Driven must report the tint and only the tint")
	}
	r.svc.SetMode(animedit.Normal)
	if r.tint() != orig {
		t.Fatalf("tint after Preview %+v, want %+v", r.tint(), orig)
	}
	if _, ok := r.s.PresentationLocal(r.hero); ok {
		t.Fatal("Preview left its pose")
	}
}

func mustField(td *schema.TypeDescriptor, name string) schema.FieldID {
	fd, _ := td.Field(name)
	return fd.ID
}

func TestPreviewDropsOnlyItsOwnPose(t *testing.T) {
	r := newRig(t)
	other := r.s.Create(r.s.Root(), "Body")
	r.s.Flush()
	one := mathx.Vec2{X: 1, Y: 1}
	physics := scene.Transform2D{Position: mathx.Vec2{X: 9}, Scale: one}
	r.s.SetPresentationLocal(other, physics)
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.25)
	r.s.SetPresentationLocal(r.hero, physics)
	r.svc.SetMode(animedit.Normal)
	if p, ok := r.s.PresentationLocal(r.hero); !ok || p != physics {
		t.Fatalf("a pose written after Preview's was dropped: %v %v", p, ok)
	}
	if p, ok := r.s.PresentationLocal(other); !ok || p != physics {
		t.Fatal("Preview dropped a pose it never wrote")
	}
}

func TestRecordKeysTheClipNotTheScene(t *testing.T) {
	r := newRig(t)
	orig := r.tint()
	r.svc.SetMode(animedit.Record)
	r.svc.SetTime(0.25)
	r.set("tint", schema.Color(mathx.Color{G: 1, A: 1}))
	c, err := r.clip().Clip()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tracks) != 2 {
		t.Fatalf("recording added a track: %d", len(c.Tracks))
	}
	k := c.Tracks[0].Keys
	if len(k) != 3 || !near(k[1].T, 0.25) || k[1].V.L != [4]float32{0, 1, 0, 1} {
		t.Fatalf("keys %+v", k)
	}
	r.d.History.Walk(func(cmd history.Command) {
		if _, ok := cmd.(*commands.Mutation); ok {
			t.Fatal("Record wrote a scene mutation")
		}
	})
	if err := r.m.Save(r.d.ID); err != nil {
		t.Fatal(err)
	}
	r.svc.SetMode(animedit.Normal)
	if r.tint() != orig {
		t.Fatalf("Record changed the scene: %+v", r.tint())
	}
	if err := r.d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if string(r.files[clipRel]) != clipSrc {
		t.Fatalf("undo did not restore the clip:\n%s", r.files[clipRel])
	}
	if err := r.d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.clip().Clip(); len(c.Tracks[0].Keys) != 3 {
		t.Fatal("redo did not put the key back")
	}
}

func TestRecordTransformAndAutoKeyOff(t *testing.T) {
	r := newRig(t)
	r.svc.SetMode(animedit.Record)
	r.svc.SetTime(0.5)
	if err := r.d.Props.EditTransform(nil, []ids.NodeID{r.s.ID(r.hero)}, func(tr *scene.Transform2D) { tr.Position.X = 5 }); err != nil {
		t.Fatal(err)
	}
	c, _ := r.clip().Clip()
	if k := c.Tracks[1].Keys; len(k) != 3 || k[1].V.L[0] != 5 {
		t.Fatalf("position keys %+v", k)
	}
	if r.s.Local(r.hero).Position.X != 0 {
		t.Fatal("Record moved the node")
	}
	snapshot := bytes.Clone(r.files[clipRel])
	steps := r.d.History.Cursor()
	r.svc.AutoKey = false
	r.set("tint", schema.Color(mathx.Color{R: 1, G: 1, A: 1}))
	if !bytes.Equal(r.files[clipRel], snapshot) || r.d.History.Cursor() != steps {
		t.Fatal("an edit with Auto Key off wrote a key")
	}
	fd, _ := r.td.Field("tint")
	if err := r.svc.Key(r.s.ID(r.hero), "SpriteRenderer.tint", fd.Kind, schema.Color(mathx.Color{R: 1, G: 1, A: 1})); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(r.files[clipRel], snapshot) {
		t.Fatal("Key did not write a key")
	}
}

func TestHistoryDuringPreviewSeesRealValues(t *testing.T) {
	r := newRig(t)
	orig := r.tint()
	r.set("tint", schema.Color(mathx.Color{R: 0.5, G: 0.5, B: 0.5, A: 1}))
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.5)
	r.set("order", schema.Int32(7))
	if err := r.d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := r.d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if c := r.tint(); !near(c.R, 0.5) || !near(c.B, 0.5) || !near(c.G, 0) {
		t.Fatalf("Preview lost its sample after Undo: %+v", c)
	}
	r.svc.SetMode(animedit.Normal)
	if r.tint() != orig {
		t.Fatalf("an Undo during Preview was lost: %+v, want %+v", r.tint(), orig)
	}
}

func TestPreviewDeliversOnlyPreviewEvents(t *testing.T) {
	r := newRig(t)
	r.svc.SetMode(animedit.Preview)
	r.svc.SetTime(0.4)
	r.svc.Play()
	r.svc.Tick(0.2)
	marks := r.svc.TakeMarks(nil)
	if len(marks) != 1 || marks[0].Name != "Mark" {
		t.Fatalf("marks %+v", marks)
	}
	r.svc.SetTime(0.9)
	r.svc.SetTime(0.1)
	if len(r.svc.TakeMarks(nil)) != 0 {
		t.Fatal("scrubbing fired events")
	}
}
