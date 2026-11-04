package widgets

import (
	"math"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/draw"
)

type Edit struct {
	Changed bool
	Begin   bool
	End     bool
}

func (e Edit) Or(o Edit) Edit {
	return Edit{Changed: e.Changed || o.Changed, Begin: e.Begin || o.Begin, End: e.End || o.End}
}

func (e Edit) Any() bool {
	return e.Changed || e.Begin || e.End
}

type NumberOpts struct {
	Speed      float64
	Min        float64
	Max        float64
	HasMin     bool
	HasMax     bool
	Prec       int
	Suffix     string
	Unit       string
	Mixed      bool
	Int        bool
	Lead       float32
	PairLabels [2]string
	Percent    bool
}

func (o NumberOpts) clamp(v float64) float64 {
	if o.HasMin && v < o.Min {
		v = o.Min
	}
	if o.HasMax && v > o.Max {
		v = o.Max
	}
	if o.Int {
		v = math.Round(v)
	}
	return v
}

func FormatNumber(v float64, o NumberOpts) string {
	if o.Percent {
		return strconv.FormatInt(int64(math.Round(v*100)), 10) + "%"
	}
	if o.Int {
		return strconv.FormatInt(int64(math.Round(v)), 10) + o.Suffix
	}
	prec := o.Prec
	if prec <= 0 {
		prec = 3
	}
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s + o.Suffix
}

func ParseNumber(s string, o NumberOpts) (float64, bool) {
	s = strings.TrimSpace(s)
	if o.Suffix != "" {
		s = strings.TrimSpace(strings.TrimSuffix(s, o.Suffix))
	}
	if o.Unit != "" {
		s = strings.TrimSpace(strings.TrimSuffix(s, o.Unit))
	}
	pct := o.Percent && strings.HasSuffix(s, "%")
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if pct || (o.Percent && f > 1) {
		f /= 100
	}
	return o.clamp(f), true
}

func DragNumber(c *core.Context, id core.ID, r mathx.Rect, v *float64, o NumberOpts) Edit {
	th := &c.Theme
	var e Edit
	if s, ok := c.LostText(id); ok {
		if f, ok := ParseNumber(s, o); ok && (f != *v || o.Mixed) {
			*v = f
			e = Edit{Changed: true, Begin: true, End: true}
		}
	}
	if c.Focus() == id {
		s := FormatNumber(*v, o)
		if _, committed := TextFieldRect(c, id, r, &s, false); committed {
			if f, ok := ParseNumber(s, o); ok {
				*v = f
				e = Edit{Changed: true, Begin: true, End: true}
			}
		}
		return e
	}
	if c.MouseOver(id, r) {
		c.SetCursorShape(core.CursorResizeEW)
	}
	if c.Pressed(id, r) {
		c.CaptureMouse(id)
		*c.DragValue() = *v
		e.Begin = true
	}
	if c.Active() == id {
		if d := c.MouseDelta().X; d != 0 && c.MouseDown(input.MouseLeft) {
			speed := o.Speed
			if speed == 0 {
				speed = 0.01
				if o.Int {
					speed = 0.1
				}
			}
			if c.Mods().Has(input.ModShift) {
				speed *= 0.1
			}
			acc := c.DragValue()
			*acc += float64(d) * speed
			if o.HasMin && *acc < o.Min {
				*acc = o.Min
			}
			if o.HasMax && *acc > o.Max {
				*acc = o.Max
			}
			if nv := o.clamp(*acc); nv != *v {
				*v = nv
				e.Changed = true
			}
		}
		if c.MouseReleased(input.MouseLeft) && !c.MouseDown(input.MouseLeft) {
			e.End = true
			c.ReleaseMouse()
			if c.Mouse().Sub(c.PressPos()).Len() < 3 {
				c.SetFocus(id)
				c.EditBuffer(id, FormatNumber(*v, o)).SelectAll()
			}
		}
	}
	c.Box(r, th.Radius.Field, c.WidgetColor(id), th.Border)
	if o.HasMin && o.HasMax && o.Max > o.Min && !o.Mixed {
		t := mathx.Clamp(float32((*v-o.Min)/(o.Max-o.Min)), 0, 1)
		corners := draw.CornersLeft
		if t >= 1 {
			corners = draw.CornersAll
		}
		bar := mathx.Rect{X: r.X + 1, Y: r.Y + 1, W: (r.W - 2) * t, H: r.H - 2}
		c.List().RoundRect(bar, th.Radius.Field-1, th.SelectionDim, corners)
	}
	label := FormatNumber(*v, o)
	if o.Mixed && !e.Changed {
		label = th.Mixed
	}
	c.PushClip(r)
	vr := mathx.Rect{X: r.X + o.Lead, Y: r.Y, W: r.W - o.Lead, H: r.H}
	if o.Unit != "" {
		if uw := c.TextSize(o.Unit).X; uw+24 < r.W {
			c.Text(o.Unit, mathx.Vec2{X: r.X + r.W - uw - 5, Y: c.TextY(r)}, th.Unit)
			vr.W -= uw + 8
		}
	}
	c.TextIn(vr, label, th.Text, core.AlignCenter)
	c.PopClip()
	return e
}

