package editor

import (
	"slices"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/modules/input"
	raw "aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

var (
	kindLabels    = []string{"Button", "Axis 1D", "Axis 2D", "Pointer", "Pointer Delta"}
	triggerLabels = []string{"Press", "Release", "Hold", "Tap", "Double Tap", "Toggle"}
	pairNames     = []string{"−", "+"}
	quadNames     = []string{"Up", "Down", "Left", "Right"}
)

type capture struct {
	on      bool
	action  int
	binding int
	part    int
	id      core.ID
}

type Page struct {
	ed    *context.Editor
	sel   int
	query string
	cap   capture
}

func NewPage(ed *context.Editor) *Page {
	return &Page{ed: ed}
}

func (p *Page) Select(i int) {
	p.sel = i
}

func (p *Page) Draw(ed *context.Editor, draft any) bool {
	c, ok := draft.(*input.Config)
	if !ok {
		return false
	}
	if ed != nil {
		p.ed = ed
	}
	ui := p.ed.UI
	changed := p.poll(c)
	changed = p.general(ui, c) || changed
	widgets.Separator(ui)
	changed = p.actions(ui, c, conflicts(c)) || changed
	widgets.Separator(ui)
	changed = p.contexts(ui, c) || changed
	if err := c.Validate(); err != nil {
		r := ui.Cursor().Next(0)
		ui.PushClip(r)
		ui.Text(err.Error(), mathx.Vec2{X: r.X, Y: ui.TextY(r)}, ui.Theme.Error)
		ui.PopClip()
	}
	return changed
}

func actionNames(c *input.Config) []string {
	out := make([]string, len(c.Actions))
	for i, a := range c.Actions {
		out[i] = a.Name
	}
	return out
}

func contextNames(c *input.Config) []string {
	out := make([]string, len(c.Contexts))
	for i, x := range c.Contexts {
		out[i] = x.Name
	}
	return out
}

func unique(base string, taken []string) string {
	for i := 1; ; i++ {
		if n := base + strconv.Itoa(i); !slices.Contains(taken, n) {
			return n
		}
	}
}

func renameAction(c *input.Config, from, to string) {
	for i := range c.Actions {
		if c.Actions[i].Name == from {
			c.Actions[i].Name = to
		}
	}
	for i := range c.Contexts {
		for j, n := range c.Contexts[i].Actions {
			if n == from {
				c.Contexts[i].Actions[j] = to
			}
		}
	}
}

func removeAction(c *input.Config, i int) {
	name := c.Actions[i].Name
	c.Actions = slices.Delete(c.Actions, i, i+1)
	for k := range c.Contexts {
		c.Contexts[k].Actions = slices.DeleteFunc(c.Contexts[k].Actions, func(n string) bool { return n == name })
	}
}

func renameContext(c *input.Config, from, to string) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == from {
			c.Contexts[i].Name = to
		}
	}
	for i, n := range c.Stack {
		if n == from {
			c.Stack[i] = to
		}
	}
}

func removeContext(c *input.Config, i int) {
	name := c.Contexts[i].Name
	c.Contexts = slices.Delete(c.Contexts, i, i+1)
	c.Stack = slices.DeleteFunc(c.Stack, func(n string) bool { return n == name })
}

func conflicts(c *input.Config) map[[2]int]string {
	type use struct{ a, b int }
	out := map[[2]int]string{}
	idx := map[string]int{}
	for i, a := range c.Actions {
		idx[a.Name] = i
	}
	for _, x := range c.Contexts {
		seen := map[input.Control]use{}
		for _, name := range x.Actions {
			ai, ok := idx[name]
			if !ok {
				continue
			}
			a := &c.Actions[ai]
			for bi := range a.Bindings {
				b := &a.Bindings[bi]
				for k := 0; k < input.PartCount(b.Shape); k++ {
					ctl := b.Parts[k]
					if u, dup := seen[ctl]; dup && u.a != ai {
						out[[2]int{ai, bi}] = ctl.Label() + " is also bound to " + c.Actions[u.a].Name + " in " + x.Name
						out[[2]int{u.a, u.b}] = ctl.Label() + " is also bound to " + a.Name + " in " + x.Name
						continue
					}
					seen[ctl] = use{ai, bi}
				}
			}
		}
	}
	return out
}

