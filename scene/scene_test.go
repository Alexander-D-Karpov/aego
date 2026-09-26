package scene

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/schema"
)

type Health struct {
	Max     float32 `aego:"max"`
	Current float32 `aego:"current"`
}

type Velocity struct {
	Value mathx.Vec2 `aego:"value"`
}

type Tag struct {
	Name string `aego:"name"`
}

func newTypes(tb testing.TB) *Types {
	tb.Helper()
	types := NewTypes(schema.NewRegistry())
	for _, fn := range []func() error{
		func() error { return RegisterComponent[Health](types, schema.Name("test.Health")) },
		func() error { return RegisterComponent[Velocity](types, schema.Name("test.Velocity")) },
		func() error { return RegisterComponent[Tag](types, schema.Name("test.Tag")) },
	} {
		if err := fn(); err != nil {
			tb.Fatal(err)
		}
	}
	if err := types.Schema().Freeze(); err != nil {
		tb.Fatal(err)
	}
	return types
}

func newScene(tb testing.TB) *Scene {
	tb.Helper()
	return New(newTypes(tb), nil, Options{})
}

func TestCreateAndHierarchy(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(a, "B")
	c := s.Create(a, "C")
	s.Flush()

	if s.Parent(b) != a || s.Parent(a) != s.Root() {
		t.Fatal("parent links wrong")
	}
	if s.ChildCount(a) != 2 {
		t.Fatalf("child count = %d", s.ChildCount(a))
	}
	if s.FirstChild(a) != b || s.NextSibling(b) != c || s.PrevSibling(c) != b {
		t.Fatal("sibling links wrong")
	}
	if s.NextSibling(c) != (NodeHandle{}) {
		t.Fatal("last sibling must have no next")
	}
	var seen []string
	s.Children(a, func(h NodeHandle) bool {
		seen = append(seen, s.Name(h))
		return true
	})
	if len(seen) != 2 || seen[0] != "B" || seen[1] != "C" {
		t.Fatalf("children order = %v", seen)
	}
}

func TestCreateOrderIsInsertionOrder(t *testing.T) {
	s := newScene(t)
	p := s.Create(s.Root(), "P")
	for _, name := range []string{"one", "two", "three"} {
		s.Create(p, name)
	}
	s.Flush()
	var order []string
	s.Children(p, func(h NodeHandle) bool {
		order = append(order, s.Name(h))
		return true
	})
	if len(order) != 3 || order[0] != "one" || order[2] != "three" {
		t.Fatalf("order = %v", order)
	}
}

func TestPreOrderParentsFirst(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(a, "B")
	c := s.Create(b, "C")
	d := s.Create(s.Root(), "D")
	s.Flush()

	order := s.PreOrder()
	pos := map[NodeHandle]int{}
	for i, h := range order {
		pos[h] = i
	}
	for _, pair := range [][2]NodeHandle{{s.Root(), a}, {a, b}, {b, c}, {s.Root(), d}} {
		if pos[pair[0]] >= pos[pair[1]] {
			t.Fatalf("%s must come before %s", s.Name(pair[0]), s.Name(pair[1]))
		}
	}
	if len(order) != 5 {
		t.Fatalf("order length = %d", len(order))
	}
}

func TestDestroyIsDeferredButLogicallyImmediate(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(a, "B")
	s.Flush()

	s.Destroy(a)
	if s.IsAlive(a) || s.IsAlive(b) {
		t.Fatal("destroy must be logically immediate for the whole subtree")
	}
	if s.ChildCount(s.Root()) != 1 {
		t.Fatal("structure must not change before Flush")
	}
	if s.Pending() == 0 {
		t.Fatal("destroy must queue a structural op")
	}
	s.Flush()
	if s.ChildCount(s.Root()) != 0 {
		t.Fatalf("subtree not detached: %d children", s.ChildCount(s.Root()))
	}
	if !s.Handle(s.ID(a)).IsZero() {
		t.Fatal("id index must drop destroyed nodes")
	}
}

func TestDestroyIsIdempotent(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()
	s.Destroy(a)
	before := s.Pending()
	s.Destroy(a)
	if s.Pending() != before {
		t.Fatal("second destroy must not queue another op")
	}
	s.Flush()
	s.Destroy(a)
	s.Flush()
}

func TestRootCannotBeDestroyedOrReparented(t *testing.T) {
	s := newScene(t)
	s.Destroy(s.Root())
	s.Flush()
	if !s.IsAlive(s.Root()) {
		t.Fatal("root was destroyed")
	}
	if err := s.Reparent(s.Root(), s.Root(), 0); err == nil {
		t.Fatal("reparenting the root must fail")
	}
}

