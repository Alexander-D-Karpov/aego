package sceneio

import (
	"testing"

	"aego/core/mathx"
)

func TestColorRoundTripIsLossless(t *testing.T) {
	for v := 0; v < 256; v++ {
		packed := uint32(v)<<24 | uint32(v)<<16 | uint32(v)<<8 | uint32(v)
		if got := packColor(unpackColor(packed)); got != packed {
			t.Fatalf("byte %d: %#08x -> %#08x", v, packed, got)
		}
	}
}

func TestColorIsLinearized(t *testing.T) {
	c := unpackColor(0x0a0d12ff)
	if c.R > 0.01 || c.G > 0.01 || c.B > 0.01 {
		t.Fatalf("#0a0d12 must decode to near-black linear, got %+v", c)
	}
	if c.A != 1 {
		t.Fatalf("alpha = %v", c.A)
	}
	mid := unpackColor(0x808080ff)
	if mathx.Abs(mid.R-0.2158) > 0.001 {
		t.Fatalf("sRGB 0x80 must decode to ~0.216 linear, got %v", mid.R)
	}
}
