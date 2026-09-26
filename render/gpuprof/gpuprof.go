package gpuprof

import "aego/gpu"

const ringSize = 3

type Profiler struct {
	dev     gpu.Device
	queries [ringSize]gpu.QueryHandle
	pending [ringSize]bool
	head    int
	last    uint64
	valid   bool
	enabled bool
}

func New(dev gpu.Device, caps gpu.Capabilities) *Profiler {
	p := &Profiler{dev: dev}
	if !caps.TimerQueries {
		return p
	}
	for i := range p.queries {
		q, err := dev.CreateQuery()
		if err != nil {
			p.Close()
			return &Profiler{dev: dev}
		}
		p.queries[i] = q
	}
	p.enabled = true
	return p
}

func (p *Profiler) Enabled() bool {
	return p.enabled
}

func (p *Profiler) Begin(pass gpu.Pass) {
	if !p.enabled || p.pending[p.head] {
		return
	}
	pass.BeginQuery(p.queries[p.head])
	p.pending[p.head] = true
}

func (p *Profiler) End(pass gpu.Pass) {
	if !p.enabled || !p.pending[p.head] {
		return
	}
	pass.EndQuery()
}

func (p *Profiler) Collect(f gpu.Frame) {
	if !p.enabled {
		return
	}
	for i := range p.queries {
		if !p.pending[i] {
			continue
		}
		if ns, ok := f.QueryResult(p.queries[i]); ok {
			p.last = ns
			p.valid = true
			p.pending[i] = false
		}
	}
	p.head = (p.head + 1) % ringSize
}

func (p *Profiler) LastNS() (uint64, bool) {
	return p.last, p.valid
}

func (p *Profiler) Close() {
	for i, q := range p.queries {
		if !q.IsZero() {
			p.dev.DestroyQuery(q)
			p.queries[i] = gpu.QueryHandle{}
		}
	}
	p.enabled = false
}
