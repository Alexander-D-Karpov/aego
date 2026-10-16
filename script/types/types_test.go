package types

import "testing"

func TestLayoutAndHeapSlots(t *testing.T) {
	u := NewInterner()
	inner := NewStruct("game/a", "Inner")
	inner.Fields = []*Field{{Name: "Name", Type: String}, {Name: "T", Type: Transform2D}}
	outer := NewStruct("game/a", "Outer")
	outer.Fields = []*Field{{Name: "Hp", Type: Int}, {Name: "In", Type: inner}, {Name: "Tags", Type: u.SliceOf(String)}}
	if !outer.Layout() || outer.Slots() != 6 {
		t.Fatalf("outer slots %d", outer.Slots())
	}
	var heap []int
	outer.HeapSlots(0, func(s int) { heap = append(heap, s) })
	if len(heap) != 2 || heap[0] != 1 || heap[1] != 5 {
		t.Fatalf("heap slots %v", heap)
	}
	loop := NewStruct("game/a", "Loop")
	loop.Fields = []*Field{{Name: "Self", Type: loop}}
	if loop.Layout() {
		t.Fatal("recursive struct must not lay out")
	}
}

func TestAssignableAndRepresent(t *testing.T) {
	u := NewInterner()
	if !Assignable(Nil, NodeRef) || Assignable(Nil, Int) || !Assignable(UntypedInt, Float) || Assignable(UntypedFloat, Int) {
		t.Fatal("assignability")
	}
	s1, s2 := u.SliceOf(Int), u.SliceOf(Int)
	r1, r2 := u.AssetRef("texture"), u.AssetRef("texture")
	if s1 != s2 || r1 != r2 {
		t.Fatal("interning")
	}
	if _, why := Represent(FloatConst(1.5), Int); why == "" {
		t.Fatal("1.5 must not become int")
	}
	if c, why := Represent(FloatConst(2), Int); why != "" || c.Int != 2 {
		t.Fatal("2.0 must become int 2")
	}
	if _, why := Represent(FloatConst(1e39), Float); why == "" {
		t.Fatal("float32 overflow not detected")
	}
}
