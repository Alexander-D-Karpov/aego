package bufferdiff

import (
	"bytes"
	"testing"
)

func TestRetentionAndOverlappingWrites(t *testing.T) {
	var last LastWrite
	dev := make([]byte, maximum+64)
	want := make([]byte, len(dev))
	write := func(at int, b []byte) int {
		copy(want[at:], b)
		begin, delta := last.Delta(at, b)
		copy(dev[begin:], delta)
		if !bytes.Equal(dev, want) {
			t.Fatal("incorrect retained bytes")
		}
		return len(delta)
	}
	a := bytes.Repeat([]byte{41}, 256)
	if write(8, a) != 256 || write(8, a) != 0 {
		t.Fatal("initial or repeated")
	}
	a[85]++
	if write(8, a) != 1 {
		t.Fatal("unchanged bytes uploaded")
	}
	write(90, []byte{1, 2, 3, 4, 5})
	write(8, a)
	write(0, bytes.Repeat([]byte{1}, len(dev)))
	if write(8, a) == 0 {
		t.Fatal("long replacing write did not invalidate")
	}
	last.Reset()
	if write(8, a) != len(a) {
		t.Fatal("restored device reused old bytes")
	}
}

func TestReservedUniformFirstWrite(t *testing.T) {
	const size = 656
	var last LastWrite
	last.ReserveSmall(64 << 10)
	if cap(last.data) != 0 {
		t.Fatal("allocated unused large shadow")
	}
	last.ReserveSmall(size)
	data := make([]byte, size)
	if count := testing.AllocsPerRun(20, func() {
		last.Reset()
		offset, payload := last.Delta(0, data)
		if offset != 0 || len(payload) != size {
			t.Fatal("first write must reach the device, including zero bytes")
		}
		if _, repeat := last.Delta(0, data); len(repeat) != 0 {
			t.Fatal("duplicate write was not skipped")
		}
	}); count != 0 {
		t.Fatalf("reserved first write allocated %g objects", count)
	}
}
