package workspace

import (
	"fmt"
	"runtime/debug"
	"sort"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/trace"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/editor/core/state"
	"aego/format/aetext"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/dock"
	"aego/ui/widgets"
)

const (
	Hierarchy = "hierarchy"
	Scene     = "scene"
	Game      = "game"
	Inspector = "inspector"
	Assets    = "assets"
	Console   = "console"
	Problems  = "problems"
	Profiler  = "profiler"
	Mixer     = "audio.mixer"
)

type Slot uint8

const (
	SlotNone Slot = iota
	SlotLeft
	SlotBrowser
	SlotCenter
	SlotBottom
	SlotRight
	slotCount
)

type Panel interface {
	ID() string
	Title() string
	Draw(f *services.Frame, r mathx.Rect)
}

type Desc struct {
	Panel  Panel
	Slot   Slot
	Near   string
	Order  int
	Hidden bool
	Icon   string
}

type Scoped interface {
	Scope() string
}

type Iconed interface {
	Icon() string
}

type Dirtier interface {
	Dirty() bool
}

type Closer interface {
	RequestClose()
}

type Workspace struct {
	Tree      *dock.Tree
	Notices   services.Notices
	Tracer    *trace.Tracer
	OnCrash   func(id string, rec any, stack []byte)
	root      string
	log       *log.ChannelLogger
	descs     map[string]Desc
	order     []string
	failed    map[string]string
	hovered   string
	focused   string
	changedAt float64
	pending   bool
	zones     map[string]trace.ZoneID
	frame     services.Frame
	presets   []Preset
	cur       string
	layouts   map[string]*layout
	foreign   []*aetext.Node
	applied   bool
}

func New(root string, lg *log.ChannelLogger) *Workspace {
	w := &Workspace{
		root:    root,
		log:     lg,
		descs:   map[string]Desc{},
		failed:  map[string]string{},
		zones:   map[string]trace.ZoneID{},
		layouts: map[string]*layout{},
	}
	w.Tree = dock.New()
	w.bind()
	return w
}

func (w *Workspace) bind() {
	w.Tree.Title = func(id string) string {
		if d, ok := w.descs[id]; ok {
			return d.Panel.Title()
		}
		return id
	}
	w.Tree.Icon = func(id string) string {
		d, ok := w.descs[id]
		if !ok {
			return ""
		}
		if ic, ok := d.Panel.(Iconed); ok {
			if s := ic.Icon(); s != "" {
				return s
			}
		}
		return d.Icon
	}
	w.Tree.Dirty = func(id string) bool {
		if d, ok := w.descs[id]; ok {
			if dr, ok := d.Panel.(Dirtier); ok {
				return dr.Dirty()
			}
		}
		return false
	}
	w.Tree.OnClose = func(id string) bool {
		d, ok := w.descs[id]
		if !ok {
			return false
		}
		if c, ok := d.Panel.(Closer); ok {
			c.RequestClose()
			return true
		}
		return false
	}
}

func (w *Workspace) Register(d Desc) error {
	if d.Panel == nil {
		return errs.New(errs.EditorPanel, "panel descriptor has no panel")
	}
	id := d.Panel.ID()
	switch {
	case id == "":
		return errs.New(errs.EditorPanel, "panel needs an id")
	case d.Slot >= slotCount:
		return errs.New(errs.EditorPanel, "panel has an unknown slot", log.F("panel", id))
	case d.Slot != SlotNone && d.Near != "":
		return errs.New(errs.EditorPanel, "panel has both a slot and a near panel", log.F("panel", id))
	}
	if _, dup := w.descs[id]; dup {
		return errs.New(errs.EditorPanel, "duplicate panel id", log.F("panel", id))
	}
	w.descs[id] = d
	w.order = append(w.order, id)
	return nil
}

func (w *Workspace) Has(id string) bool {
	_, ok := w.descs[id]
	return ok
}

func (w *Workspace) Unregister(id string) {
	if _, ok := w.descs[id]; !ok {
		return
	}
	delete(w.descs, id)
	for i, o := range w.order {
		if o == id {
			w.order = append(w.order[:i], w.order[i+1:]...)
			break
		}
	}
	w.Tree.Undock(id)
}

func (w *Workspace) Panel(id string) (Panel, bool) {
	d, ok := w.descs[id]
	return d.Panel, ok
}

func (w *Workspace) Panels() []Panel {
	out := make([]Panel, 0, len(w.order))
	for _, id := range w.order {
		out = append(out, w.descs[id].Panel)
	}
	return out
}