func hasConflict(conf map[[2]int]string, ai int) bool {
	for k := range conf {
		if k[0] == ai {
			return true
		}
	}
	return false
}

func (p *Page) arm(ui *core.Context, id core.ID, ai, bi, part int) {
	p.cap = capture{on: true, action: ai, binding: bi, part: part, id: id}
	ui.SetFocus(id)
}

func (p *Page) stop() {
	p.cap.on = false
	if p.ed != nil && p.ed.UI != nil && p.ed.UI.Focus() == p.cap.id {
		p.ed.UI.CancelFocus()
	}
}

func (p *Page) capturing(ai, bi, part int) bool {
	cp := &p.cap
	return cp.on && cp.action == ai && cp.binding == bi && cp.part == part
}

func (p *Page) poll(c *input.Config) bool {
	cp := &p.cap
	if !cp.on || p.ed == nil || p.ed.Engine == nil {
		return false
	}
	in := p.ed.Engine.Input()
	if in.KeyPressed(raw.KeyEscape) || in.ButtonPressed(raw.MouseLeft) {
		p.stop()
		return false
	}
	ctl, ok := input.FirstPressed(in, 0xF)
	if !ok {
		return false
	}
	p.stop()
	if cp.action >= len(c.Actions) || cp.binding >= len(c.Actions[cp.action].Bindings) {
		return false
	}
	b := &c.Actions[cp.action].Bindings[cp.binding]
	switch {
	case cp.part < 0:
		b.Mod = ctl
	case b.Parts[cp.part].Device == input.DevStick:
		if ctl.Device != input.DevPadAxis {
			return false
		}
		b.Parts[cp.part] = input.Control{Device: input.DevStick, Code: ctl.Code / 2}
	default:
		b.Parts[cp.part] = ctl
	}
	b.Refresh()
	return true
}

func num(ui *core.Context, label string, v *float32, lo, hi float64, unit string) bool {
	f := float64(*v)
	e := widgets.DragNumber(ui, ui.ID(label), widgets.Field(ui, label), &f, widgets.NumberOpts{Speed: 0.01, Min: lo, Max: hi, HasMin: true, HasMax: true, Prec: 3, Unit: unit})
	if e.Changed {
		*v = float32(f)
	}
	return e.Changed
}

func (p *Page) general(ui *core.Context, c *input.Config) bool {
	widgets.Label(ui, "General")
	changed := false
	n := float64(c.Players)
	if e := widgets.DragNumber(ui, ui.ID("players"), widgets.Field(ui, "Players"), &n, widgets.NumberOpts{Int: true, Speed: 0.05, Min: 1, Max: input.MaxPlayers, HasMin: true, HasMax: true}); e.Changed {
		c.Players = int(n)
		changed = true
	}
	changed = num(ui, "Dead Zone", &c.Deadzone, 0, 0.95, "") || changed
	names := contextNames(c)
	if len(names) == 0 {
		return changed
	}
	widgets.LabelDim(ui, "Start stack: contexts active when the game starts")
	changed = chips(ui, ui.ID("stack"), names, func(i int) bool { return slices.Contains(c.Stack, names[i]) }, func(i int) {
		if k := slices.Index(c.Stack, names[i]); k >= 0 {
			c.Stack = slices.Delete(c.Stack, k, k+1)
		} else {
			c.Stack = append(c.Stack, names[i])
		}
	}) || changed
	return changed
}

