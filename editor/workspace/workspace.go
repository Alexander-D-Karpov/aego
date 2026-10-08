package workspace

import (
	"fmt"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/state"
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
	Profiler  = "profiler"
)

func DefaultLayout() *dock.Tree {
	return dock.Columns(
		[]string{Hierarchy},
		[]string{Assets},
		[]string{Scene, Game},
		[]string{Console, Profiler},
		[]string{Inspector},
	)
}

type Panel interface {
	ID() string
	Title() string
	Draw(ctx *context.Editor, r mathx.Rect)
}

type Scoped interface {
	Scope() string
}

type Workspace struct {
	Tree      *dock.Tree
	root      string
	log       *log.ChannelLogger
	panels    map[string]Panel
	order     []string
	failed    map[string]string
	hovered   string
	focused   string
	changedAt float64
	pending   bool
}

func New(root string, lg *log.ChannelLogger) *Workspace {
	w := &Workspace{
		root:   root,
		log:    lg,
		panels: map[string]Panel{},
		failed: map[string]string{},
	}
	w.Tree = DefaultLayout()
	w.bind()
	return w
}

func (w *Workspace) bind() {
	w.Tree.Title = func(id string) string {
		if p, ok := w.panels[id]; ok {
			return p.Title()
		}
		return id
	}
}

func (w *Workspace) Register(p Panel) error {
	id := p.ID()
	if id == "" {
		return errs.New(errs.EditorPanel, "panel needs an id")
	}
	if _, dup := w.panels[id]; dup {
		return errs.New(errs.EditorPanel, "duplicate panel id", log.F("panel", id))
	}
	w.panels[id] = p
	w.order = append(w.order, id)
	return nil
}

func (w *Workspace) Has(id string) bool {
	_, ok := w.panels[id]
	return ok
}

func (w *Workspace) Unregister(id string) {
	if _, ok := w.panels[id]; !ok {
		return
	}
	delete(w.panels, id)
	for i, o := range w.order {
		if o == id {
			w.order = append(w.order[:i], w.order[i+1:]...)
			break
		}
	}
	w.Tree.Undock(id)
}

func (w *Workspace) Panel(id string) (Panel, bool) {
	p, ok := w.panels[id]
	return p, ok
}

func (w *Workspace) Panels() []Panel {
	out := make([]Panel, 0, len(w.order))
	for _, id := range w.order {
		out = append(out, w.panels[id])
	}
	return out
}

func (w *Workspace) retain() {
	w.Tree.Retain(func(id string) bool {
		_, ok := w.panels[id]
		return ok
	})
}

func (w *Workspace) LoadLayout() {
	if data, ok := state.LoadLayout(w.root); ok {
		if t, err := dock.Load(data); err == nil {
			w.Tree = t
		} else if w.log != nil {
			w.log.Warn("layout is unreadable, using the default", log.F("err", err.Error()))
		}
	}
	w.bind()
	w.retain()
	if len(w.Tree.Panels()) == 0 {
		w.ResetLayout()
	}
}

func (w *Workspace) SaveLayout() error {
	return state.SaveLayout(w.root, w.Tree.Save())
}

func (w *Workspace) ResetLayout() {
	if err := state.RemoveLayout(w.root); err != nil && w.log != nil {
		w.log.Warn("cannot remove the saved layout", log.F("err", err.Error()))
	}
	w.Tree = DefaultLayout()
	w.bind()
	w.retain()
	w.changedAt = 0
	w.pending = false
}

func (w *Workspace) Visible(id string) bool {
	return w.Tree.Has(id)
}

func (w *Workspace) Show(id string) {
	if !w.Has(id) {
		return
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
	if p, ok := w.panels[id]; ok {
		if s, ok := p.(Scoped); ok {
			return s.Scope()
		}
	}
	return id
}

func (w *Workspace) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	w.Tree.Draw(ui, r, func(id string, pr mathx.Rect) {
		w.drawPanel(ctx, id, pr)
	})
	w.hovered, _ = w.Tree.PanelAt(ui.Mouse())
	if ui.MousePressed(input.MouseLeft) || ui.MousePressed(input.MouseRight) {
		w.focused = w.hovered
	}
}

func (w *Workspace) drawPanel(ctx *context.Editor, id string, r mathx.Rect) {
	ui := ctx.UI
	p, ok := w.panels[id]
	if !ok {
		ui.TextIn(r, "unknown panel "+id, ui.Theme.TextDim, core.AlignCenter)
		return
	}
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
	mark := ui.Mark()
	defer func() {
		if rec := recover(); rec != nil {
			ui.Restore(mark)
			msg := fmt.Sprint(rec)
			w.failed[id] = msg
			ctx.Notify(context.LevelError, "panel "+id+" crashed: "+msg)
		}
	}()
	ui.Region(r)
	p.Draw(ctx, r)
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