func DragFloat32(c *core.Context, id core.ID, r mathx.Rect, v *float32, o NumberOpts) Edit {
	f := float64(*v)
	e := DragNumber(c, id, r, &f, o)
	if e.Changed {
		*v = float32(f)
	}
	return e
}

func DragInt64(c *core.Context, id core.ID, r mathx.Rect, v *int64, o NumberOpts) Edit {
	o.Int = true
	f := float64(*v)
	e := DragNumber(c, id, r, &f, o)
	if e.Changed {
		*v = int64(f)
	}
	return e
}

func vecField(c *core.Context, id core.ID, r mathx.Rect, ptrs []*float32, mixed []bool, o NumberOpts) Edit {
	th := &c.Theme
	const gap = float32(4)
	n := len(ptrs)
	w := (r.W - gap*float32(n-1)) / float32(n)
	cols := [3]mathx.Color{th.AxisX, th.AxisY, th.AxisZ}
	names := [3]string{"X", "Y", "Z"}
	rangePair := n == 2 && o.PairLabels[0] != "" && o.PairLabels[1] != ""
	if rangePair {
		names[0], names[1] = o.PairLabels[0], o.PairLabels[1]
		cols[0], cols[1] = th.TextDim, th.TextDim
	}
	var e Edit
	for i, ptr := range ptrs {
		cell := mathx.Rect{X: r.X + float32(i)*(w+gap), Y: r.Y, W: w, H: r.H}
		sub := core.IDSeq(id, i+1)
		oo := o
		oo.Mixed = mixed[i]
		oo.Lead = 14
		if rangePair {
			oo.Lead = c.TextSize(names[i]).X + 9
		}
		e = e.Or(DragFloat32(c, sub, cell, ptr, oo))
		if c.Focus() != sub {
			c.Rect(mathx.Rect{X: cell.X + 2, Y: cell.Y + 4, W: 2, H: cell.H - 8}, cols[i])
			c.Text(names[i], mathx.Vec2{X: cell.X + 6, Y: c.TextY(cell)}, cols[i])
		}
	}
	return e
}

func Vec2Field(c *core.Context, id core.ID, r mathx.Rect, v *mathx.Vec2, o NumberOpts, mixedX, mixedY bool) Edit {
	return vecField(c, id, r, []*float32{&v.X, &v.Y}, []bool{mixedX, mixedY}, o)
}

func Vec3Field(c *core.Context, id core.ID, r mathx.Rect, v *mathx.Vec3, o NumberOpts, mixed [3]bool) Edit {
	return vecField(c, id, r, []*float32{&v.X, &v.Y, &v.Z}, mixed[:], o)
}