func (p *Page) actions(ui *core.Context, c *input.Config, conf map[[2]int]string) bool {
	th := &ui.Theme
	widgets.Label(ui, "Actions")
	changed := false
	bar := ui.Cursor().Next(0)
	bw := float32(100)
	widgets.SearchField(ui, ui.ID("search"), mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, bar.W-bw-4), H: bar.H}, &p.query)
	if widgets.ButtonRect(ui, ui.ID("add"), mathx.Rect{X: bar.X + bar.W - bw, Y: bar.Y, W: bw, H: bar.H}, "Add Action") {
		c.Actions = append(c.Actions, input.Action{Name: unique("Action", actionNames(c)), Kind: input.Button})
		p.sel = len(c.Actions) - 1
		changed = true
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	for i := range c.Actions {
		a := &c.Actions[i]
		if q != "" && !strings.Contains(strings.ToLower(a.Name), q) {
			continue
		}
		r := ui.Cursor().Next(th.RowHeight + 4)
		id := core.IDSeq(ui.ID("row"), i)
		if ui.Clicked(id, r) {
			if p.sel == i {
				p.sel = -1
			} else {
				p.sel = i
			}
		}
		switch {
		case p.sel == i:
			ui.RoundRect(r, th.Radius.Menu, th.SelectionDim)
			ui.Rect(mathx.Rect{X: r.X, Y: r.Y + 4, W: 2, H: r.H - 8}, th.Accent)
		case ui.Hot() == id:
			ui.RoundRect(r, th.Radius.Menu, th.WidgetHot)
		}
		ui.PushClip(r)
		widgets.Chevron(ui, mathx.Rect{X: r.X + 4, Y: r.Y, W: 12, H: r.H}, p.sel == i, th.TextDim)
		col := th.Text
		if hasConflict(conf, i) {
			col = th.Error
		}
		ui.Text(a.Name, mathx.Vec2{X: r.X + 20, Y: ui.TextY(r)}, col)
		x := r.X + max(r.W*0.28, ui.TextSize(a.Name).X+30)
		x += badge(ui, mathx.Vec2{X: x, Y: r.Y}, r.H, kindLabels[a.Kind]) + 8
		for bi := range a.Bindings {
			g := strings.Trim(a.Bindings[bi].Glyph(), "[]")
			w := capW(ui, g)
			if x+w > r.X+r.W-4 {
				ui.Text("…", mathx.Vec2{X: x, Y: ui.TextY(r)}, th.TextDim)
				break
			}
			keycap(ui, core.IDSeq(id, 100+bi), mathx.Rect{X: x, Y: r.Y + 3, W: w, H: r.H - 6}, g, false)
			x += w + 4
		}
		ui.PopClip()
		if p.sel == i {
			changed = p.detail(ui, c, i, conf) || changed
		}
	}
	return changed
}

func (p *Page) detail(ui *core.Context, c *input.Config, i int, conf map[[2]int]string) bool {
	a := &c.Actions[i]
	ui.PushIDInt(i)
	defer ui.PopID()
	cd := beginCard(ui)
	changed := false
	name := a.Name
	if _, ok := widgets.TextFieldRect(ui, ui.ID("name"), widgets.Field(ui, "Name"), &name, false); ok && name != "" && name != a.Name {
		renameAction(c, a.Name, name)
		changed = true
	}
	kind := int(a.Kind)
	if widgets.Combo(ui, ui.ID("kind"), widgets.Field(ui, "Type"), &kind, kindLabels, false) {
		a.Kind = input.Kind(kind)
		kept := a.Bindings[:0]
		for _, b := range a.Bindings {
			if b.Fits(a.Kind) {
				kept = append(kept, b)
			}
		}
		a.Bindings = kept
		changed = true
	}
	switch a.Kind {
	case input.Button:
		tr := int(a.Interaction.Trigger)
		if widgets.Combo(ui, ui.ID("trigger"), widgets.Field(ui, "Interaction"), &tr, triggerLabels, false) {
			a.Interaction.Trigger = input.Trigger(tr)
			changed = true
		}
		switch a.Interaction.Trigger {
		case input.Hold, input.Tap, input.DoubleTap:
			changed = num(ui, "Time", &a.Interaction.Time, 0, 5, "s") || changed
		}
		changed = num(ui, "Buffer", &a.Buffer, 0, 2, "s") || changed
	case input.Axis1D, input.Axis2D:
		changed = num(ui, "Dead Zone", &a.Deadzone, 0, 0.95, "") || changed
		changed = num(ui, "Smoothing", &a.Smoothing, 0, 2, "s") || changed
	case input.PointerDelta:
		changed = num(ui, "Sensitivity", &a.Sensitivity, 0, 20, "") || changed
	}
	if a.Kind != input.Pointer && a.Kind != input.PointerDelta {
		widgets.Label(ui, "Bindings")
		if len(a.Bindings) == 0 {
			widgets.LabelDim(ui, "No bindings yet: the action never fires")
		}
		for j := 0; j < len(a.Bindings); j++ {
			ch, removed := p.bindingRow(ui, c, i, j, conf)
			changed = changed || ch
			if removed {
				a.Bindings = slices.Delete(a.Bindings, j, j+1)
				p.cap.on = false
				changed = true
				break
			}
		}
		changed = p.addButtons(ui, a, i) || changed
	}
	row := ui.Cursor().Next(0)
	if widgets.ButtonRect(ui, ui.ID("rmact"), mathx.Rect{X: row.X + row.W - 130, Y: row.Y, W: 130, H: row.H}, "Remove Action") {
		removeAction(c, i)
		p.sel = -1
		changed = true
	}
	endCard(ui, cd)
	return changed
}

