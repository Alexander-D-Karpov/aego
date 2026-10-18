package savegame_test

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/mathx"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/modules/savegame"
	"aego/save"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/bind"
)

type stats struct {
	HP   float32 `aego:"hp"`
	Name string  `aego:"name"`
}

type mod struct{}

func (mod) Info() module.Info {
	return module.Info{ID: "savecase"}
}

func (mod) Register(r *module.Registry) error {
	return module.Component[stats](r, schema.Name("test.Stats"), schema.Persist("hp")).Err()
}

func types(t *testing.T) *scene.Types {
	t.Helper()
	tp, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Headless().Load(tp, bind.New(), assembly.Options{}, mod{}); err != nil {
		t.Fatal(err)
	}
	if err := tp.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return tp
}

func TestCaptureApplyRoundTrip(t *testing.T) {
	tp := types(t)
	s := scene.New(tp, nil, scene.Options{})
	a := s.Create(s.Root(), "A")
	st := scene.Add[stats](s, a)
	st.HP, st.Name = 3, "start"
	scene.Add[savegame.Persist](s, a)
	s.SetPosition(a, mathx.Vec2{X: 1, Y: 2})
	b := s.Create(s.Root(), "B")
	c := s.Create(s.Root(), "C")
	scene.Add[stats](s, c).HP = 9
	s.Flush()
	original := sceneio.Save(s)
	aID, bID, cID := s.ID(a), s.ID(b), s.ID(c)

	m := savegame.New()
	svc := savegame.NewService(m, nil, s)
	st, _ = scene.Get[stats](s, a)
	st.HP, st.Name = 7, "changed"
	s.SetPosition(a, mathx.Vec2{X: 5, Y: 6})
	s.SetEnabled(a, false)
	s.Destroy(b)
	s.Flush()
	m.SetGlobal("kills", aetext.Int(3))

	doc, err := svc.Capture("t")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := save.Decode(save.Encode(doc), "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(dec.Destroyed) != 1 || dec.Destroyed[0] != bID {
		t.Fatalf("destroyed = %v, want [%v]", dec.Destroyed, bID)
	}

	fresh, _, err := sceneio.Load(original, "t", tp, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	m2 := savegame.New()
	if err := savegame.NewService(m2, nil, fresh).Apply(dec); err != nil {
		t.Fatal(err)
	}
	fresh.Flush()
	fa := fresh.Handle(aID)
	got, ok := scene.Get[stats](fresh, fa)
	if !ok || got.HP != 7 || got.Name != "start" {
		t.Fatalf("A stats = %+v, want hp 7 and the scene name kept", got)
	}
	if p := fresh.Local(fa).Position; p != (mathx.Vec2{X: 5, Y: 6}) {
		t.Fatalf("A position = %v", p)
	}
	if fresh.Enabled(fa) {
		t.Fatal("A must come back disabled")
	}
	if h := fresh.Handle(bID); !h.IsZero() && fresh.IsAlive(h) {
		t.Fatal("B must be destroyed")
	}
	if cs, _ := scene.Get[stats](fresh, fresh.Handle(cID)); cs.HP != 9 {
		t.Fatalf("C keeps its value, got %v", cs.HP)
	}
	if v, ok := m2.Global("kills"); !ok || v.AsInt() != 3 {
		t.Fatalf("global kills = %v", v)
	}
}

func TestUnknownSectionKept(t *testing.T) {
	tp := types(t)
	s := scene.New(tp, nil, scene.Options{})
	m := savegame.New()
	svc := savegame.NewService(m, nil, s)
	doc := &save.Doc{Sections: []save.Section{{ID: "other.mod", Version: 2, Node: &aetext.Node{Fields: []aetext.Field{{Name: "x", Value: aetext.Int(4)}}}}}}
	if err := svc.Apply(doc); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Capture("t")
	if err != nil {
		t.Fatal(err)
	}
	sec, ok := out.Section("other.mod")
	if !ok || sec.Version != 2 {
		t.Fatalf("unknown section is lost: %+v", out.Sections)
	}
	if v, _ := sec.Node.Field("x"); v.AsInt() != 4 {
		t.Fatal("unknown section content changed")
	}
}
