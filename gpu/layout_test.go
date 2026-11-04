package gpu

import "testing"

func TestCompleteAttributeBudgetAndTypedStride(t *testing.T) {
	l := VertexLayout{Strides: [MaxVertexSlots]uint16{64, 64}}
	for i := 0; i < 8; i++ {
		l.Attrs = append(l.Attrs, VertexAttr{Location: uint8(i), Slot: uint8(i / 4), Format: VFUInt4, Offset: uint16((i % 4) * 16)})
	}
	if err := ValidateVertexLayout(l, Capabilities{MaxVertexAttributes: 8}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVertexLayout(l, Capabilities{MaxVertexAttributes: 7}); err == nil {
		t.Fatal("second buffer bypassed total attribute budget")
	}
	l.Attrs[7].Location = 0
	if err := ValidateVertexLayout(l, Capabilities{MaxVertexAttributes: 16}); err == nil {
		t.Fatal("duplicate location accepted")
	}
	l.Attrs[7].Location = 7
	l.Attrs[7].Offset = 60
	if err := ValidateVertexLayout(l, Capabilities{MaxVertexAttributes: 16}); err == nil {
		t.Fatal("uint4 overflow accepted")
	}
	if VFUInt4.Size() != 16 || VFUInt4.Components() != 4 {
		t.Fatal("integer layout width")
	}
}
