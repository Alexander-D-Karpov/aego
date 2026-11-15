package vm

import (
	"math/rand"
	"testing"

	"aego/core/rewind"
)

type snapshotRoots []Value

func (r snapshotRoots) MarkRoots(m *Marker) {
	for _, v := range r {
		m.Mark(v)
	}
}

func TestKeyframeSliceCounterAcrossAllocationCollectionAndRestore(t *testing.T) {
	for _, seed := range []int64{1, 2, 2026} {
		rng := rand.New(rand.NewSource(seed))
		m := New(Options{})
		var roots snapshotRoots
		var saved State
		for step := 0; step < 500; step++ {
			switch rng.Intn(5) {
			case 0:
				roots = append(roots, m.NewString("snapshot"))
			case 1:
				v := m.heap.newSlice(1, 0, rng.Intn(5))
				o := m.heap.get(v.A)
				if cap(o.elems) > 0 {
					o.elems = append(o.elems, Int(int64(step)))
				}
				roots = append(roots, v)
			case 2:
				if len(roots) > 0 {
					roots = append(roots[:0], roots[len(roots)/2:]...)
				}
				if !m.Collect(roots) {
					t.Fatal("collection refused")
				}
			default:
				m.Capture(&saved)
				want := snapshotHash(m)
				m.heap.newSlice(1, 0, 2)
				m.NewString("after capture")
				if !m.Collect(nil) {
					t.Fatal("collection refused")
				}
				m.Restore(&saved)
				if snapshotHash(m) != want {
					t.Fatalf("seed %d step %d: restore changed heap", seed, step)
				}
			}
			live := 0
			for i := range m.heap.objs {
				o := &m.heap.objs[i]
				if o.live && !o.str {
					live++
				}
				if (!o.live || o.str) && o.elems != nil {
					t.Fatalf("seed %d step %d: unexpected slice in a string/dead object", seed, step)
				}
			}
			if m.heap.sliceCount != live {
				t.Fatalf("seed %d step %d: count %d, actual %d", seed, step, m.heap.sliceCount, live)
			}
		}
	}
}

func snapshotHash(m *Machine) [32]byte {
	h := rewind.NewHasher()
	m.Hash(h)
	return h.Sum()
}

func TestKeyframeAcrossStringAndSliceHeaps(t *testing.T) {
	m := New(Options{})
	word := m.NewString("kept")
	for i := 0; i < 40; i++ {
		m.NewString("temporary")
	}
	if m.heap.sliceCount != 0 {
		t.Fatal("strings counted as slices")
	}
	var onlyStrings State
	m.Capture(&onlyStrings)
	if onlyStrings.sliceCount != 0 || len(onlyStrings.lens) != 0 {
		t.Fatal("string-only snapshot did not use the compact path")
	}
	stringsHash := snapshotHash(m)
	values := m.heap.newSlice(1, 0, 4)
	obj := m.heap.get(values.A)
	obj.elems = append(obj.elems, Int(3), Int(7))
	empty := m.heap.newSlice(1, 0, 0)
	unused := m.heap.newSlice(1, 0, 3)
	if m.heap.sliceCount != 3 {
		t.Fatalf("live slices: %d", m.heap.sliceCount)
	}
	var withSlices State
	m.Capture(&withSlices)
	slicesHash := snapshotHash(m)
	obj.elems[0] = Int(99)
	if !m.Collect(snapshotRoots{word, values, empty}) {
		t.Fatal("collection refused")
	}
	if m.heap.sliceCount != 2 || m.heap.get(unused.A) != nil {
		t.Fatal("GC failed to update live slice count")
	}
	m.Restore(&withSlices)
	if got := snapshotHash(m); got != slicesHash || m.heap.sliceCount != 3 {
		t.Fatal("slice restore changed heap state")
	}
	obj = m.heap.get(values.A)
	if obj == nil || len(obj.elems) != 2 || cap(obj.elems) != 4 || obj.elems[0] != Int(3) || obj.elems[1] != Int(7) {
		t.Fatalf("snapshot aliases live slice: %+v", obj)
	}
	if o := m.heap.get(empty.A); o == nil || len(o.elems) != 0 || cap(o.elems) != 0 {
		t.Fatal("empty slice was not restored")
	}
	// Switch between independently held snapshots repeatedly; reusing a State
	// for Capture must also clear its previous slice bookkeeping.
	for range 3 {
		m.Restore(&onlyStrings)
		if m.heap.sliceCount != 0 || snapshotHash(m) != stringsHash || m.heap.get(values.A) != nil {
			t.Fatal("string-only restore changed heap state")
		}
		var reused State
		m.Capture(&reused)
		m.Restore(&withSlices)
		if m.heap.sliceCount != 3 || snapshotHash(m) != slicesHash {
			t.Fatal("repeated slice restore changed heap state")
		}
		m.Capture(&reused)
		m.Restore(&onlyStrings)
		m.Capture(&reused)
		if reused.sliceCount != 0 || len(reused.lens) != 0 || len(reused.elems) != 0 {
			t.Fatal("reused snapshot retained slice metadata")
		}
	}
	m.Restore(&withSlices)
	if !m.Collect(snapshotRoots{word}) || m.heap.sliceCount != 0 {
		t.Fatal("collecting all slices did not restore the fast path")
	}
	var afterGC State
	m.Capture(&afterGC)
	if afterGC.sliceCount != 0 || len(afterGC.lens) != 0 {
		t.Fatal("GC left stale slice bookkeeping")
	}
}