func TestReparent(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(s.Root(), "B")
	child := s.Create(a, "Child")
	s.Flush()

	if err := s.Reparent(child, b, -1); err != nil {
		t.Fatal(err)
	}
	if s.Parent(child) != a {
		t.Fatal("reparent must be deferred")
	}
	s.Flush()
	if s.Parent(child) != b || s.ChildCount(a) != 0 || s.ChildCount(b) != 1 {
		t.Fatal("reparent did not apply")
	}
	if s.Child(b, "Child") != child {
		t.Fatal("name index not moved with the node")
	}
	if !s.Child(a, "Child").IsZero() {
		t.Fatal("stale name index under the old parent")
	}
}

func TestReparentIndex(t *testing.T) {
	s := newScene(t)
	p := s.Create(s.Root(), "P")
	one := s.Create(p, "one")
	two := s.Create(p, "two")
	three := s.Create(p, "three")
	s.Flush()

	if err := s.Reparent(three, p, 0); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	var order []NodeHandle
	s.Children(p, func(h NodeHandle) bool {
		order = append(order, h)
		return true
	})
	if len(order) != 3 || order[0] != three || order[1] != one || order[2] != two {
		t.Fatal("reparent with index 0 must put the node first")
	}

	if err := s.Reparent(three, p, 1); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	order = order[:0]
	s.Children(p, func(h NodeHandle) bool {
		order = append(order, h)
		return true
	})
	if order[1] != three {
		t.Fatalf("reparent to index 1 failed: %v", names(s, order))
	}
}

func names(s *Scene, hs []NodeHandle) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = s.Name(h)
	}
	return out
}

func TestReparentCycleRejected(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(a, "B")
	c := s.Create(b, "C")
	s.Flush()

	if err := s.Reparent(a, c, -1); err == nil {
		t.Fatal("cycle must be rejected")
	}
	if err := s.Reparent(a, a, -1); err == nil {
		t.Fatal("self-parenting must be rejected")
	}
	if s.Pending() != 0 {
		t.Fatal("rejected reparent must not queue an op")
	}
}

func TestNamesAreUniqueAmongSiblings(t *testing.T) {
	s := newScene(t)
	p := s.Create(s.Root(), "P")
	other := s.Create(s.Root(), "Q")
	a := s.Create(p, "Item")
	b := s.Create(p, "Item")
	c := s.Create(p, "Item")
	elsewhere := s.Create(other, "Item")
	s.Flush()

	if s.Name(a) != "Item" || s.Name(b) != "Item_2" || s.Name(c) != "Item_3" {
		t.Fatalf("names = %v", names(s, []NodeHandle{a, b, c}))
	}
	if s.Name(elsewhere) != "Item" {
		t.Fatal("uniqueness must be per parent")
	}
	if s.Child(p, "Item_2") != b {
		t.Fatal("name index missing")
	}
	if got := s.SetName(b, "Item"); got != "Item_2" {
		t.Fatalf("renaming to a taken name returned %q", got)
	}
	if got := s.SetName(b, "Fresh"); got != "Fresh" || s.Child(p, "Fresh") != b {
		t.Fatalf("rename failed: %q", got)
	}
	if !s.Child(p, "Item_2").IsZero() {
		t.Fatal("old name still indexed")
	}
}

func TestFindAndPath(t *testing.T) {
	s := newScene(t)
	player := s.Create(s.Root(), "Player")
	weapon := s.Create(player, "Weapon")
	gun := s.Create(weapon, "Gun")
	s.Flush()

	cases := []struct {
		from NodeHandle
		path string
		want NodeHandle
	}{
		{s.Root(), "Player/Weapon/Gun", gun},
		{player, "Weapon/Gun", gun},
		{gun, "../..", player},
		{gun, "/Player", player},
		{gun, "/", s.Root()},
		{player, "", player},
		{player, "Missing", NodeHandle{}},
		{gun, "../../../..", NodeHandle{}},
	}
	for _, c := range cases {
		if got := s.Find(c.from, c.path); got != c.want {
			t.Fatalf("Find(%q) = %q, want %q", c.path, s.Name(got), s.Name(c.want))
		}
	}
	if p := s.Path(gun); p != "/Player/Weapon/Gun" {
		t.Fatalf("path = %q", p)
	}
	if p := s.Path(s.Root()); p != "/" {
		t.Fatalf("root path = %q", p)
	}
}

func TestCreateWithIDIsStable(t *testing.T) {
	s := newScene(t)
	id := ids.NewNodeID()
	a := s.CreateWithID(s.Root(), "A", id)
	s.Flush()
	if s.ID(a) != id || s.Handle(id) != a {
		t.Fatal("explicit id not honoured")
	}
	if again := s.CreateWithID(s.Root(), "B", id); again != a {
		t.Fatal("duplicate id must return the existing node")
	}
}
