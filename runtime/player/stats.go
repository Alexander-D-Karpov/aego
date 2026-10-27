package player

import (
	"runtime/metrics"

	"aego/core/clock"
	"aego/engine"
	"aego/gpu"
	"aego/platform"
	"aego/render"
)

var statNames = [...]string{
	"/gc/cycles/total:gc-cycles",
	"/gc/heap/allocs:objects",
	"/gc/heap/allocs:bytes",
	"/memory/classes/heap/objects:bytes",
	"/gc/heap/goal:bytes",
}

type Stats struct {
	Frames       uint64            `json:"frames"`
	Ticks        uint64            `json:"ticks"`
	GC           uint64            `json:"gc"`
	AllocObjects uint64            `json:"allocObjects"`
	AllocBytes   uint64            `json:"allocBytes"`
	HeapBytes    uint64            `json:"heapBytes"`
	HeapGoal     uint64            `json:"heapGoal"`
	Zones        map[string]uint64 `json:"zones,omitempty"`
	FrameBytes   uint64            `json:"frameBytes,omitempty"`
	Bridge       *gpu.BridgeStats  `json:"bridge,omitempty"`
	GPUNs        uint64            `json:"gpuNs"`
	GPUFrames    uint64            `json:"gpuFrames"`
	Draws        uint64            `json:"draws"`
	Sprites      uint64            `json:"sprites"`
	Keys         uint64            `json:"keys"`
	Clamped      float64           `json:"clamped"`
	MaxReal      float64           `json:"maxReal"`
	Lost         bool              `json:"lost"`
	Restores     int               `json:"restores"`
	Renderer     string            `json:"renderer"`
}

type gpuLoss interface {
	Lost() bool
	Restores() int
}

type sampler struct {
	render    func() render.Stats
	gpuNs     uint64
	gpuFrames uint64
	draws     uint64
	sprites   uint64
	keys      uint64
	maxReal   float64
	metrics   []metrics.Sample
	zones     []uint64
	win       *window
}

type window struct {
	from, to uint64
	start    Stats
	opened   func()
	closed   func(start, end Stats)
}

func (s *sampler) frame(w *engine.World) {
	if s.render != nil {
		r := s.render()
		if r.GPUValid {
			s.gpuNs += r.GPUNS
			s.gpuFrames++
		}
		s.draws += uint64(r.DrawCalls)
		s.sprites += uint64(r.Visible)
	}
	if w != nil && w.Time != nil {
		s.maxReal = max(s.maxReal, w.Time.Span(clock.Real).Delta)
	}
}

func (s *sampler) event(ev platform.Event) {
	if ev.Kind == platform.EvKeyDown || ev.Kind == platform.EvKeyUp {
		s.keys++
	}
}

func (s *sampler) value(i int) uint64 {
	if v := s.metrics[i].Value; v.Kind() == metrics.KindUint64 {
		return v.Uint64()
	}
	return 0
}

func (p *Player) Stats() Stats {
	s := &p.sampler
	if s.metrics == nil {
		s.metrics = make([]metrics.Sample, len(statNames))
		for i, n := range statNames {
			s.metrics[i].Name = n
		}
	}
	metrics.Read(s.metrics)
	e := p.Engine
	w := e.MainWorld()
	out := Stats{
		Frames: w.Time.Frame(), Ticks: w.Time.Tick(), GC: s.value(0), AllocObjects: s.value(1), AllocBytes: s.value(2),
		HeapBytes: s.value(3), HeapGoal: s.value(4),
		GPUNs: s.gpuNs, GPUFrames: s.gpuFrames, Draws: s.draws, Sprites: s.sprites, Keys: s.keys,
		Clamped: w.Time.ClampedTime(), MaxReal: s.maxReal, Renderer: e.Caps().Renderer,
	}
	if p.cfg.Probe {
		s.zones = e.AllocTotals(s.zones[:0])
		out.Zones = make(map[string]uint64, len(s.zones))
		for i, n := range s.zones {
			out.Zones[engine.AllocZone(i)] = n
		}
		out.FrameBytes = e.FrameBytes()
	}
	if b, ok := e.GPU().(gpu.BridgeReporter); ok {
		st := b.BridgeStats()
		out.Bridge = &st
	}
	if l, ok := e.GPU().(gpuLoss); ok {
		out.Lost, out.Restores = l.Lost(), l.Restores()
	}
	return out
}

func (p *Player) Window(from, to uint64, opened func(), closed func(start, end Stats)) bool {
	if to <= from || p.Engine.MainWorld().Time.Frame() >= from {
		return false
	}
	p.sampler.win = &window{from: from, to: to, opened: opened, closed: closed}
	return true
}

func (p *Player) markWindow() {
	w := p.sampler.win
	if w == nil {
		return
	}
	switch p.Engine.MainWorld().Time.Frame() {
	case w.from:
		if p.cfg.MemProfile {
			p.memStart = memSnapshot()
		}
		w.start = p.Stats()
		w.opened()
	case w.to:
		p.sampler.win = nil
		end := p.Stats()
		if p.memStart != nil {
			r := memDiff(p.memStart, memSnapshot())
			p.allocs, p.memStart = &r, nil
		}
		w.closed(w.start, end)
	}
}

func (p *Player) ResetMaxReal() {
	p.sampler.maxReal = 0
}
