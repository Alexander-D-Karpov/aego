package overlay

import (
	"strconv"
	"unsafe"

	"aego/core/stats"
	"aego/engine"
	"aego/platform"
)

const (
	interval = 0.5
	bufSize  = 160
)

type Overlay struct {
	win      platform.Window
	stats    *stats.Frame
	base     string
	buf      []byte
	elapsed  float64
	Interval float64
}

func New(e *engine.Engine) *Overlay {
	return &Overlay{
		win:      e.Window(),
		stats:    e.Stats(),
		base:     e.Config().Title,
		buf:      make([]byte, 0, bufSize),
		elapsed:  interval,
		Interval: interval,
	}
}

func (o *Overlay) Update(dt float64) {
	o.elapsed += dt
	if o.elapsed < o.Interval {
		return
	}
	o.elapsed = 0

	s := o.stats
	b := append(o.buf[:0], o.base...)
	b = append(b, " | "...)
	b = strconv.AppendFloat(b, s.FPS, 'f', 1, 64)
	b = append(b, " fps  "...)
	b = appendMillis(b, s.FrameTime)
	b = append(b, " ms (upd "...)
	b = appendMillis(b, s.UpdateTime)
	b = append(b, " / gpu "...)
	b = appendMillis(b, s.RenderTime)
	b = append(b, ")  draws "...)
	b = strconv.AppendUint(b, uint64(s.DrawCalls), 10)
	b = append(b, "  tris "...)
	b = strconv.AppendUint(b, uint64(s.Triangles), 10)
	b = append(b, "  allocs "...)
	b = strconv.AppendUint(b, s.Allocs, 10)
	o.buf = b

	// SetTitle implementations copy the text, so a borrowed string keeps the frame allocation-free.
	o.win.SetTitle(unsafe.String(unsafe.SliceData(b), len(b)))
}

func appendMillis(b []byte, seconds float64) []byte {
	return strconv.AppendFloat(b, seconds*1000, 'f', 2, 64)
}
