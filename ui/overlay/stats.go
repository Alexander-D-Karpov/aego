package overlay

import (
	"strconv"

	"aego/core/mathx"
	"aego/core/stats"
	"aego/gpu"
	"aego/render"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Stats struct {
	ui   *core.Context
	buf  []byte
	Wide bool
}

func NewStats(ui *core.Context) *Stats {
	return &Stats{ui: ui, buf: make([]byte, 0, 128)}
}

func (s *Stats) Draw(fs *stats.Frame, rs *render.Stats, caps gpu.Capabilities, extra func(*core.Context)) {
	c := s.ui
	cur := c.Cursor()
	cur.W = 240
	if s.Wide {
		cur.W = 320
	}
	slot := c.List().Reserve()
	top := cur.Y

	s.row("fps ", fs.FPS, 0)
	s.row("frame ms ", fs.FrameTime*1000, 2)
	s.row("update ms ", fs.UpdateTime*1000, 2)
	s.row("render ms ", fs.RenderTime*1000, 2)
	s.rowU("draw calls ", uint64(rs.DrawCalls))
	s.rowU("batches ", uint64(rs.Batches))
	s.rowU("sprites ", uint64(rs.Visible))
	s.rowU("culled ", uint64(rs.Culled))
	s.rowU("allocs ", fs.Allocs)

	s.buf = s.buf[:0]
	s.buf = append(s.buf, "gpu ms "...)
	if rs.GPUValid {
		s.buf = strconv.AppendFloat(s.buf, float64(rs.GPUNS)/1e6, 'f', 2, 64)
	} else {
		s.buf = append(s.buf, "n/a"...)
	}
	s.text()

	if rs.Dropped > 0 || rs.ZClamped > 0 {
		s.buf = s.buf[:0]
		s.buf = append(s.buf, "dropped "...)
		s.buf = strconv.AppendUint(s.buf, uint64(rs.Dropped), 10)
		s.buf = append(s.buf, " clamped "...)
		s.buf = strconv.AppendUint(s.buf, uint64(rs.ZClamped), 10)
		s.text()
	}

	widgets.Separator(c)
	widgets.LabelDim(c, caps.Renderer)
	if extra != nil {
		widgets.Separator(c)
		extra(c)
	}

	c.List().Patch(slot, mathx.Rect{
		X: cur.X - cur.Pad,
		Y: top - cur.Pad,
		W: cur.W + cur.Pad*2,
		H: cur.Y - top + cur.Pad,
	}, c.Theme.Panel)
}

func (s *Stats) row(label string, v float64, prec int) {
	s.buf = s.buf[:0]
	s.buf = append(s.buf, label...)
	s.buf = strconv.AppendFloat(s.buf, v, 'f', prec, 64)
	s.text()
}

func (s *Stats) rowU(label string, v uint64) {
	s.buf = s.buf[:0]
	s.buf = append(s.buf, label...)
	s.buf = strconv.AppendUint(s.buf, v, 10)
	s.text()
}

func (s *Stats) text() {
	c := s.ui
	r := c.Cursor().Next(0)
	h := c.TextSize("M").Y
	c.List().Text(unsafeString(s.buf), mathx.Vec2{X: r.X, Y: r.Y + (r.H-h)/2}, c.Theme.Text)
}