func partNames(s input.Shape) []string {
	switch s {
	case input.Pair:
		return pairNames
	case input.Quad:
		return quadNames
	}
	return []string{""}
}

func (p *Page) bindingRow(ui *core.Context, c *input.Config, ai, bi int, conf map[[2]int]string) (bool, bool) {
	th := &ui.Theme
	a := &c.Actions[ai]
	b := &a.Bindings[bi]
	ui.PushIDInt(100 + bi)
	defer ui.PopID()
	r := ui.Cursor().Next(th.RowHeight + 4)
	ui.RoundRect(r, th.Radius.Field, th.PanelAlt)
	if why, bad := conf[[2]int{ai, bi}]; bad {
		bg := th.Error
		bg.A = 0.18
		ui.RoundRect(r, th.Radius.Field, bg)
		tip := ui.ID("conflict")
		ui.MouseOver(tip, r)
		widgets.Tooltip(ui, tip, why)
	}
	ui.Text(deviceOf(b), mathx.Vec2{X: r.X + 8, Y: ui.TextY(r)}, th.TextDim)
	x := r.X + 84
	changed := false
	names := partNames(b.Shape)
	inner := mathx.Rect{Y: r.Y + 3, H: r.H - 6}
	for k := 0; k < input.PartCount(b.Shape); k++ {
		if names[k] != "" {
			ui.Text(names[k], mathx.Vec2{X: x, Y: ui.TextY(r)}, th.TextDim)
			x += ui.TextSize(names[k]).X + 4
		}
		label := b.Parts[k].Label()
		armed := p.capturing(ai, bi, k)
		if armed {
			label = "Press…"
		}
		id := ui.IDInt(k)
		w := capW(ui, label)
		inner.X, inner.W = x, w
		if keycap(ui, id, inner, label, armed) {
			p.arm(ui, id, ai, bi, k)
		}
		widgets.Tooltip(ui, id, "Click, then press a key, a mouse button or a pad control")
		x += w + 8
	}
	if a.Kind == input.Button && b.Shape == input.Single {
		armed := p.capturing(ai, bi, -1)
		label := "+ Modifier"
		if !b.Mod.IsZero() {
			label = "with " + b.Mod.Label()
		}
		if armed {
			label = "Press…"
		}
		id := ui.ID("mod")
		w := capW(ui, label)
		inner.X, inner.W = x, w
		if keycap(ui, id, inner, label, armed) {
			p.arm(ui, id, ai, bi, -1)
		}
		x += w + 4
		if !b.Mod.IsZero() && widgets.IconButton(ui, ui.ID("nomod"), mathx.Rect{X: x, Y: r.Y + 2, W: r.H - 4, H: r.H - 4}, "ui.minus", "Remove the modifier") {
			b.Mod = input.Control{}
			b.Refresh()
			changed = true
		}
	}
	end := r.X + r.W - r.H
	if a.Kind == input.Axis1D || a.Kind == input.Axis2D {
		sr := mathx.Rect{X: end - 84, Y: r.Y, W: 80, H: r.H}
		if widgets.Switch(ui, ui.ID("invert"), sr, &b.Invert, "Invert") {
			b.Refresh()
			changed = true
		}
	}
	removed := widgets.IconButton(ui, ui.ID("rm"), mathx.Rect{X: end + 2, Y: r.Y + 2, W: r.H - 4, H: r.H - 4}, "ui.close", "Remove binding")
	return changed, removed
}

