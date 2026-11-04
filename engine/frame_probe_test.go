package engine

import (
	"testing"

	"aego/platform"
)

// probeByteApp retains one small allocation per frame so allocator caches
// cannot make the engine's per-frame byte total skip an allocation.
type probeByteApp struct {
	blocks [16][]byte
	n      int
	alloc  bool
}

func (*probeByteApp) Init(*Engine) error   { return nil }
func (*probeByteApp) Event(platform.Event) {}
func (*probeByteApp) FixedUpdate(float64)  {}
func (a *probeByteApp) Update(float64) {
	if a.alloc {
		a.blocks[a.n] = make([]byte, 128)
		a.n++
	}
}
func (*probeByteApp) Render(float64) {}
func (*probeByteApp) Shutdown()      {}

func TestFrameByteProbeCountsEachSmallAllocation(t *testing.T) {
	e, _ := newTestEngine(t)
	defer e.Close()
	app := &probeByteApp{}
	for range 5 {
		if !e.Step(app) {
			t.Fatal(e.Err())
		}
	}
	e.SetPhaseAllocs(true)
	app.alloc = true
	for i := 0; i < len(app.blocks); i++ {
		before := e.FrameBytes()
		if !e.Step(app) {
			t.Fatal(e.Err())
		}
		if got := e.FrameBytes() - before; got < 128 {
			t.Fatalf("frame %d: counted %d bytes, allocated at least 128", i, got)
		}
	}
	e.SetPhaseAllocs(false)
	app.alloc = false
	before := e.FrameBytes()
	if !e.Step(app) {
		t.Fatal(e.Err())
	}
	if got := e.FrameBytes(); got != before {
		t.Fatalf("disabled byte probe changed: %d -> %d", before, got)
	}
}
