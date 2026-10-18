package settings

import (
	"errors"
	"strings"
	"testing"

	"aego/format/aetext"
	"aego/schema"
)

func reg(t *testing.T, s *Store, d Desc) Key {
	t.Helper()
	k, err := s.Register(d)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func volume(owner string) Desc {
	return Desc{ID: owner + ".music", Owner: owner, Kind: schema.KindFloat64, Default: schema.Float64(1), Flags: User | Project, Min: 0, Max: 1, HasMin: true, HasMax: true}
}

func TestOwnerPrefix(t *testing.T) {
	if _, err := New().Register(Desc{ID: "other.x", Owner: "audio", Kind: schema.KindBool}); err == nil {
		t.Fatal("a setting outside its owner was accepted")
	}
}

func TestLayers(t *testing.T) {
	s := New()
	k := reg(t, s, volume("audio"))
	s.Freeze()
	s.LoadProject(&aetext.Node{Head: ProjectHead, Blocks: []*aetext.Node{{Head: "audio", Fields: []aetext.Field{{Name: "music", Value: aetext.Float64(0.8)}}}}}, "project")
	if s.Float(k) != 0.8 || s.Source(k) != LayerProject {
		t.Fatalf("project layer: %v %v", s.Float(k), s.Source(k))
	}
	s.Start()
	tx := s.Begin()
	if err := tx.Set(k, schema.Float64(0.5)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := s.Override(k, schema.Float64(0.1)); err != nil {
		t.Fatal(err)
	}
	if s.Float(k) != 0.1 || s.Source(k) != LayerSession {
		t.Fatal("session layer does not win")
	}
	if strings.Contains(string(s.EncodeProfile()), "0.1") {
		t.Fatal("a session override reached the profile")
	}
	_ = s.ClearOverride(k)
	if s.Float(k) != 0.5 {
		t.Fatal("clearing the override did not return the user value")
	}
}

func TestRollback(t *testing.T) {
	s := New()
	var got []float64
	a := volume("audio")
	a.Apply = func(v schema.Value) error {
		got = append(got, v.F)
		return nil
	}
	b := Desc{ID: "display.mode", Owner: "display", Kind: schema.KindInt64, Flags: User, Apply: func(v schema.Value) error {
		if v.I == 1 {
			return errors.New("refused")
		}
		return nil
	}}
	ka, kb := reg(t, s, a), reg(t, s, b)
	s.Freeze()
	s.Start()
	got = nil
	tx := s.Begin()
	_ = tx.Set(ka, schema.Float64(0.3))
	_ = tx.Set(kb, schema.Int64(1))
	if tx.Apply() == nil {
		t.Fatal("a failing apply was accepted")
	}
	if s.Float(ka) != 1 || s.Int(kb) != 0 {
		t.Fatal("the transaction was not rolled back")
	}
	if len(got) != 2 || got[1] != 1 {
		t.Fatalf("the first setting was not re-applied with its old value: %v", got)
	}
}

func TestConfirm(t *testing.T) {
	s := New()
	k := reg(t, s, volume("audio"))
	s.Freeze()
	s.Start()
	s.Tick(10)
	tx := s.Begin()
	_ = tx.Set(k, schema.Float64(0.2))
	if err := tx.ApplyWithConfirm(DefaultConfirm); err != nil {
		t.Fatal(err)
	}
	s.Tick(20)
	if s.Float(k) != 0.2 {
		t.Fatal("reverted too early")
	}
	s.Tick(26)
	if s.Float(k) != 1 {
		t.Fatal("not reverted without confirmation")
	}
}

func TestSupportsAndDependencies(t *testing.T) {
	s := New()
	mode := reg(t, s, Desc{ID: "display.mode", Owner: "display", Kind: schema.KindInt64, Flags: User})
	res := reg(t, s, Desc{ID: "display.res", Owner: "display", Kind: schema.KindString, Flags: User,
		Supports:    func(v schema.Value) bool { return v.S != "8000x8000" },
		EnabledWhen: func(s *Store) bool { return s.Int(mode) != 1 },
		ChoicesWhen: func(s *Store) []Choice {
			if s.Int(mode) == 2 {
				return []Choice{{Name: "a", Value: schema.String("1920x1080")}}
			}
			return nil
		}})
	s.Freeze()
	s.Start()
	if s.Begin().Set(res, schema.String("8000x8000")) == nil {
		t.Fatal("the platform validator did not reject")
	}
	_ = s.Override(mode, schema.Int64(1))
	if s.Enabled(res) {
		t.Fatal("EnabledWhen ignored")
	}
	_ = s.Override(mode, schema.Int64(2))
	if s.Begin().Set(res, schema.String("1280x720")) == nil {
		t.Fatal("ChoicesWhen ignored")
	}
}

func TestProfile(t *testing.T) {
	s := New()
	k := reg(t, s, volume("audio"))
	_ = s.OwnerVersion(OwnerVersion{Owner: "audio", Version: 2, Migrate: func(n *aetext.Node, from, to int) error {
		for i := range n.Fields {
			if n.Fields[i].Name == "bgm" {
				n.Fields[i].Name = "music"
			}
		}
		return nil
	}})
	s.Freeze()
	src := "profile version=1 {\n  audio {\n    bgm=0.25\n    future=1\n  }\n  modded {\n    x=1\n  }\n}\n"
	if err := s.LoadProfile([]byte(src), "p"); err != nil {
		t.Fatal(err)
	}
	if s.Float(k) != 0.25 {
		t.Fatal("migration was not applied")
	}
	out := string(s.EncodeProfile())
	for _, want := range []string{"audio version=2", "music=0.25", "future=1", "modded"} {
		if !strings.Contains(out, want) {
			t.Fatalf("profile lost %q:\n%s", want, out)
		}
	}
	b := New()
	reg(t, b, volume("audio"))
	b.Freeze()
	if b.LoadProfile([]byte("profile {"), "broken") == nil || len(b.Diagnostics()) == 0 {
		t.Fatal("a damaged profile gave no diagnostic")
	}
}

func TestNotifyOnceAndPreset(t *testing.T) {
	s := New()
	p := reg(t, s, Desc{ID: "display.preset", Owner: "display", Kind: schema.KindEnum, Flags: User, Preset: true, Default: schema.Enum(1),
		Choices: []Choice{{Name: "Low", Value: schema.Enum(0)}, {Name: "High", Value: schema.Enum(1)}, {Name: PresetCustom, Value: schema.Enum(2)}}})
	q := reg(t, s, Desc{ID: "display.quality", Owner: "display", Kind: schema.KindInt64, Flags: User, Default: schema.Int64(3),
		Presets: map[string]schema.Value{"Low": schema.Int64(1), "High": schema.Int64(3)}})
	s.Freeze()
	s.Start()
	calls := 0
	s.Subscribe(q, func(Key) { calls++ })
	tx := s.Begin()
	_ = tx.Set(p, schema.Enum(0))
	_ = tx.Apply()
	if s.Int(q) != 1 || calls != 1 {
		t.Fatalf("preset: quality %d, calls %d", s.Int(q), calls)
	}
	tx = s.Begin()
	_ = tx.Set(q, schema.Int64(2))
	_ = tx.Set(q, schema.Int64(2))
	_ = tx.Apply()
	if s.ChoiceName(p) != PresetCustom || calls != 2 {
		t.Fatalf("manual change: preset %s, calls %d", s.ChoiceName(p), calls)
	}
}

func TestReadAllocs(t *testing.T) {
	s := New()
	k := reg(t, s, volume("audio"))
	s.Freeze()
	if n := testing.AllocsPerRun(100, func() { _ = s.Float(k) }); n != 0 {
		t.Fatalf("reading a setting allocates %v", n)
	}
}
