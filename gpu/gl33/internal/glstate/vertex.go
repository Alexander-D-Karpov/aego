package glstate

const MaxVertexSlots = 4

type Key struct {
	Index  uint32
	Gen    uint32
	Offset uint32
}

func (k Key) IsZero() bool {
	return k.Gen == 0
}

func (b *VertexBinding) BindSlot(api VertexAPI, slot int, key Key, id uint32, base int, attrs []Attrib) bool {
	if slot < 0 || slot >= MaxVertexSlots || key.IsZero() || id == 0 {
		return false
	}
	if b.slots[slot] == key {
		return true
	}
	api.BindArrayBuffer(id)
	for i := range attrs {
		if attrs[i].Slot != slot {
			continue
		}
		a := attrs[i]
		a.Offset += base
		api.AttribPointer(a)
	}
	b.slots[slot] = key
	return true
}

type Attrib struct {
	Location   uint32
	Slot       int
	Size       int32
	Type       uint32
	Normalized bool
	Integer    bool
	Stride     int32
	Offset     int
}

type VertexAPI interface {
	BindArrayBuffer(id uint32)
	BindElementBuffer(id uint32)
	AttribPointer(a Attrib)
}

type VertexBinding struct {
	slots [MaxVertexSlots]Key
	index Key
}

func (b *VertexBinding) Invalidate() {
	*b = VertexBinding{}
}

func (b *VertexBinding) BindIndex(api VertexAPI, key Key, id uint32) bool {
	if key.IsZero() || id == 0 {
		return false
	}
	if b.index == key {
		return true
	}
	api.BindElementBuffer(id)
	b.index = key
	return true
}