func (p *Page) addButtons(ui *core.Context, a *input.Action, ai int) bool {
	type form struct {
		label, text string
		capture     bool
	}
	var forms []form
	switch a.Kind {
	case input.Button:
		forms = []form{{"Key", "Key.Space", true}, {"Mouse Button", "Mouse.Right", true}, {"Pad Button", "Pad.A", true}}
	case input.Axis1D:
		forms = []form{{"Two Keys (− / +)", "Key.A/Key.D", false}, {"Pad Axis", "Pad.LeftX", true}, {"Pad Trigger", "Pad.RT", true}}
	case input.Axis2D:
		forms = []form{{"Four Keys (WASD)", "Key.W/Key.S/Key.A/Key.D", false}, {"Arrow Keys", "Key.Up/Key.Down/Key.Left/Key.Right", false}, {"Stick", "Pad.LeftStick", false}}
	}
	row := ui.Cursor().Next(0)
	id := ui.ID("addbind")
	br := mathx.Rect{X: row.X, Y: row.Y, W: 130, H: row.H}
	if widgets.ButtonRect(ui, id, br, "+ Add Binding") {
		ui.OpenPopup(id, mathx.Vec2{X: br.X, Y: br.Y + br.H})
	}
	changed := false
	if ui.BeginPopup(id, 200) {
		for _, f := range forms {
			if widgets.Entry(ui, widgets.Item{Label: f.label, Enabled: true}) {
				if b, err := input.ParseBinding(f.text); err == nil {
					a.Bindings = append(a.Bindings, b)
					changed = true
					if f.capture {
						p.arm(ui, id, ai, len(a.Bindings)-1, 0)
					}
				}
			}
		}
		ui.EndPopup()
	}
	return changed
}

func (p *Page) contexts(ui *core.Context, c *input.Config) bool {
	widgets.Label(ui, "Contexts")
	widgets.LabelWrapped(ui, "The stack reads higher priority first; a consuming context hides its controls from the ones below.", true)
	changed := false
	names := actionNames(c)
	remove := -1
	for i := range c.Contexts {
		x := &c.Contexts[i]
		ui.PushIDInt(1000 + i)
		cd := beginCard(ui)
		row := ui.Cursor().Next(0)
		pw := float32(110)
		nr := mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-pw-row.H-12), H: row.H}
		name := x.Name
		if _, ok := widgets.TextFieldRect(ui, ui.ID("name"), nr, &name, false); ok && name != "" && name != x.Name {
			renameContext(c, x.Name, name)
			changed = true
		}
		prio := float64(x.Priority)
		pr := mathx.Rect{X: nr.X + nr.W + 6, Y: row.Y, W: pw, H: row.H}
		if e := widgets.DragNumber(ui, ui.ID("prio"), pr, &prio, widgets.NumberOpts{Int: true, Speed: 0.1, Suffix: " priority"}); e.Changed {
			x.Priority = int(prio)
			changed = true
		}
		if widgets.IconButton(ui, ui.ID("remove"), mathx.Rect{X: row.X + row.W - row.H, Y: row.Y, W: row.H, H: row.H}, "ui.close", "Remove context") {
			remove = i
		}
		flags := ui.Cursor().Next(0)
		if widgets.Switch(ui, ui.ID("consume"), mathx.Rect{X: flags.X, Y: flags.Y, W: flags.W / 2, H: flags.H}, &x.Consume, "Consume controls") {
			changed = true
		}
		if widgets.Switch(ui, ui.ID("release"), mathx.Rect{X: flags.X + flags.W/2, Y: flags.Y, W: flags.W / 2, H: flags.H}, &x.Release, "Release on leave") {
			changed = true
		}
		if len(names) > 0 {
			changed = chips(ui, ui.ID("acts"), names, func(k int) bool { return slices.Contains(x.Actions, names[k]) }, func(k int) {
				if j := slices.Index(x.Actions, names[k]); j >= 0 {
					x.Actions = slices.Delete(x.Actions, j, j+1)
				} else {
					x.Actions = append(x.Actions, names[k])
				}
			}) || changed
		}
		if len(x.Actions) == 0 {
			widgets.LabelDim(ui, "No actions: this context reads nothing")
		}
		endCard(ui, cd)
		ui.PopID()
	}
	if remove >= 0 {
		removeContext(c, remove)
		changed = true
	}
	if widgets.Button(ui, "Add Context") {
		c.Contexts = append(c.Contexts, input.Context{Name: unique("Context", contextNames(c))})
		changed = true
	}
	return changed
}
