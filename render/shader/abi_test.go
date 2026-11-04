package shader

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
)

func TestStd140Offsets(t *testing.T) {
	kinds := []Kind{KindFloat, KindVec3, KindFloat, KindVec2, KindVec4, KindBool, KindInt}
	offs := make([]int, len(kinds))
	size := Std140(kinds, offs)
	if want := []int{0, 16, 28, 32, 48, 64, 68}; !slices.Equal(offs, want) {
		t.Fatalf("offsets %v, want %v", offs, want)
	}
	if size != 80 {
		t.Fatalf("size %d, want 80", size)
	}
}

func TestStd140RequiredMixedSequences(t *testing.T) {
	for _, tc := range []struct {
		kinds   []Kind
		offsets []int
		size    int
	}{
		{[]Kind{KindFloat, KindVec3, KindFloat}, []int{0, 16, 28}, 32},
		{[]Kind{KindVec2, KindBool, KindVec4}, []int{0, 8, 16}, 32},
		{[]Kind{KindBool, KindInt, KindFloat}, []int{0, 4, 8}, 16},
	} {
		offsets := make([]int, len(tc.kinds))
		if size := Std140(tc.kinds, offsets); size != tc.size || !slices.Equal(offsets, tc.offsets) {
			t.Fatalf("kinds %v: offsets=%v size=%d, expected %v size=%d", tc.kinds, offsets, size, tc.offsets, tc.size)
		}
	}
}

func f32(b []byte, at int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b[at:]))
}

func TestPutFrameAndView(t *testing.T) {
	frame := make([]byte, FrameSize)
	PutFrame(frame, 1.5, 0.25, 0.5, 7)
	if got := []float32{f32(frame, 0), f32(frame, 4), f32(frame, 8), f32(frame, 12)}; !slices.Equal(got, []float32{1.5, 0.25, 0.5, 7}) {
		t.Fatalf("frame block %v", got)
	}
	view := make([]byte, ViewSize)
	var vp [16]float32
	for i := range vp {
		vp[i] = float32(i + 1)
	}
	PutView(view, &vp, [4]float32{10, 20, 640, 360}, 2, 16, 4, [3]float32{3, 4, 5})
	for i := range vp {
		if f32(view, 4*i) != vp[i] {
			t.Fatalf("view_proj[%d] = %v", i, f32(view, 4*i))
		}
	}
	checks := map[int]float32{64: 10, 68: 20, 72: 640, 76: 360, 80: 2, 84: 16, 88: 4, 92: 1, 96: 3, 100: 4, 104: 5, 108: 0}
	for at, want := range checks {
		if got := f32(view, at); got != want {
			t.Errorf("view block at %d = %v, want %v", at, got, want)
		}
	}
	if ViewSize != 112 || FrameSize != 16 {
		t.Fatalf("block sizes %d and %d", ViewSize, FrameSize)
	}
}
