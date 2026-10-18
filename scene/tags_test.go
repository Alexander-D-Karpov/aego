package scene

import (
	"testing"

	"aego/schema"
)

func tagScene(t *testing.T) *Scene {
	t.Helper()
	types := NewTypes(schema.NewRegistry())
	if err := RegisterComponent[Tags](types, schema.Name(TagsName)); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return New(types, nil, Options{})
}

func TestTagIndexAndPrefixes(t *testing.T) {
	s := tagScene(t)
	a := s.Create(s.Root(), "a")
	b := s.Create(s.Root(), "b")
	s.AddTag(a, "Enemy.Flying")
	s.AddTag(a, "Enemy.Boss")
	s.AddTag(b, "Enemy")
	s.Flush()
	s.UpdateTransforms()
	out := make([]NodeHandle, 8)
	if n := s.WithTag("Enemy", out); n != 2 || out[0] != a || out[1] != b {
		t.Fatalf("Enemy: %d %v", n, out[:n])
	}
	if n := s.WithTag("Enemy.Flying", out); n != 1 || out[0] != a {
		t.Fatal("full tag")
	}
	if !s.HasTag(a, "Enemy") || s.HasTag(b, "Enemy.Flying") || s.AddTag(a, "bad..tag") {
		t.Fatal("HasTag or validation")
	}
	s.RemoveTag(a, "Enemy.Flying")
	if n := s.WithTag("Enemy.Flying", out); n != 0 || !s.HasTag(a, "Enemy") {
		t.Fatal("removing one tag must keep the shared prefix")
	}
	s.SetEnabled(b, false)
	s.UpdateTransforms()
	if n := s.WithTag("Enemy", out); n != 1 || s.WithTagAll("Enemy", out) != 2 {
		t.Fatal("disabled nodes")
	}
	s.Destroy(a)
	if n := s.WithTagAll("Enemy", out); n != 1 || out[0] != b {
		t.Fatal("a destroyed node must leave the index")
	}
}

func TestTagsLoadedThroughSchemaWrites(t *testing.T) {
	s := tagScene(t)
	h := s.Create(s.Root(), "n")
	p, _ := s.AddByType(h, schema.HashName(TagsName))
	(*Tags)(p).Tags = []string{"Pickup"}
	s.TagsChanged(h)
	s.Flush()
	s.UpdateTransforms()
	out := make([]NodeHandle, 1)
	if s.WithTag("Pickup", out) != 1 {
		t.Fatal("a tag written directly and announced with TagsChanged is not indexed")
	}
}

func TestTagQueryAllocatesNothing(t *testing.T) {
	s := tagScene(t)
	for i := range 10000 {
		h := s.Create(s.Root(), "n")
		if i%3 == 0 {
			s.AddTag(h, "Group.A")
		}
	}
	s.Flush()
	s.UpdateTransforms()
	out := make([]NodeHandle, 4096)
	s.WithTag("Group", out)
	if n := testing.AllocsPerRun(50, func() { s.WithTag("Group", out) }); n != 0 {
		t.Fatalf("WithTag allocates %v", n)
	}
}
