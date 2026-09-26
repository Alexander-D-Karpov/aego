package sprite

import (
	"math/rand"
	"slices"
	"testing"
)

func TestKeyMonotonic(t *testing.T) {
	base := MakeKey(0, 0, 0.5, 0, 1, 0)
	if MakeKey(1, 0, 0.5, 0, 1, 0) <= base {
		t.Fatal("layer must dominate")
	}
	if MakeKey(0, 1, 0.5, 0, 1, 0) <= base {
		t.Fatal("order must follow layer")
	}
	if MakeKey(0, 0, 0.6, 0, 1, 0) <= base {
		t.Fatal("z must follow order")
	}
	if MakeKey(0, 0, 0.5, 0, 1, 1) <= base {
		t.Fatal("submission must follow z")
	}
	if MakeKey(-1, 100, 1, 0, 1, 0) >= MakeKey(0, -100, 0, 0, 1, 0) {
		t.Fatal("lower layer must sort first regardless of order")
	}
}

func TestKeyClamping(t *testing.T) {
	if MakeKey(30000, 0, 0, 0, 1, 0) != MakeKey(LayerMax, 0, 0, 0, 1, 0) {
		t.Fatal("layer not clamped")
	}
	if MakeKey(0, 0, -5, 0, 1, 0) != MakeKey(0, 0, 0, 0, 1, 0) {
		t.Fatal("z not clamped low")
	}
	if MakeKey(0, 0, 5, 0, 1, 0) != MakeKey(0, 0, 1, 0, 1, 0) {
		t.Fatal("z not clamped high")
	}
	if !OutOfRange(0, 0, 2, 0, 1) || OutOfRange(0, 0, 0.5, 0, 1) {
		t.Fatal("OutOfRange wrong")
	}
}

func TestKeyStableBySubmission(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	keys := make([]uint64, 0, 1000)
	for i := 0; i < 1000; i++ {
		keys = append(keys, uint64(MakeKey(int16(rng.Intn(4)), 0, 0.5, 0, 1, uint32(i))))
	}
	sorted := slices.Clone(keys)
	slices.Sort(sorted)
	prevLayer, prevSub := uint64(0), int64(-1)
	for _, k := range sorted {
		layer := k >> (orderBits + zBits + submitBits)
		sub := int64(SortKey(k).Submission())
		if layer == prevLayer && sub <= prevSub {
			t.Fatal("submission order not preserved within a layer")
		}
		if layer != prevLayer {
			prevLayer = layer
			prevSub = -1
			continue
		}
		prevSub = sub
	}
}
