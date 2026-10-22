package watch

import (
	"strconv"
	"strings"

	"aego/core/clock"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/props"
	"aego/editor/core/services"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "watch"

var ops = [...]string{">=", "<=", "==", "!=", ">", "<"}

type row struct {
	src   string
	expr  string
	op    string
	limit float64
	armed bool
	text  string
	num   float64
	isNum bool
	err   string
}

type Deps struct {
	World    func() *engine.World
	Scripts  func() context.ScriptHost
	Viewport *services.Viewport
}

type Panel struct {
	d      Deps
	rows   []row
	input  string
	hud    bool
	scroll float32
}

func New(d Deps) *Panel {
	p := &Panel{d: d, hud: true}
	if d.Viewport != nil {
		d.Viewport.GameHUD = append(d.Viewport.GameHUD, p.drawHUD)
	}
	return p
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Watch"
}

func parse(src string) row {
	r := row{src: strings.TrimSpace(src), armed: true}
	r.expr = r.src
	for _, op := range ops {
		i := strings.Index(r.src, op)
		if i < 0 {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(r.src[i+len(op):]), 64)
		if err != nil {
			r.err = "the right side of " + op + " must be a number"
			break
		}
		r.expr, r.op, r.limit = strings.TrimSpace(r.src[:i]), op, v
		break
	}
	return r
}

func (p *Panel) Add(src string) {
	if strings.TrimSpace(src) != "" {
		p.rows = append(p.rows, parse(src))
	}
}

func (p *Panel) Exprs() []string {
	out := make([]string, len(p.rows))
	for i, r := range p.rows {
		out[i] = r.src
	}
	return out
}

func (p *Panel) SetExprs(list []string) {
	p.rows = p.rows[:0]
	for _, s := range list {
		p.Add(s)
	}
}

func holds(op string, a, b float64) bool {
	switch op {
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	case "==":
		return a == b
	case "!=":
		return a != b
	case ">":
		return a > b
	}
	return a < b
}

func (p *Panel) Check() (string, bool) {
	w := p.world()
	if w == nil {
		return "", false
	}
	for i := range p.rows {
		r := &p.rows[i]
		if r.op == "" {
			continue
		}
		p.eval(r, w)
		hit := r.err == "" && r.isNum && holds(r.op, r.num, r.limit)
		if !hit {
			r.armed = true
			continue
		}
		if r.armed {
			r.armed = false
			return r.expr + " = " + r.text, true
		}
	}
	return "", false
}

func (p *Panel) world() *engine.World {
	if p.d.World == nil {
		return nil
	}
	return p.d.World()
}

func laneOf(s string) (int, bool) {
	switch strings.ToLower(s) {
	case "x", "r":
		return 0, true
	case "y", "g":
		return 1, true
	case "z", "b":
		return 2, true
	case "w", "a":
		return 3, true
	}
	return -1, false
}

func findType(list []*schema.TypeDescriptor, name string) *schema.TypeDescriptor {
	for _, td := range list {
		if td.FullName == name || strings.HasSuffix(td.FullName, "."+name) || td.DisplayName == name {
			return td
		}
	}
	return nil
}

func fieldOf(td *schema.TypeDescriptor, name string) (*schema.FieldDescriptor, bool) {
	if fd, ok := td.Field(name); ok {
		return fd, true
	}
	if name != "" {
		return td.Field(strings.ToLower(name[:1]) + name[1:])
	}
	return nil, false
}

func (p *Panel) timeValue(r *row, t *clock.Time, name string) {
	var v float64
	switch name {
	case "Scale":
		v = t.Scale()
	case "Elapsed":
		v = t.Span(clock.Game).Elapsed
	case "Unscaled":
		v = t.Span(clock.Real).Elapsed
	case "Tick":
		v = float64(t.Tick())
	case "Frame":
		v = float64(t.Frame())
	case "Paused":
		if t.Paused() {
			v = 1
		}
	default:
		r.err = "time has Scale, Elapsed, Unscaled, Tick, Frame and Paused"
		return
	}
	r.text, r.num, r.isNum = strconv.FormatFloat(v, 'g', 6, 64), v, true
}

func setValue(r *row, v schema.Value, lane int) {
	num := func(f float64) {
		r.text, r.num, r.isNum = strconv.FormatFloat(f, 'g', 5, 64), f, true
	}
	l4 := func(xs ...float32) {
		if lane >= 0 && lane < len(xs) {
			num(float64(xs[lane]))
			return
		}
		parts := make([]string, len(xs))
		for i, x := range xs {
			parts[i] = strconv.FormatFloat(float64(x), 'g', 5, 32)
		}
		r.text = "(" + strings.Join(parts, ", ") + ")"
	}
	switch v.Kind {
	case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindEnum:
		num(float64(v.I))
	case schema.KindFloat32, schema.KindFloat64:
		num(v.F)
	case schema.KindVec2:
		l4(v.V.X, v.V.Y)
	case schema.KindVec3:
		l4(v.V3.X, v.V3.Y, v.V3.Z)
	case schema.KindColor:
		l4(v.C.R, v.C.G, v.C.B, v.C.A)
	case schema.KindString:
		r.text = strconv.Quote(v.S)
	case schema.KindNodeRef:
		r.text = v.Node.String()[:8]
	default:
		r.text = v.Kind.String()
	}
}

func (p *Panel) eval(r *row, w *engine.World) {
	r.text, r.num, r.isNum = "", 0, false
	if r.err != "" && r.op == "" && strings.Contains(r.err, "number") {
		return
	}
	r.err = ""
	segs := strings.Split(r.expr, ".")
	if segs[0] == "time" && len(segs) == 2 {
		p.timeValue(r, w.Time, segs[1])
		return
	}
	lane := -1
	if n := len(segs); n >= 4 {
		if l, ok := laneOf(segs[n-1]); ok {
			lane, segs = l, segs[:n-1]
		}
	}
	n := len(segs)
	if n < 3 {
		r.err = "write Node.Component.Field or time.Scale"
		return
	}
	s, ok := engine.ServiceOf[*scene.Scene](w, scene.ServiceID)
	if !ok || s == nil {
		r.err = "no scene"
		return
	}
	path, comp, field := strings.Join(segs[:n-2], "."), segs[n-2], segs[n-1]
	h := s.Find(s.Root(), path)
	if h.IsZero() || !s.IsAlive(h) {
		r.err = "no node " + path
		return
	}
	reg := s.Registry()
	if td := findType(reg.Components(), comp); td != nil {
		fd, ok := fieldOf(td, field)
		if !ok {
			r.err = comp + " has no field " + field
			return
		}
		v, err := s.GetProperty(h, td.ID, fd.ID)
		if err != nil {
			r.err = err.Error()
			return
		}
		setValue(r, v, lane)
		if v.Kind == schema.KindNodeRef {
			if _, name, _ := props.NodeLabel(s, s.Handle(v.Node), false); name != "" {
				r.text = name
			}
		}
		return
	}
	var host context.ScriptHost
	if p.d.Scripts != nil {
		host = p.d.Scripts()
	}
	if td := findType(reg.Scripts(), comp); td != nil && host != nil {
		text, num, isNum, ok := host.Watch(w, h, td.FullName, field, lane)
		if !ok {
			r.err = "no running " + comp + "." + field + " on " + path
			return
		}
		r.text, r.num, r.isNum = text, num, isNum
		return
	}
	r.err = "no component or script " + comp
}

func (r *row) value() string {
	if r.err != "" {
		return r.err
	}
	return r.text
}

func (p *Panel) refresh() {
	w := p.world()
	for i := range p.rows {
		if w == nil {
			p.rows[i].text, p.rows[i].err = "", "not playing"
			continue
		}
		p.eval(&p.rows[i], w)
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	p.refresh()
	row := ui.Cursor().Next(0)
	bw := float32(60)
	id := ui.ID("expr")
	_, submit := widgets.TextFieldRect(ui, id, mathx.Rect{X: row.X, Y: row.Y, W: row.W - 2*bw - 8, H: row.H}, &p.input, true)
	if widgets.ButtonRect(ui, ui.ID("add"), mathx.Rect{X: row.X + row.W - 2*bw - 4, Y: row.Y, W: bw, H: row.H}, "Add") || submit {
		p.Add(p.input)
		p.input = ""
	}
	widgets.ToggleButton(ui, ui.ID("hud"), mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}, "HUD", &p.hud)
	if len(p.rows) == 0 {
		widgets.LabelDim(ui, "Player.PlayerController.Speed, Player.PlayerController.Velocity.y, time.Scale; add >= 3 to pause there")
		return
	}
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	remove := -1
	widgets.VirtualList(ui, ui.ID("rows"), list, len(p.rows), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		w := &p.rows[i]
		col := th.Text
		if w.err != "" {
			col = th.TextDim
		}
		ui.PushClip(mathx.Rect{X: rr.X, Y: rr.Y, W: rr.W - rr.H - 4, H: rr.H})
		ui.Text(w.src, mathx.Vec2{X: rr.X + 4, Y: ui.TextY(rr)}, th.TextDim)
		ui.Text(w.value(), mathx.Vec2{X: rr.X + rr.W*0.55, Y: ui.TextY(rr)}, col)
		ui.PopClip()
		if widgets.ButtonRect(ui, core.IDSeq(ui.ID("rm"), i), mathx.Rect{X: rr.X + rr.W - rr.H, Y: rr.Y, W: rr.H, H: rr.H}, "×") {
			remove = i
		}
	})
	if remove >= 0 {
		p.rows = append(p.rows[:remove], p.rows[remove+1:]...)
	}
}

func (p *Panel) drawHUD(ui *core.Context, r mathx.Rect) {
	if !p.hud || len(p.rows) == 0 || p.world() == nil {
		return
	}
	p.refresh()
	th := &ui.Theme
	lh := ui.LineHeight()
	box := mathx.Rect{X: r.X + r.W - 288, Y: r.Y + th.BarHeight + 12, W: 280, H: float32(len(p.rows))*lh + 8}
	bg := th.PanelAlt
	bg.A = 0.85
	ui.Rect(box, bg)
	ui.PushClip(box)
	y := box.Y + 4
	for i := range p.rows {
		w := &p.rows[i]
		ui.Text(w.expr+" = "+w.value(), mathx.Vec2{X: box.X + 6, Y: y}, th.Text)
		y += lh
	}
	ui.PopClip()
}
