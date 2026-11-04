package gpuprof

import (
	"testing"

	"aego/gpu"
	"aego/gpu/null"
)

type sampleFrame struct {
	gpu.Frame
	ns    uint64
	ready bool
	polls int
}

func (f *sampleFrame) QueryResult(gpu.QueryHandle) (uint64, bool) { f.polls++; return f.ns, f.ready }

type samplePass struct {
	gpu.Pass
	begins, ends int
}

func (p *samplePass) BeginQuery(gpu.QueryHandle) { p.begins++ }
func (p *samplePass) EndQuery()                  { p.ends++ }

type contextDevice struct {
	*null.Device
	generation uint64
	lost       bool
}

func (d *contextDevice) ContextGeneration() uint64 { return d.generation }
func (d *contextDevice) ContextLost() bool         { return d.lost }

func TestCompletedDisjointSampleIsNotReportedAsGPUTime(t *testing.T) {
	p := &Profiler{dev: null.New(), enabled: true, valid: true, last: 100}
	p.pending[0] = true
	f := &sampleFrame{ready: true, ns: 0}
	p.Collect(f)
	if _, valid := p.LastNS(); valid || p.pending[0] {
		t.Fatal("disjoint sample was displayed or its slot was not recycled")
	}
	p.pending[1] = true
	f.ns = 200
	p.Collect(f)
	if ns, valid := p.LastNS(); !valid || ns != 200 {
		t.Fatal("valid asynchronous sample was lost")
	}
}

func TestBusyQueryDoesNotEndAnUnstartedInterval(t *testing.T) {
	p := &Profiler{enabled: true}
	p.pending[p.head] = true
	pass := &samplePass{}
	p.Begin(pass)
	p.End(pass)
	if pass.begins != 0 || pass.ends != 0 {
		t.Fatal("busy slot ended another interval")
	}
	p.pending[p.head] = false
	p.Begin(pass)
	p.End(pass)
	p.End(pass)
	if pass.begins != 1 || pass.ends != 1 {
		t.Fatal("interval begin/end must be paired exactly once")
	}
}

func TestContextLossInvalidatesSamplesAndPendingSlots(t *testing.T) {
	d := &contextDevice{Device: null.New(), generation: 2, lost: true}
	p := &Profiler{dev: d, enabled: true, generation: 1, valid: true, last: 10, active: true}
	p.pending[0] = true
	f := &sampleFrame{ns: 999, ready: true}
	p.Collect(f)
	if p.valid || p.active || f.polls != 0 {
		t.Fatal("lost context exposed or polled GPU time")
	}
	d.lost = false
	p.Collect(f)
	if p.valid || p.pending[0] || f.polls != 0 {
		t.Fatal("old-device query survived restore")
	}
}
