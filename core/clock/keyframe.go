package clock

import "aego/core/rewind"

type TimeState struct {
	t Time
}

func (t *Time) Capture(dst *TimeState) {
	dst.t = *t
}

func (t *Time) Restore(src *TimeState) {
	*t = src.t
}

func (t *Time) Hash(h *rewind.Hasher) {
	h.String("time")
	h.U64(t.frame)
	h.U64(t.updates)
	h.U64(t.tick)
	h.U64(t.dropped)
	h.F64(t.scale)
	h.Bool(t.paused)
	h.U64(uint64(t.pending))
	h.F64(t.acc)
	h.F64(t.alpha)
	h.F64(t.clamped)
	h.F64(t.lastGame)
	for i := range t.spans {
		s := &t.spans[i]
		h.F64(s.Delta)
		h.F64(s.Elapsed)
		h.F64(s.Scale)
		h.Bool(s.Paused)
	}
}
