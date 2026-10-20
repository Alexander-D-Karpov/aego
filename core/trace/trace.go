package trace

import (
	"math"
	"runtime/metrics"
	"slices"
	"time"
)

type ZoneID uint16

const (
	MaxDepth      = 32
	DefaultFrames = 240
	DefaultEvents = 1024
	gcMetric      = "/gc/cycles/total:gc-cycles"
)

type Event struct {
	Zone  ZoneID
	Depth uint8
	Start uint32
	Dur   uint32
}

type Frame struct {
	Index     uint64
	Start     int64
	Dur       int64
	GC        uint32
	Dropped   uint32
	World     uint64
	FrameID   uint64
	FirstTick uint64
	LastTick  uint64
	Events    []Event
}

func (f *Frame) Duration() time.Duration {
	return time.Duration(f.Dur)
}

func (f *Frame) Depth() int {
	d := 0
	for _, e := range f.Events {
		d = max(d, int(e.Depth)+1)
	}
	return d
}

type Options struct {
	Frames int
	Events int
}

type Tracer struct {
	opt    Options
	names  []string
	index  map[string]ZoneID
	frames []Frame
	head   int
	count  int
	cur    *Frame
	stack  [MaxDepth]int32
	depth  int
	on     bool
	seq    uint64
	base   time.Time
	gc     []metrics.Sample
	lastGC uint64
}

func New(o Options) *Tracer {
	if o.Frames < 2 {
		o.Frames = DefaultFrames
	}
	if o.Events <= 0 {
		o.Events = DefaultEvents
	}
	return &Tracer{opt: o, names: []string{""}, index: map[string]ZoneID{}, base: time.Now()}
}

func (t *Tracer) now() int64 {
	return int64(time.Since(t.base))
}

func (t *Tracer) rel(f *Frame) uint32 {
	d := t.now() - f.Start
	if d > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(d)
}

func (t *Tracer) gcCycles() uint64 {
	metrics.Read(t.gc)
	if t.gc[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return t.gc[0].Value.Uint64()
}

func (t *Tracer) Zone(name string) ZoneID {
	if t == nil {
		return 0
	}
	if id, ok := t.index[name]; ok {
		return id
	}
	if len(t.names) > math.MaxUint16 {
		return 0
	}
	id := ZoneID(len(t.names))
	t.names = append(t.names, name)
	t.index[name] = id
	return id
}

func (t *Tracer) Name(z ZoneID) string {
	if t == nil || int(z) >= len(t.names) {
		return ""
	}
	return t.names[z]
}

func (t *Tracer) Enabled() bool {
	return t != nil && t.on
}

func (t *Tracer) SetEnabled(on bool) {
	if t == nil {
		return
	}
	if on == t.on {
		return
	}
	if on && t.frames == nil {
		t.frames = make([]Frame, t.opt.Frames)
		for i := range t.frames {
			t.frames[i].Events = make([]Event, 0, t.opt.Events)
		}
		t.gc = []metrics.Sample{{Name: gcMetric}}
	}
	if on {
		t.lastGC = t.gcCycles()
	}
	t.cur, t.depth = nil, 0
	t.on = on
}

func (t *Tracer) BeginFrame() {
	if t == nil || !t.on {
		return
	}
	if t.cur != nil {
		t.EndFrame()
	}
	t.seq++
	f := &t.frames[t.head]
	f.Index, f.Start, f.Dur, f.GC, f.Dropped = t.seq, t.now(), 0, 0, 0
	f.World, f.FrameID, f.FirstTick, f.LastTick = 0, 0, 0, 0
	f.Events = f.Events[:0]
	t.cur, t.depth = f, 0
}

func (t *Tracer) Mark(world, frame, first, last uint64) {
	if t == nil || t.cur == nil {
		return
	}
	f := t.cur
	f.World, f.FrameID, f.FirstTick, f.LastTick = world, frame, first, last
}

func (t *Tracer) EndFrame() {
	if t == nil || t.cur == nil {
		return
	}
	for t.depth > 0 {
		t.end()
	}
	f := t.cur
	f.Dur = t.now() - f.Start
	n := t.gcCycles()
	f.GC = uint32(n - t.lastGC)
	t.lastGC = n
	t.cur = nil
	t.head = (t.head + 1) % len(t.frames)
	if t.count < len(t.frames)-1 {
		t.count++
	}
}

type Scope struct {
	t     *Tracer
	frame uint64
}

func (t *Tracer) Begin(z ZoneID) Scope {
	if t == nil || t.cur == nil {
		return Scope{}
	}
	f := t.cur
	i := int32(-1)
	if t.depth < MaxDepth && len(f.Events) < cap(f.Events) {
		i = int32(len(f.Events))
		f.Events = append(f.Events, Event{Zone: z, Depth: uint8(t.depth), Start: t.rel(f)})
	} else {
		f.Dropped++
	}
	if t.depth < MaxDepth {
		t.stack[t.depth] = i
	}
	t.depth++
	return Scope{t: t, frame: f.Index}
}

func (s Scope) End() {
	t := s.t
	if t == nil || t.cur == nil || t.cur.Index != s.frame {
		return
	}
	t.end()
}

func (t *Tracer) end() {
	if t.depth == 0 {
		return
	}
	t.depth--
	if t.depth >= MaxDepth {
		return
	}
	if i := t.stack[t.depth]; i >= 0 {
		f := t.cur
		f.Events[i].Dur = t.rel(f) - f.Events[i].Start
	}
}

func (t *Tracer) Frames() int {
	if t == nil {
		return 0
	}
	return t.count
}

func (t *Tracer) Frame(back int) *Frame {
	if t == nil || back < 0 || back >= t.count {
		return nil
	}
	n := len(t.frames)
	return &t.frames[(t.head-1-back+2*n)%n]
}

func (t *Tracer) Snapshot(n int) []Frame {
	n = min(n, t.Frames())
	out := make([]Frame, n)
	for k := 0; k < n; k++ {
		f := t.Frame(n - 1 - k)
		out[k] = *f
		out[k].Events = slices.Clone(f.Events)
	}
	return out
}
