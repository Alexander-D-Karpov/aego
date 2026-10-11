package coroutine

import (
	"cmp"
	"slices"

	"aego/script/vm"
)

type Stats struct {
	Live     int
	Ready    int
	Sleeping int
	Fixed    int
	Awaiting int
	Resumed  uint64
	Canceled uint64
	Visits   uint64
}

const (
	inNone uint8 = iota
	inFrames
	inTimed
	inFixed
	inAwait
)

type rec struct {
	gen    uint32
	live   bool
	owner  vm.InstanceID
	seq    uint64
	where  uint8
	pos    int
	key    float64
	frame  uint64
	target vm.CoHandle
}

type item struct {
	h   vm.CoHandle
	seq uint64
}

type queue struct {
	heap []uint32
	tag  uint8
}

func bySeq(a, b item) int {
	return cmp.Compare(a.seq, b.seq)
}

type Scheduler struct {
	m        *vm.Machine
	recs     []rec
	live     int
	awaiting map[vm.CoHandle][]uint32
	waiters  int
	frames   queue
	timed    queue
	fixed    queue
	run      []item
	frun     []item
	woken    []item
	later    []uint32
	seq      uint64
	now      float64
	frame    uint64
	fixedN   uint64
	inFrame  bool
	inFixed  bool
	stats    Stats
	Self     func(vm.InstanceID) ([]vm.Value, bool)
	OnFault  func(vm.InstanceID, *vm.Fault)
}

func New(m *vm.Machine) *Scheduler {
	s := &Scheduler{
		m:        m,
		awaiting: map[vm.CoHandle][]uint32{},
		frames:   queue{tag: inFrames},
		timed:    queue{tag: inTimed},
		fixed:    queue{tag: inFixed},
	}
	m.Co = s
	return s
}

func (s *Scheduler) Start(f *vm.Func, self []vm.Value, args []vm.Value, owner vm.InstanceID) (vm.CoHandle, *vm.Fault) {
	return s.m.Start(f, self, args, owner)
}

func (s *Scheduler) less(a, b uint32) bool {
	ra, rb := &s.recs[a], &s.recs[b]
	return ra.key < rb.key || (ra.key == rb.key && ra.seq < rb.seq)
}

func (s *Scheduler) swap(q *queue, i, j int) {
	q.heap[i], q.heap[j] = q.heap[j], q.heap[i]
	s.recs[q.heap[i]].pos = i
	s.recs[q.heap[j]].pos = j
}

func (s *Scheduler) up(q *queue, i int) {
	for i > 0 {
		p := (i - 1) / 2
		if !s.less(q.heap[i], q.heap[p]) {
			return
		}
		s.swap(q, i, p)
		i = p
	}
}

func (s *Scheduler) down(q *queue, i int) {
	n := len(q.heap)
	for {
		l := 2*i + 1
		if l >= n {
			return
		}
		m := l
		if r := l + 1; r < n && s.less(q.heap[r], q.heap[l]) {
			m = r
		}
		if !s.less(q.heap[m], q.heap[i]) {
			return
		}
		s.swap(q, i, m)
		i = m
	}
}

func (s *Scheduler) push(q *queue, i uint32) {
	r := &s.recs[i]
	r.where, r.pos = q.tag, len(q.heap)
	q.heap = append(q.heap, i)
	s.up(q, r.pos)
}

func (s *Scheduler) removeAt(q *queue, pos int) uint32 {
	i := q.heap[pos]
	last := len(q.heap) - 1
	if pos != last {
		s.swap(q, pos, last)
	}
	q.heap = q.heap[:last]
	if pos < last {
		s.down(q, pos)
		s.up(q, pos)
	}
	s.recs[i].where = inNone
	return i
}

func (s *Scheduler) take(q *queue) item {
	i := s.removeAt(q, 0)
	r := &s.recs[i]
	return item{h: vm.CoHandle{Index: i, Gen: r.gen}, seq: r.seq}
}

func (s *Scheduler) get(h vm.CoHandle) *rec {
	if int(h.Index) >= len(s.recs) {
		return nil
	}
	r := &s.recs[h.Index]
	if !r.live || r.gen != h.Gen {
		return nil
	}
	return r
}

func (s *Scheduler) track(h vm.CoHandle) *rec {
	for int(h.Index) >= len(s.recs) {
		s.recs = append(s.recs, rec{})
	}
	r := &s.recs[h.Index]
	if r.live && r.gen == h.Gen {
		return r
	}
	if r.live {
		s.unlink(h.Index)
		s.live--
	}
	owner, _ := s.m.CoOwner(h)
	s.seq++
	*r = rec{gen: h.Gen, live: true, owner: owner, seq: s.seq}
	s.live++
	return r
}

func (s *Scheduler) unlink(i uint32) {
	r := &s.recs[i]
	switch r.where {
	case inFrames:
		s.removeAt(&s.frames, r.pos)
	case inTimed:
		s.removeAt(&s.timed, r.pos)
	case inFixed:
		s.removeAt(&s.fixed, r.pos)
	case inAwait:
		ws := s.awaiting[r.target]
		if k := slices.Index(ws, i); k >= 0 {
			ws = slices.Delete(ws, k, k+1)
			s.waiters--
			if len(ws) == 0 {
				delete(s.awaiting, r.target)
			} else {
				s.awaiting[r.target] = ws
			}
		}
		r.where, r.target = inNone, vm.CoHandle{}
	}
}

