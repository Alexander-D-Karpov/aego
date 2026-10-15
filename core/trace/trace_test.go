package trace

import (
	"bytes"
	"encoding/json"
	"testing"
)

func record(t *Tracer, a, b ZoneID) {
	t.BeginFrame()
	outer := t.Begin(a)
	inner := t.Begin(b)
	inner.End()
	outer.End()
	t.EndFrame()
}

func TestNesting(t *testing.T) {
	tr := New(Options{Frames: 4, Events: 8})
	tr.SetEnabled(true)
	a, b := tr.Zone("a"), tr.Zone("b")
	record(tr, a, b)
	f := tr.Frame(0)
	if f == nil || len(f.Events) != 2 {
		t.Fatalf("frame %+v", f)
	}
	if f.Events[0].Depth != 0 || f.Events[1].Depth != 1 || f.Events[1].Start < f.Events[0].Start {
		t.Fatalf("events %+v", f.Events)
	}
	if f.Events[1].Start+f.Events[1].Dur > f.Events[0].Start+f.Events[0].Dur {
		t.Fatal("child ends after the parent")
	}
	if tr.Zone("a") != a || tr.Name(b) != "b" {
		t.Fatal("zone registry")
	}
}

func TestRingKeepsNewest(t *testing.T) {
	tr := New(Options{Frames: 4, Events: 8})
	tr.SetEnabled(true)
	z := tr.Zone("z")
	for i := 0; i < 10; i++ {
		record(tr, z, z)
	}
	if tr.Frames() != 3 {
		t.Fatalf("frames %d", tr.Frames())
	}
	if tr.Frame(0).Index != 10 || tr.Frame(2).Index != 8 || tr.Frame(3) != nil {
		t.Fatal("ring order")
	}
}

func TestOverflowAndOpenScopes(t *testing.T) {
	tr := New(Options{Frames: 4, Events: 2})
	tr.SetEnabled(true)
	z := tr.Zone("z")
	tr.BeginFrame()
	s := tr.Begin(z)
	tr.Begin(z)
	tr.Begin(z)
	tr.EndFrame()
	s.End()
	f := tr.Frame(0)
	if len(f.Events) != 2 || f.Dropped != 1 {
		t.Fatalf("events %d dropped %d", len(f.Events), f.Dropped)
	}
}

func TestDisabledRecordsNothing(t *testing.T) {
	tr := New(Options{})
	z := tr.Zone("z")
	tr.BeginFrame()
	tr.Begin(z).End()
	tr.EndFrame()
	if tr.Frames() != 0 {
		t.Fatal("disabled tracer recorded a frame")
	}
	var nilTracer *Tracer
	nilTracer.BeginFrame()
	nilTracer.Begin(z).End()
	nilTracer.EndFrame()
}

func TestZeroAllocs(t *testing.T) {
	tr := New(Options{})
	tr.SetEnabled(true)
	a, b := tr.Zone("a"), tr.Zone("b")
	record(tr, a, b)
	if n := testing.AllocsPerRun(100, func() { record(tr, a, b) }); n != 0 {
		t.Fatalf("%v allocations per traced frame", n)
	}
}

func TestChromeExport(t *testing.T) {
	tr := New(Options{Frames: 4, Events: 8})
	tr.SetEnabled(true)
	a, b := tr.Zone("phase.Update"), tr.Zone("quote\"d\x01")
	record(tr, a, b)
	record(tr, a, b)
	var buf bytes.Buffer
	if err := WriteChrome(&buf, tr, tr.Snapshot(tr.Frames())); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		TraceEvents []struct {
			Name string  `json:"name"`
			Ph   string  `json:"ph"`
			Dur  float64 `json:"dur"`
		} `json:"traceEvents"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if len(doc.TraceEvents) != 1+2*3 || doc.TraceEvents[3].Name != "quote\"d\x01" {
		t.Fatalf("events %+v", doc.TraceEvents)
	}
}

func TestDisableDropsOpenFrame(t *testing.T) {
	tr := New(Options{Frames: 4, Events: 8})
	tr.SetEnabled(true)
	z := tr.Zone("z")
	record(tr, z, z)
	tr.BeginFrame()
	s := tr.Begin(z)
	tr.SetEnabled(false)
	s.End()
	if tr.Frames() != 1 || tr.Frame(0).Index != 1 {
		t.Fatalf("frames %d", tr.Frames())
	}
	tr.SetEnabled(true)
	record(tr, z, z)
	if tr.Frames() != 2 || tr.Frame(0).GC > 1 {
		t.Fatalf("frames %d gc %d", tr.Frames(), tr.Frame(0).GC)
	}
}
