package material

import (
	"encoding/binary"
	"math"
	"testing"

	"aego/render/shader"
)

func TestStd140PackingZeroPaddingAndIntegerWidths(t *testing.T) {
	sh := &Shader{Variants: []Variant{{Mask: 0}}, Schema: NewSchema([]shader.Param{
		{Name: "Two", Kind: shader.KindVec2, Offset: 0, Default: [4]float32{1, 2}},
		{Name: "Enabled", Kind: shader.KindBool, Offset: 8, Default: [4]float32{1}},
		{Name: "Four", Kind: shader.KindVec4, Offset: 16, Default: [4]float32{3, 4, 5, 6}},
		{Name: "Counter", Kind: shader.KindInt, Offset: 32, IntDefault: math.MaxInt32},
	}, 48, 0)}
	m, ok := New("packing", sh, 0, shader.BlendPremultiplied)
	if !ok || len(m.data) != 48 {
		t.Fatal("material block has the wrong size")
	}
	word := func(at int) uint32 { return binary.LittleEndian.Uint32(m.data[at:]) }
	for at, want := range map[int]uint32{0: math.Float32bits(1), 4: math.Float32bits(2), 8: 1, 16: math.Float32bits(3), 20: math.Float32bits(4), 24: math.Float32bits(5), 28: math.Float32bits(6), 32: math.MaxInt32} {
		if got := word(at); got != want {
			t.Fatalf("word at %d = %#x, expected %#x", at, got, want)
		}
	}
	checkPadding := func() {
		t.Helper()
		for _, span := range [][2]int{{12, 16}, {36, 48}} {
			for i := span[0]; i < span[1]; i++ {
				if m.data[i] != 0 {
					t.Fatalf("nonzero std140 padding at %d", i)
				}
			}
		}
	}
	checkPadding()
	if !m.SetInt("Counter", math.MinInt32) || !m.Set("Enabled", [4]float32{0}) {
		t.Fatal("typed scalar assignment failed")
	}
	if word(32) != 0x80000000 || word(8) != 0 {
		t.Fatal("int/bool did not occupy exactly one 32-bit word")
	}
	checkPadding()
}

func TestStd140Vec3WriteDoesNotOverwriteFollowingScalar(t *testing.T) {
	sh := &Shader{Variants: []Variant{{Mask: 0}}, Schema: NewSchema([]shader.Param{
		{Name: "Head", Kind: shader.KindFloat, Offset: 0, Default: [4]float32{1}},
		{Name: "Three", Kind: shader.KindVec3, Offset: 16},
		{Name: "Tail", Kind: shader.KindFloat, Offset: 28, Default: [4]float32{0.25}},
	}, 32, 0)}
	m, _ := New("vec3", sh, 0, shader.BlendPremultiplied)
	if !m.Set("Three", [4]float32{4, 5, 6, 999}) {
		t.Fatal("vec3 assignment failed")
	}
	if got, _ := m.Get("Tail"); got[0] != 0.25 {
		t.Fatalf("vec3 padding overwrote tail: %v", got)
	}
	for i := 4; i < 16; i++ {
		if m.data[i] != 0 {
			t.Fatalf("nonzero alignment padding at %d", i)
		}
	}
}