func (s *Scheduler) frameDue(n int64) uint64 {
	due := s.frame + uint64(n)
	if !s.inFrame {
		due++
	}
	return due
}

func (s *Scheduler) fixedDue(n int64) uint64 {
	due := s.fixedN + uint64(n)
	if !s.inFixed {
		due++
	}
	return due
}

func (s *Scheduler) Yielded(h vm.CoHandle, w vm.WaitKind, arg vm.Value) {
	r := s.track(h)
	i := h.Index
	switch w {
	case vm.WaitTime:
		if d := float64(arg.Float()); d > 0 {
			r.key, r.frame = s.now+d, s.frameDue(1)
			s.push(&s.timed, i)
			return
		}
	case vm.WaitFrames:
		r.frame = s.frameDue(max(arg.Int(), 1))
		r.key = float64(r.frame)
		s.push(&s.frames, i)
		return
	case vm.WaitFixed:
		r.frame = s.fixedDue(max(arg.Int(), 1))
		r.key = float64(r.frame)
		s.push(&s.fixed, i)
		return
	case vm.WaitAwait:
		t := vm.CoOf(arg)
		if st, ok := s.m.CoState(t); ok && st != vm.CoDone && t != h {
			r.where, r.target = inAwait, t
			s.awaiting[t] = append(s.awaiting[t], i)
			s.waiters++
			return
		}
	}
	r.frame = s.frameDue(1)
	r.key = float64(r.frame)
	s.push(&s.frames, i)
}

func (s *Scheduler) Ended(h vm.CoHandle, st vm.CoState) {
	if r := s.get(h); r != nil {
		s.unlink(h.Index)
		r.live = false
		s.live--
	}
	if st == vm.CoCanceled {
		s.stats.Canceled++
	}
	ws, ok := s.awaiting[h]
	if !ok {
		return
	}
	delete(s.awaiting, h)
	s.waiters -= len(ws)
	for _, i := range ws {
		r := &s.recs[i]
		r.where, r.target = inNone, vm.CoHandle{}
		it := item{h: vm.CoHandle{Index: i, Gen: r.gen}, seq: r.seq}
		if s.inFrame {
			s.run = append(s.run, it)
		} else {
			s.woken = append(s.woken, it)
		}
	}
}

func (s *Scheduler) resume(h vm.CoHandle) {
	r := s.get(h)
	if r == nil {
		return
	}
	if st, ok := s.m.CoState(h); !ok || st != vm.CoSuspended {
		return
	}
	owner := r.owner
	var self []vm.Value
	if s.Self != nil {
		var ok bool
		if self, ok = s.Self(owner); !ok {
			s.m.Cancel(h)
			return
		}
	}
	s.stats.Resumed++
	if _, fl := s.m.Resume(h, self); fl != nil && s.OnFault != nil {
		s.OnFault(owner, fl)
	}
}

func (s *Scheduler) Frame(dt float32) {
	s.frame++
	s.now += float64(dt)
	s.inFrame = true
	s.run = append(s.run[:0], s.woken...)
	s.woken = s.woken[:0]
	for len(s.frames.heap) > 0 && s.recs[s.frames.heap[0]].frame <= s.frame {
		s.run = append(s.run, s.take(&s.frames))
	}
	for len(s.timed.heap) > 0 && s.recs[s.timed.heap[0]].key <= s.now {
		if s.recs[s.timed.heap[0]].frame > s.frame {
			s.later = append(s.later, s.removeAt(&s.timed, 0))
			continue
		}
		s.run = append(s.run, s.take(&s.timed))
	}
	for _, i := range s.later {
		s.push(&s.timed, i)
	}
	s.later = s.later[:0]
	slices.SortFunc(s.run, bySeq)
	for k := 0; k < len(s.run); k++ {
		s.stats.Visits++
		s.resume(s.run[k].h)
	}
	s.run = s.run[:0]
	s.inFrame = false
}

func (s *Scheduler) Fixed() {
	s.fixedN++
	s.inFixed = true
	s.frun = s.frun[:0]
	for len(s.fixed.heap) > 0 && s.recs[s.fixed.heap[0]].frame <= s.fixedN {
		s.frun = append(s.frun, s.take(&s.fixed))
	}
	slices.SortFunc(s.frun, bySeq)
	for k := 0; k < len(s.frun); k++ {
		s.stats.Visits++
		s.resume(s.frun[k].h)
	}
	s.inFixed = false
}

func (s *Scheduler) Stop(h vm.CoHandle) {
	s.m.Cancel(h)
}

func (s *Scheduler) CancelOwner(id vm.InstanceID) int {
	return s.CancelTypes(func(o vm.InstanceID) bool { return o == id })
}

func (s *Scheduler) CancelTypes(owners func(vm.InstanceID) bool) int {
	n := 0
	for i := range s.recs {
		r := &s.recs[i]
		if r.live && owners(r.owner) {
			s.m.Cancel(vm.CoHandle{Index: uint32(i), Gen: r.gen})
			n++
		}
	}
	return n
}

func (s *Scheduler) CancelAll() int {
	return s.CancelTypes(func(vm.InstanceID) bool { return true })
}

func (s *Scheduler) Stats() Stats {
	st := s.stats
	st.Live = s.live
	st.Ready = len(s.woken)
	st.Sleeping = len(s.frames.heap) + len(s.timed.heap)
	st.Fixed = len(s.fixed.heap)
	st.Awaiting = s.waiters
	return st
}
