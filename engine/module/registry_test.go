package module

import (
	"strings"
	"testing"
	"testing/fstest"

	"aego/core/errs"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

type probe struct {
	Speed  float32 `aego:"speed"`
	Hidden float32 `aego:"hidden,noscript"`
}

type dep struct {
	V int32 `aego:"v"`
}

type fake struct {
	info Info
	reg  func(*Registry) error
	log  *[]string
}

func (m fake) Info() Info {
	return m.info
}

func (m fake) Register(r *Registry) error {
	*m.log = append(*m.log, m.info.ID)
	if m.reg != nil {
		return m.reg(r)
	}
	return nil
}

func (m fake) Init(*InitContext) error {
	*m.log = append(*m.log, "init:"+m.info.ID)
	return nil
}

func newRegistry() *Registry {
	return New(scene.NewTypes(schema.NewRegistry()), bind.New())
}

func icons() fstest.MapFS {
	return fstest.MapFS{"spin@16x1.png": {Data: []byte{0}}, "spin@16x2.png": {Data: []byte{0}}}
}

func TestDuplicateID(t *testing.T) {
	var log []string
	r := newRegistry()
	if err := r.Add(fake{info: Info{ID: "a"}, log: &log}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(fake{info: Info{ID: "a"}, log: &log}); !errs.Has(err, errs.EngineModule) {
		t.Fatalf("duplicate accepted: %v", err)
	}
}

func TestMissingRequires(t *testing.T) {
	var log []string
	r := newRegistry()
	r.Add(fake{info: Info{ID: "a", Requires: []string{"ghost"}}, log: &log})
	_, err := r.Build()
	if err == nil || !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), "a requires") {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderFollowsRequires(t *testing.T) {
	var log []string
	r := newRegistry()
	r.Add(fake{info: Info{ID: "c"}, log: &log})
	r.Add(fake{info: Info{ID: "b", Requires: []string{"a"}}, log: &log})
	r.Add(fake{info: Info{ID: "a"}, log: &log})
	m, err := r.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Init(&InitContext{}); err != nil {
		t.Fatal(err)
	}
	want := "c,a,b,init:c,init:a,init:b"
	if got := strings.Join(log, ","); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestCycle(t *testing.T) {
	var log []string
	r := newRegistry()
	r.Add(fake{info: Info{ID: "a", Requires: []string{"b"}}, log: &log})
	r.Add(fake{info: Info{ID: "b", Requires: []string{"a"}}, log: &log})
	if _, err := r.Build(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuilderFillsDescriptor(t *testing.T) {
	var log []string
	var d *schema.TypeDescriptor
	r := newRegistry()
	r.Add(fake{info: Info{ID: "game", Builtin: true}, log: &log, reg: func(r *Registry) error {
		r.Icons(icons(), "game.")
		if err := Component[dep](r, schema.Name("game.Dep")).Err(); err != nil {
			return err
		}
		b := Component[probe](r, schema.Name("game.Probe")).
			Category("Game").
			Icon("game.spin").
			Default(func() probe { return probe{Speed: 3} }).
			Script("speed").
			Requires("game.Dep").
			SceneUnique().
			Template("", nil)
		d = b.Descriptor()
		return b.Err()
	}})
	if _, err := r.Build(); err != nil {
		t.Fatal(err)
	}
	if d.Owner != "game" || !d.Builtin || !d.SceneUnique || d.Category != "Game" || len(d.ScriptFields) != 1 {
		t.Fatalf("descriptor = %+v", d)
	}
	if d.Template == nil || d.Template.Name != "Probe" || d.Template.Category != "Game" || d.Template.Icon != "game.spin" {
		t.Fatalf("template = %+v", d.Template)
	}
	if err := r.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(r.Types(), nil, scene.Options{})
	h := s.Create(s.Root(), "N")
	d.Template.Setup.(func(*scene.Scene, scene.NodeHandle))(s, h)
	if p, ok := scene.Get[probe](s, h); !ok || p.Speed != 3 {
		t.Fatal("template did not add the component with its default")
	}
	if !scene.Has[dep](s, h) {
		t.Fatal("template did not add the required component")
	}
}

func TestUnknownIconAndRequires(t *testing.T) {
	var log []string
	r := newRegistry()
	r.Add(fake{info: Info{ID: "game"}, log: &log, reg: func(r *Registry) error {
		Component[probe](r, schema.Name("game.Probe")).Icon("game.nope").Requires("game.Missing")
		return nil
	}})
	_, err := r.Build()
	if err == nil || !strings.Contains(err.Error(), "game.nope") || !strings.Contains(err.Error(), "game.Missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestSystemRules(t *testing.T) {
	var log []string
	r := newRegistry()
	r.Add(fake{info: Info{ID: "game"}, log: &log, reg: func(r *Registry) error {
		r.System(engineDesc("other.update"))
		d := engineDesc("game.early")
		d.Phase = 0
		r.System(d)
		return nil
	}})
	_, err := r.Build()
	if err == nil || !strings.Contains(err.Error(), "other.update") || !strings.Contains(err.Error(), "game.early") {
		t.Fatalf("err = %v", err)
	}
}

func TestFinalizeSeesEveryModule(t *testing.T) {
	var log []string
	var seen int
	r := newRegistry()
	r.Add(fake{info: Info{ID: "a"}, log: &log, reg: func(r *Registry) error {
		r.Finalize(func(r *Registry) error {
			seen = len(r.Components())
			return nil
		})
		return nil
	}})
	r.Add(fake{info: Info{ID: "b"}, log: &log, reg: func(r *Registry) error {
		return Component[dep](r, schema.Name("b.Dep")).Err()
	}})
	if _, err := r.Build(); err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("finalizer saw %d components", seen)
	}
}

func TestTemplateIDsAreUnique(t *testing.T) {
	var log []string
	r := newRegistry()
	setup := func(*scene.Scene, scene.NodeHandle) {}
	r.Add(fake{info: Info{ID: "game"}, log: &log, reg: func(r *Registry) error {
		r.Template(schema.TemplateDesc{Name: "Enemy", Setup: setup})
		r.Template(schema.TemplateDesc{Name: "enemy", Setup: setup})
		return nil
	}})
	_, err := r.Build()
	if err == nil || !strings.Contains(err.Error(), "game.enemy") {
		t.Fatalf("err = %v", err)
	}
}

func TestDuplicateService(t *testing.T) {
	var log []string
	r := newRegistry()
	r.Add(fake{info: Info{ID: "a"}, log: &log, reg: func(r *Registry) error {
		r.Service("x", 1)
		return nil
	}})
	r.Add(fake{info: Info{ID: "b"}, log: &log, reg: func(r *Registry) error {
		r.Service("x", 2)
		return nil
	}})
	if _, err := r.Build(); err == nil || !strings.Contains(err.Error(), "already registered by a") {
		t.Fatalf("err = %v", err)
	}
}
