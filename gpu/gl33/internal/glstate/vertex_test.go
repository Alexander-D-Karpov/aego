package glstate

import "testing"

type mockVertexAPI struct {
	arrayBinds   []uint32
	elementBinds []uint32
	attribs      int
}

func (m *mockVertexAPI) BindArrayBuffer(id uint32)   { m.arrayBinds = append(m.arrayBinds, id) }
func (m *mockVertexAPI) BindElementBuffer(id uint32) { m.elementBinds = append(m.elementBinds, id) }
func (m *mockVertexAPI) AttribPointer(Attrib)        { m.attribs++ }

func attrs() []Attrib {
	return []Attrib{
		{Location: 0, Slot: 0},
		{Location: 1, Slot: 0},
		{Location: 2, Slot: 1},
	}
}

func TestSlotBoundOncePerKey(t *testing.T) {
	m := &mockVertexAPI{}
	var b VertexBinding
	for i := 0; i < 3; i++ {
		if !b.BindSlot(m, 0, Key{Index: 5, Gen: 1}, 42, 0, attrs()) {
			t.Fatal("bind failed")
		}
	}
	if len(m.arrayBinds) != 1 || m.attribs != 2 {
		t.Fatalf("binds=%v attribs=%d", m.arrayBinds, m.attribs)
	}
}

func TestRecreatedBufferInvalidatesBinding(t *testing.T) {
	m := &mockVertexAPI{}
	var b VertexBinding
	b.BindSlot(m, 0, Key{Index: 5, Gen: 1}, 42, 0, attrs())
	b.BindSlot(m, 0, Key{Index: 5, Gen: 2}, 42, 0, attrs())
	if len(m.arrayBinds) != 2 {
		t.Fatalf("same slot index with a new generation must rebind: %v", m.arrayBinds)
	}
	if m.attribs != 4 {
		t.Fatalf("attribs = %d", m.attribs)
	}
}

func TestSlotsAreIndependent(t *testing.T) {
	m := &mockVertexAPI{}
	var b VertexBinding
	b.BindSlot(m, 0, Key{Index: 1, Gen: 1}, 10, 0, attrs())
	b.BindSlot(m, 1, Key{Index: 2, Gen: 1}, 20, 0, attrs())
	if len(m.arrayBinds) != 2 || m.attribs != 3 {
		t.Fatalf("binds=%v attribs=%d", m.arrayBinds, m.attribs)
	}
	m.attribs = 0
	b.BindSlot(m, 0, Key{Index: 1, Gen: 1}, 10, 0, attrs())
	if m.attribs != 0 {
		t.Fatal("cached slot was rebound")
	}
}

func TestIndexBinding(t *testing.T) {
	m := &mockVertexAPI{}
	var b VertexBinding
	b.BindIndex(m, Key{Index: 7, Gen: 3}, 70)
	b.BindIndex(m, Key{Index: 7, Gen: 3}, 70)
	b.BindIndex(m, Key{Index: 7, Gen: 4}, 70)
	if len(m.elementBinds) != 2 {
		t.Fatalf("element binds = %v", m.elementBinds)
	}
	if b.BindIndex(m, Key{}, 70) || b.BindIndex(m, Key{Index: 1, Gen: 1}, 0) {
		t.Fatal("invalid key or id accepted")
	}
}

func TestInvalidate(t *testing.T) {
	m := &mockVertexAPI{}
	var b VertexBinding
	b.BindSlot(m, 0, Key{Index: 1, Gen: 1}, 10, 0, attrs())
	b.BindIndex(m, Key{Index: 2, Gen: 1}, 20)
	b.Invalidate()
	b.BindSlot(m, 0, Key{Index: 1, Gen: 1}, 10, 0, attrs())
	b.BindIndex(m, Key{Index: 2, Gen: 1}, 20)
	if len(m.arrayBinds) != 2 || len(m.elementBinds) != 2 {
		t.Fatal("invalidate did not force rebind")
	}
}
