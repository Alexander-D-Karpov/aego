package widgets

import (
	"math"
	"slices"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

type CurveState struct {
	Key   int
	part  int8
	drag  bool
	view  [4]float32
	times [2]float32
	timed bool
}

// CurvePad is the margin between a curve graph's box and its plot.
const CurvePad = 10

// SetTimeRange makes the graph show times t0..t1 across its plot instead of fitting the keys, to line up with an outside ruler.
func (s *CurveState) SetTimeRange(t0, t1 float32) {
	s.times, s.timed = [2]float32{t0, t1}, t1 > t0
}

func NewCurveState() *CurveState {
	return &CurveState{Key: -1}
}

var (
	curveCommit  = Edit{Changed: true, Begin: true, End: true}
	curveInterps = func() []string {
		return []string{Tr("editor.widgets.curve.bezier", "Bezier"), Tr("editor.widgets.curve.linear", "Linear"), Tr("editor.widgets.curve.step", "Step")}
	}
	curveTangents = func() []string {
		return []string{Tr("editor.widgets.curve.auto", "Auto"), Tr("editor.widgets.curve.linear", "Linear"), Tr("editor.widgets.curve.free", "Free")}
	}
	curveWraps = func() []string {
		return []string{Tr("editor.widgets.curve.clamp", "Clamp"), Tr("editor.widgets.curve.loop", "Loop"), Tr("editor.widgets.curve.ping_pong", "Ping-pong")}
	}
	curvePresets = func() []string {
		out := []string{"Keys"}
		for e := curve.Linear; e < curve.EasingCount; e++ {
			out = append(out, e.String())
		}
		return out
	}()
)

type curveFrame struct {
	r mathx.Rect
	v [4]float32
}

func (g curveFrame) at(t, v float32) mathx.Vec2 {
	return mathx.Vec2{
		X: g.r.X + (t-g.v[0])/(g.v[1]-g.v[0])*g.r.W,
		Y: g.r.Y + (1-(v-g.v[2])/(g.v[3]-g.v[2]))*g.r.H,
	}
}

func (g curveFrame) inv(p mathx.Vec2) (float32, float32) {
	return g.v[0] + (p.X-g.r.X)/g.r.W*(g.v[1]-g.v[0]),
		g.v[2] + (1-(p.Y-g.r.Y)/g.r.H)*(g.v[3]-g.v[2])
}

func curveView(cv *curve.Curve) [4]float32 {
	t0, t1 := cv.Span()
	if t1 <= t0 {
		t1 = t0 + 1
	}
	lo, hi := cv.Range(64)
	if hi-lo < 1e-3 {
		lo, hi = lo-0.5, hi+0.5
	}
	m := (hi - lo) * 0.1
	return [4]float32{t0, t1, lo - m, hi + m}
}

func curvePlot(c *core.Context, g curveFrame, cv *curve.Curve, col mathx.Color) {
	n := int(g.r.W)
	if n < 2 {
		return
	}
	lo, hi := g.r.Y, g.r.Y+g.r.H
	prev := g.at(g.v[0], cv.Eval(g.v[0])).Y
	for i := 1; i <= n; i++ {
		t := g.v[0] + (g.v[1]-g.v[0])*float32(i)/float32(n)
		y := g.at(t, cv.Eval(t)).Y
		y0 := mathx.Clamp(min(prev, y), lo, hi)
		y1 := mathx.Clamp(max(prev, y), lo, hi)
		c.Rect(mathx.Rect{X: g.r.X + float32(i-1), Y: y0, W: 1.5, H: max(y1-y0, 1.5)}, col)
		prev = y
	}
}

func curveGrid(c *core.Context, g curveFrame) {
	th := &c.Theme
	col := th.Border
	col.A *= 0.6
	for i := 0; i <= 4; i++ {
		f := float32(i) / 4
		c.Rect(mathx.Rect{X: g.r.X + g.r.W*f, Y: g.r.Y, W: 1, H: g.r.H}, col)
		c.Rect(mathx.Rect{X: g.r.X, Y: g.r.Y + g.r.H*f, W: g.r.W, H: 1}, col)
	}
	o := NumberOpts{Prec: 2}
	lh := c.LineHeight()
	c.Text(FormatNumber(float64(g.v[3]), o), mathx.Vec2{X: g.r.X + 2, Y: g.r.Y}, th.TextDim)
	c.Text(FormatNumber(float64(g.v[2]), o), mathx.Vec2{X: g.r.X + 2, Y: g.r.Y + g.r.H - lh}, th.TextDim)
	end := FormatNumber(float64(g.v[1]), o)
	c.Text(end, mathx.Vec2{X: g.r.X + g.r.W - c.TextSize(end).X - 2, Y: g.r.Y + g.r.H - lh}, th.TextDim)
}

func curveSquare(p mathx.Vec2, r float32) mathx.Rect {
	return mathx.Rect{X: p.X - r, Y: p.Y - r, W: 2 * r, H: 2 * r}
}

func curveDotted(c *core.Context, a, b mathx.Vec2, col mathx.Color) {
	d := b.Sub(a)
	n := max(int(d.Len()/3), 1)
	for i := 0; i <= n; i++ {
		p := a.Add(d.Scale(float32(i) / float32(n)))
		c.Rect(mathx.Rect{X: p.X, Y: p.Y, W: 1, H: 1}, col)
	}
}

func curveHandle(g curveFrame, cv *curve.Curve, i int, side int8) (mathx.Vec2, bool) {
	ks := cv.Keys
	k := &ks[i]
	if side > 0 {
		if i+1 >= len(ks) || k.Interp != curve.InterpBezier {
			return mathx.Vec2{}, false
		}
		d := (ks[i+1].T - k.T) / 3
		return g.at(k.T+d, k.V+k.Out*d), true
	}
	if i == 0 || ks[i-1].Interp != curve.InterpBezier {
		return mathx.Vec2{}, false
	}
	d := (k.T - ks[i-1].T) / 3
	return g.at(k.T-d, k.V-k.In*d), true
}

func curveKeys(c *core.Context, g curveFrame, cv *curve.Curve, st *CurveState) {
	th := &c.Theme
	if st.Key >= 0 {
		k := &cv.Keys[st.Key]
		p := g.at(k.T, k.V)
		for _, side := range [2]int8{-1, 1} {
			if h, ok := curveHandle(g, cv, st.Key, side); ok {
				curveDotted(c, p, h, th.TextDim)
				c.Rect(curveSquare(h, 3), th.Text)
			}
		}
	}
	for i := range cv.Keys {
		k := &cv.Keys[i]
		col := th.Text
		if i == st.Key {
			col = th.Accent
		}
		p := g.at(k.T, k.V)
		c.Rect(curveSquare(p, 5), th.Border)
		c.Rect(curveSquare(p, 4), col)
	}
}

func curvePick(g curveFrame, cv *curve.Curve, st *CurveState, m mathx.Vec2) (int, int8) {
	const r2 = 49
	if st.Key >= 0 {
		for _, side := range [2]int8{-1, 1} {
			if p, ok := curveHandle(g, cv, st.Key, side); ok && p.Sub(m).LenSq() <= r2 {
				return st.Key, side
			}
		}
	}
	for i := range cv.Keys {
		k := &cv.Keys[i]
		if g.at(k.T, k.V).Sub(m).LenSq() <= r2 {
			return i, 0
		}
	}
	return -1, 0
}

func curveSnap(x, lo, hi float32) float32 {
	step := (hi - lo) / 20
	if step <= 0 {
		return x
	}
	return lo + float32(math.Round(float64((x-lo)/step)))*step
}

func curveMove(cv *curve.Curve, i int, t, v float32) {
	ks := cv.Keys
	const gap = 1e-4
	if i > 0 {
		t = max(t, ks[i-1].T+gap)
	}
	if i < len(ks)-1 {
		t = min(t, ks[i+1].T-gap)
	}
	ks[i].T, ks[i].V = t, v
	cv.Smooth()
}

func curveSlope(k *curve.Key, s float32, side int8, broken bool) {
	k.Tangent = curve.TangentFree
	switch {
	case broken && side > 0:
		k.Out = s
	case broken:
		k.In = s
	default:
		k.In, k.Out = s, s
	}
}

func curveDrag(cv *curve.Curve, st *CurveState, t, v float32, broken bool) bool {
	i := st.Key
	if i < 0 || i >= len(cv.Keys) {
		return false
	}
	k := &cv.Keys[i]
	switch {
	case st.part == 0:
		if t == k.T && v == k.V {
			return false
		}
		curveMove(cv, i, t, v)
		return true
	case st.part > 0:
		if t <= k.T+1e-6 {
			return false
		}
		curveSlope(k, (v-k.V)/(t-k.T), 1, broken)
	default:
		if t >= k.T-1e-6 {
			return false
		}
		curveSlope(k, (k.V-v)/(k.T-t), -1, broken)
	}
	cv.Smooth()
	return true
}

func curveAdd(cv *curve.Curve, g curveFrame, m mathx.Vec2) int {
	t, v := g.inv(m)
	if t0, t1 := cv.Span(); len(cv.Keys) > 0 && t > t0 && t < t1 {
		v = cv.Eval(t)
	}
	cv.Keys = append(cv.Keys, curve.Key{T: t, V: v})
	cv.Fix()
	for i := range cv.Keys {
		if cv.Keys[i].T == t {
			if i > 0 {
				cv.Keys[i].Interp = cv.Keys[i-1].Interp
				cv.Smooth()
			}
			return i
		}
	}
	return -1
}

func curveGraph(c *core.Context, r mathx.Rect, cv *curve.Curve, st *CurveState) Edit {
	th := &c.Theme
	var e Edit
	c.Box(r, th.Radius.Field, th.Background, th.Border)
	const pad = CurvePad
	if !st.drag {
		st.view = curveView(cv)
		if st.timed {
			st.view[0], st.view[1] = st.times[0], st.times[1]
		}
	}
	g := curveFrame{r: mathx.Rect{X: r.X + pad, Y: r.Y + pad, W: r.W - 2*pad, H: r.H - 2*pad}, v: st.view}
	keyed := cv.Ease == curve.None
	if !keyed || st.Key >= len(cv.Keys) {
		st.Key = -1
	}
	c.PushClip(r)
	curveGrid(c, g)
	curvePlot(c, g, cv, th.Accent)
	if keyed {
		curveKeys(c, g, cv, st)
	}
	c.PopClip()
	if !keyed {
		return e
	}
	id := c.ID("graph")
	m := c.Mouse()
	if c.Pressed(id, r) {
		k, part := curvePick(g, cv, st, m)
		switch {
		case k >= 0:
			st.Key, st.part, st.drag = k, part, true
			c.CaptureMouse(id)
			e.Begin = true
		case c.DoubleClicked(id):
			st.Key = curveAdd(cv, g, m)
			e = e.Or(curveCommit)
		default:
			st.Key = -1
		}
	}
	if c.RightClicked(id, r) {
		if k, part := curvePick(g, cv, st, m); k >= 0 && part == 0 && len(cv.Keys) > 1 {
			cv.Keys = slices.Delete(cv.Keys, k, k+1)
			cv.Smooth()
			st.Key = -1
			e = e.Or(curveCommit)
		}
	}
	switch {
	case st.drag && c.Active() == id:
		if c.MouseDown(input.MouseLeft) {
			t, v := g.inv(m)
			if c.Primary() {
				t, v = curveSnap(t, g.v[0], g.v[1]), curveSnap(v, g.v[2], g.v[3])
			}
			if curveDrag(cv, st, t, v, c.Mods().Has(input.ModShift)) {
				e.Changed = true
			}
		}
		if c.MouseReleased(input.MouseLeft) && !c.MouseDown(input.MouseLeft) {
			st.drag = false
			c.ReleaseMouse()
			e.End = true
		}
	case st.drag:
		st.drag = false
		e.End = true
	}
	return e
}

func curveKeyRows(c *core.Context, cv *curve.Curve, st *CurveState) Edit {
	var e Edit
	k := cv.Keys[st.Key]
	o := NumberOpts{Speed: 0.01}
	t, v := k.T, k.V
	e = e.Or(DragFloat32(c, c.ID("t"), Field(c, Tr("editor.widgets.curve.time", "Time")), &t, o))
	e = e.Or(DragFloat32(c, c.ID("v"), Field(c, Tr("editor.widgets.curve.value", "Value")), &v, o))
	if t != k.T || v != k.V {
		curveMove(cv, st.Key, t, v)
	}
	ip, tg := int(k.Interp), int(k.Tangent)
	if Combo(c, c.ID("interp"), Field(c, Tr("editor.widgets.curve.segment", "Segment")), &ip, curveInterps(), false) {
		cv.Keys[st.Key].Interp = curve.Interp(ip)
		e = e.Or(curveCommit)
	}
	if Combo(c, c.ID("tangent"), Field(c, Tr("editor.widgets.curve.tangent", "Tangent")), &tg, curveTangents(), false) {
		cv.Keys[st.Key].Tangent = curve.Tangent(tg)
		cv.Smooth()
		e = e.Or(curveCommit)
	}
	if len(cv.Keys) > 1 && Button(c, "Delete Key") {
		cv.Keys = slices.Delete(cv.Keys, st.Key, st.Key+1)
		cv.Smooth()
		st.Key = -1
		e = e.Or(curveCommit)
	}
	return e
}

func curveEditor(c *core.Context, cv *curve.Curve, st *CurveState) Edit {
	var e Edit
	cur := 0
	if cv.Ease != curve.None {
		cur = int(cv.Ease-curve.Linear) + 1
	}
	if Combo(c, c.ID("preset"), Field(c, Tr("editor.widgets.curve.preset", "Preset")), &cur, curvePresets, false) {
		if cur == 0 {
			*cv = cv.Bake(9)
		} else {
			*cv = curve.Curve{Ease: curve.Linear + curve.Easing(cur-1), Pre: cv.Pre, Post: cv.Post}
		}
		st.Key = -1
		e = e.Or(curveCommit)
	}
	e = e.Or(curveGraph(c, c.Cursor().Next(180), cv, st))
	switch {
	case cv.Ease != curve.None:
		LabelDim(c, "Choose Keys to edit the preset point by point")
	case st.Key >= 0 && st.Key < len(cv.Keys):
		e = e.Or(curveKeyRows(c, cv, st))
	default:
		LabelDim(c, "Double-click the graph to add a key")
	}
	pre, post := int(cv.Pre), int(cv.Post)
	if Combo(c, c.ID("pre"), Field(c, Tr("editor.widgets.curve.before", "Before")), &pre, curveWraps(), false) {
		cv.Pre = curve.Wrap(pre)
		e = e.Or(curveCommit)
	}
	if Combo(c, c.ID("post"), Field(c, Tr("editor.widgets.curve.after", "After")), &post, curveWraps(), false) {
		cv.Post = curve.Wrap(post)
		e = e.Or(curveCommit)
	}
	return e
}

func CurveField(c *core.Context, id core.ID, r mathx.Rect, cv *curve.Curve, mixed bool, st *CurveState) Edit {
	th := &c.Theme
	var e Edit
	if c.Pressed(id, r) {
		if c.PopupOpen(id) {
			c.ClosePopup(id)
		} else {
			c.CloseAllPopups()
			c.OpenPopup(id, mathx.Vec2{X: r.X, Y: r.Y + r.H})
		}
	}
	c.Box(r, th.Radius.Field, c.WidgetColor(id), th.Border)
	if mixed {
		c.TextIn(r, th.Mixed, th.Text, core.AlignCenter)
	} else {
		in := mathx.Rect{X: r.X + 4, Y: r.Y + 3, W: r.W - 8, H: r.H - 6}
		c.PushClip(in)
		curvePlot(c, curveFrame{r: in, v: curveView(cv)}, cv, th.Accent)
		c.PopClip()
	}
	if !c.BeginPopup(id, 380) {
		return e
	}
	c.PushRawID(id)
	e = curveEditor(c, cv, st)
	c.PopID()
	c.EndPopup()
	return e
}

// CurveGraph draws an editable keyed curve in r without the popup of CurveField.
func CurveGraph(c *core.Context, id core.ID, r mathx.Rect, cv *curve.Curve, st *CurveState) Edit {
	c.PushRawID(id)
	defer c.PopID()
	return curveGraph(c, r, cv, st)
}

// Dragging reports whether a key or a handle of the graph is held.
func (s *CurveState) Dragging() bool {
	return s.drag
}