func (w *Workspace) sorted() []Desc {
	out := make([]Desc, 0, len(w.order))
	for _, id := range w.order {
		out = append(out, w.descs[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func (w *Workspace) Menu() []Desc {
	var out []Desc
	for _, d := range w.sorted() {
		if !d.Hidden {
			out = append(out, d)
		}
	}
	return out
}

func (w *Workspace) DefaultLayout() *dock.Tree {
	return w.layoutFor(w.preset(w.cur))
}

func (w *Workspace) retain() {
	w.Tree.Retain(func(id string) bool {
		_, ok := w.descs[id]
		return ok
	})
}

func (w *Workspace) LoadLayout() {
	if data, ok := state.LoadWorkspaces(w.root); ok {
		if err := w.decode(data); err != nil {
			w.warn("workspaces are unreadable, using the defaults", err)
		}
	} else if data, ok := state.LoadLayout(w.root); ok {
		w.migrate(data)
	}
	if w.preset(w.cur) == nil && len(w.presets) > 0 {
		w.cur = w.presets[0].ID
	}
	w.apply()
}

func (w *Workspace) SaveLayout() error {
	w.stash()
	return state.SaveWorkspaces(w.root, w.encode())
}

func (w *Workspace) ResetLayout() {
	if err := state.RemoveLayout(w.root); err != nil {
		w.warn("cannot remove the saved layout", err)
	}
	l := w.layoutOf(w.cur)
	l.tree, l.set = nil, false
	w.Tree = w.layoutFor(w.preset(w.cur))
	w.bind()
	w.retain()
	w.applied = true
	w.changedAt = 0
	w.pending = true
}

func (w *Workspace) Visible(id string) bool {
	return w.Tree.Has(id)
}

func (w *Workspace) anchor(d Desc) string {
	if d.Near != "" {
		return d.Near
	}
	p := w.preset(w.cur)
	slot := slotIn(p, d)
	if slot == SlotNone {
		slot = d.Slot
	}
	if slot == SlotNone {
		return ""
	}
	for _, o := range w.sorted() {
		if id := o.Panel.ID(); slotIn(p, o) == slot && w.Tree.Has(id) {
			return id
		}
	}
	return ""
}

func (w *Workspace) Show(id string) {
	d, ok := w.descs[id]
	if !ok {
		return
	}
	if m := w.Tree.Maximized(); m != "" && m != id {
		w.Tree.Restore()
	}
	if !w.Tree.Has(id) {
		if near := w.anchor(d); near != "" {
			if t := w.Tree.FindPanel(near); t != 0 {
				w.Tree.DockPanel(id, t, dock.SideCenter)
			}
		}
	}
	w.Tree.EnsurePanel(id)
	w.Tree.Activate(id)
}

func (w *Workspace) Toggle(id string) {
	if w.Tree.Has(id) {
		w.Tree.Undock(id)
		return
	}
	w.Show(id)
}

func (w *Workspace) Hovered() string {
	return w.hovered
}

func (w *Workspace) Focused() string {
	return w.focused
}

func (w *Workspace) Dragging() bool {
	return w.Tree.Dragging()
}

func (w *Workspace) Scope(id string) string {
	if d, ok := w.descs[id]; ok {
		if s, ok := d.Panel.(Scoped); ok {
			return s.Scope()
		}
	}
	return id
}

func (w *Workspace) Draw(f services.Frame, r mathx.Rect) {
	ui := f.UI
	w.frame = f
	w.Tree.Draw(ui, r, func(id string, pr mathx.Rect) {
		w.drawPanel(ui, id, pr)
	})
	w.hovered, _ = w.Tree.PanelAt(ui.Mouse())
	if ui.MousePressed(input.MouseLeft) || ui.MousePressed(input.MouseRight) {
		w.focused = w.hovered
	}
}

func (w *Workspace) drawPanel(ui *core.Context, id string, r mathx.Rect) {
	d, ok := w.descs[id]
	if !ok {
		ui.TextIn(r, "unknown panel "+id, ui.Theme.TextDim, core.AlignCenter)
		return
	}
	mark := ui.Mark()
	ui.PushClip(r)
	defer ui.PopClip()
	if msg, bad := w.failed[id]; bad {
		ui.Region(r)
		widgets.Label(ui, "This panel crashed and was disabled.")
		widgets.LabelWrapped(ui, msg, true)
		if widgets.Button(ui, "Retry") {
			delete(w.failed, id)
		}
		ui.PopCursor()
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			if w.OnCrash != nil {
				w.OnCrash(id, rec, debug.Stack())
			}
			ui.Restore(mark)
			ui.PushClip(r)
			msg := fmt.Sprint(rec)
			w.failed[id] = msg
			if w.Notices != nil {
				w.Notices.Notify(context.LevelError, "panel "+id+" crashed: "+msg)
			}
		}
	}()
	z, ok := w.zones[id]
	if !ok {
		z = w.Tracer.Zone("panel." + id)
		w.zones[id] = z
	}
	sc := w.Tracer.Begin(z)
	defer sc.End()
	ui.Region(r)
	w.frame.Focused = w.focused == id
	d.Panel.Draw(&w.frame, r)
	ui.PopCursor()
}

func (w *Workspace) Tick(now float64) {
	if w.Tree.Changed() || w.pending {
		w.changedAt = now
		w.pending = false
	}
	if w.changedAt > 0 && now-w.changedAt > 1 {
		if err := w.SaveLayout(); err != nil && w.log != nil {
			w.log.Warn("cannot save the layout", log.F("err", err.Error()))
		}
		w.changedAt = 0
	}
}
