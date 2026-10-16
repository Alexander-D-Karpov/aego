package module_test

import (
	"strings"
	"testing"

	"aego/engine/base"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/script/bind"
)

type fakeSettings struct {
	N int64
}

type sectionMod struct {
	id      string
	version int
}

func (m sectionMod) Info() module.Info {
	return module.Info{ID: m.id}
}

func (m sectionMod) Register(r *module.Registry) error {
	r.ProjectSection(module.SectionDesc[fakeSettings]{
		ID:      m.id,
		Version: m.version,
		Default: func() fakeSettings { return fakeSettings{N: 7} },
		Decode: func(n *aetext.Node, v *fakeSettings) error {
			if f, ok := n.Field("n"); ok {
				v.N = f.AsInt()
			}
			return nil
		},
		Encode: func(v *fakeSettings, n *aetext.Node) {
			n.Fields = append(n.Fields, aetext.Field{Name: "n", Value: aetext.Int(v.N)})
		},
		Migrate: func(n *aetext.Node, from, to int) error {
			if f, ok := n.Field("count"); ok {
				n.RemoveField("count")
				n.SetField("n", f)
			}
			return nil
		},
	})
	return nil
}

func load(t *testing.T, m sectionMod, src string, reserved ...string) (*module.Sections, error) {
	t.Helper()
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, bind.New(), m)
	if err != nil {
		t.Fatal(err)
	}
	var blk *aetext.Node
	if src != "" {
		if blk, err = aetext.Parse([]byte(src), "test"); err != nil {
			t.Fatal(err)
		}
	}
	return man.LoadSections(func(head string) *aetext.Node {
		if blk != nil && blk.Head == head {
			return blk
		}
		return nil
	}, reserved...)
}

func TestSectionDefault(t *testing.T) {
	s, err := load(t, sectionMod{id: "fake"}, "")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := module.SectionValue[fakeSettings](s, "fake")
	if v.N != 7 {
		t.Fatalf("want default 7, got %d", v.N)
	}
	n, err := s.Node("fake", v)
	if err != nil || n != nil {
		t.Fatalf("default value should not be written, got %v %v", n, err)
	}
}

func TestSectionMigrates(t *testing.T) {
	s, err := load(t, sectionMod{id: "fake", version: 2}, "fake { count=3 }")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := module.SectionValue[fakeSettings](s, "fake")
	if v.N != 3 {
		t.Fatalf("migration lost the value, got %d", v.N)
	}
	n, _ := s.Node("fake", v)
	if a, ok := n.Attr(module.VersionAttr); !ok || a.AsInt() != 2 {
		t.Fatal("encoded section has no version 2")
	}
}

func TestSectionNewerVersion(t *testing.T) {
	_, err := load(t, sectionMod{id: "fake"}, "fake version=3 { n=1 }")
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("want a newer-version error, got %v", err)
	}
}

func TestSectionReservedHead(t *testing.T) {
	if _, err := load(t, sectionMod{id: "window"}, "", "window", "input"); err == nil {
		t.Fatal("a section named after a core block must fail")
	}
}

func TestDraftIsIndependent(t *testing.T) {
	s, err := load(t, sectionMod{id: "fake"}, "fake { n=4 }")
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Draft("fake")
	if err != nil {
		t.Fatal(err)
	}
	d.(*fakeSettings).N = 9
	v, _ := module.SectionValue[fakeSettings](s, "fake")
	if v.N != 4 {
		t.Fatal("editing a draft changed the live value")
	}
}
