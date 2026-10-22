package events

import (
	"slices"
	"strconv"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/props"
	"aego/editor/core/services"
	"aego/editor/core/state"
	"aego/engine"
	"aego/engine/recorder"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "events"

var shown = [...]recorder.Kind{recorder.Signal, recorder.Event, recorder.Fault, recorder.Spawn, recorder.Despawn, recorder.SceneChange, recorder.Input, recorder.Audio, recorder.Gameplay}

type Deps struct {
	World     func() *engine.World
	Selection services.Selection
	Notices   services.Notices
	Root      string
}

type Panel struct {
	d        Deps
	recs     []recorder.Record
	view     []int
	rev      uint64
	src      *recorder.Recorder
	show     [recorder.KindCount]bool
	search   string
	frame    uint64
	selected bool
	replayed bool
	brk      bool
	brkText  string
	scroll   float32
	sel      int
	lastSel  []ids.NodeID
}

func New(d Deps) *Panel {
	p := &Panel{d: d, sel: -1}
	for i := range p.show {
		p.show[i] = true
	}
	return p
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Event Monitor"
}

func (p *Panel) Focus(frame uint64) {
	p.frame = frame
	p.scroll = 0
}

func (p *Panel) source() (*recorder.Recorder, *scene.Scene) {
	if p.d.World == nil {
		return nil, nil
	}
	w := p.d.World()
	if w == nil || w.Recorder == nil {
		return nil, nil
	}
	s, _ := engine.ServiceOf[*scene.Scene](w, scene.ServiceID)
	return w.Recorder, s
}

func (p *Panel) mask() uint32 {
	var m uint32
	for i, on := range p.show {
		if on {
			m |= recorder.Bit(recorder.Kind(i))
		}
	}
	return m
}

func (p *Panel) apply(rec *recorder.Recorder) {
	if p.brk {
		rec.BreakOn(p.mask(), p.brkText)
	} else {
		rec.BreakOn(0, "")
	}
	var nodes []ids.NodeID
	if p.selected {
		nodes = p.d.Selection.Selected()
	}
	if rec != p.src || !slices.Equal(nodes, p.lastSel) {
		rec.SetFilter(recorder.All, nodes)
		p.lastSel = slices.Clone(nodes)
	}
}

func (p *Panel) Check() (string, bool) {
	rec, _ := p.source()
	if rec == nil || !p.brk {
		return "", false
	}
	p.apply(rec)
	r, ok := rec.Tripped()
	if !ok {
		return "", false
	}
	return r.Kind.String() + " " + r.Name, true
}

func nameOf(s *scene.Scene) func(ids.NodeID) string {
	return func(id ids.NodeID) string {
		if s != nil {
			if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
				return s.Name(h)
			}
		}
		return id.String()[:8]
	}
}

func (p *Panel) refresh(rec *recorder.Recorder, s *scene.Scene) {
	if rec != p.src || rec.Revision() != p.rev {
		p.recs = rec.Snapshot(p.recs)
		p.rev = rec.Revision()
	}
	p.src = rec
	q := strings.ToLower(strings.TrimSpace(p.search))
	name := nameOf(s)
	p.view = p.view[:0]
	for i := range p.recs {
		r := &p.recs[i]
		if !p.show[r.Kind] || (r.Replayed && !p.replayed) || (p.frame != 0 && r.Frame != p.frame) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Line(name)), q) {
			continue
		}
		p.view = append(p.view, i)
	}
}

func (p *Panel) export(s *scene.Scene) {
	var b strings.Builder
	name := nameOf(s)
	for _, i := range p.view {
		b.WriteString(p.recs[i].Line(name))
		b.WriteByte('\n')
	}
	path, err := state.WriteEvents(p.d.Root, []byte(b.String()))
	if err != nil {
		p.d.Notices.Notify(context.LevelError, "cannot write the event log: "+err.Error())
		return
	}
	p.d.Notices.Notify(context.LevelInfo, "Event log written to "+path)
}

func (p *Panel) toolbar(ui *core.Context, s *scene.Scene) {
	row := ui.Cursor().Next(0)
	x := row.X
	for i, k := range shown {
		w := ui.TextSize(k.String()).X + 16
		widgets.ToggleButton(ui, core.IDSeq(ui.ID("kind"), i), mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}, k.String(), &p.show[k])
		x += w + 4
	}
	if row.X+row.W-x < 120 {
		row = ui.Cursor().Next(0)
		x = row.X
	}
	widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: x, Y: row.Y, W: row.X + row.W - x, H: row.H}, &p.search)
	row = ui.Cursor().Next(0)
	x = row.X
	place := func(w float32) mathx.Rect {
		r := mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}
		x += w + 4
		return r
	}
	widgets.ToggleButton(ui, ui.ID("selected"), place(130), "Selected nodes", &p.selected)
	widgets.ToggleButton(ui, ui.ID("replayed"), place(130), "Show replayed", &p.replayed)
	if p.frame != 0 && widgets.ButtonRect(ui, ui.ID("frame"), place(130), "frame "+strconv.FormatUint(p.frame, 10)+"  ×") {
		p.frame = 0
	}
	if widgets.ButtonRect(ui, ui.ID("export"), place(70), "Export") {
		p.export(s)
	}
	widgets.ToggleButton(ui, ui.ID("break"), place(110), "Break on", &p.brk)
	if row.X+row.W-x > 40 {
		widgets.TextFieldRect(ui, ui.ID("breaktext"), mathx.Rect{X: x, Y: row.Y, W: row.X + row.W - x, H: row.H}, &p.brkText, true)
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	rec, s := p.source()
	if rec == nil {
		widgets.LabelDim(ui, "Events of the running game appear here while Play runs")
		return
	}
	p.apply(rec)
	p.refresh(rec, s)
	p.toolbar(ui, s)
	counts := rec.Counts()
	var c strings.Builder
	for _, k := range shown {
		c.WriteString(k.String() + " " + strconv.FormatUint(counts[k], 10) + "   ")
	}
	widgets.LabelDim(ui, c.String())
	top := ui.Cursor().Y
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	name := nameOf(s)
	widgets.VirtualList(ui, ui.ID("list"), list, len(p.view), th.RowHeight, &p.scroll, func(i int, row mathx.Rect) {
		idx := p.view[i]
		rc := &p.recs[idx]
		id := core.IDSeq(ui.ID("row"), idx)
		if ui.Pressed(id, row) {
			p.sel = idx
			if !rc.Owner.IsZero() {
				p.d.Selection.Select(rc.Owner)
			}
		}
		switch {
		case p.sel == idx:
			ui.Rect(row, th.SelectionDim)
		case ui.Hot() == id:
			ui.Rect(row, th.WidgetHot)
		}
		col := th.Text
		if rc.Kind == recorder.Fault {
			col = th.Error
		}
		x := row.X + 24
		if s != nil && !rc.Owner.IsZero() {
			if icon, _, tip := props.NodeLabel(s, s.Handle(rc.Owner), false); icon != "" {
				widgets.IconAt(ui, mathx.Rect{X: row.X + 4, Y: row.Y + (row.H-16)/2, W: 16, H: 16}, icon, th.TextDim)
				widgets.Tooltip(ui, id, tip)
			}
		}
		ui.PushClip(row)
		ui.Text(rc.Line(name), mathx.Vec2{X: x, Y: ui.TextY(row)}, col)
		ui.PopClip()
	})
}
