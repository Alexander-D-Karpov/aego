package trace

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestPaceWaitMetadataSurvivesOverflowAndResets(t *testing.T) {
	tr := New(Options{Frames: 2, Events: 1})
	tr.SetEnabled(true)
	zone := tr.Zone("test")
	tr.BeginFrame()
	tr.Begin(zone).End()
	tr.Begin(zone).End() // No room for the pacing event itself.
	tr.AddPaceWait(10*time.Millisecond, false)
	tr.AddPaceWait(60*time.Millisecond, true)
	tr.AddPaceWait(-time.Second, true)
	tr.EndFrame()
	f := tr.Frame(0)
	if f.Dropped != 1 || f.PaceWaitNS != int64(70*time.Millisecond) || f.IdleWaitNS != int64(60*time.Millisecond) {
		t.Fatalf("dropped=%d pace=%d idle=%d", f.Dropped, f.PaceWaitNS, f.IdleWaitNS)
	}
	snapshot := tr.Snapshot(1)
	for range 2 {
		tr.BeginFrame()
		tr.EndFrame()
	}
	if f = tr.Frame(0); f.PaceWaitNS != 0 || f.IdleWaitNS != 0 {
		t.Fatalf("reused frame retained wait: %+v", f)
	}
	if snapshot[0].PaceWaitNS != int64(70*time.Millisecond) || snapshot[0].IdleWaitNS != int64(60*time.Millisecond) {
		t.Fatal("snapshot lost pacing metadata")
	}
	var out bytes.Buffer
	if err := WriteChrome(&out, tr, snapshot); err != nil {
		t.Fatal(err)
	}
	var exported struct {
		Events []struct {
			Args struct {
				PaceWaitNS int64 `json:"paceWaitNs"`
				IdleWaitNS int64 `json:"idleWaitNs"`
			} `json:"args"`
		} `json:"traceEvents"`
	}
	if err := json.Unmarshal(out.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported.Events) < 2 || exported.Events[1].Args.PaceWaitNS != int64(70*time.Millisecond) || exported.Events[1].Args.IdleWaitNS != int64(60*time.Millisecond) {
		t.Fatalf("exported pacing metadata: %+v", exported.Events)
	}
	var nilTracer *Tracer
	nilTracer.AddPaceWait(time.Second, true)
}
