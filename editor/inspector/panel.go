package inspector

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/clipboard"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/history"
	"aego/editor/history/commands"
	"aego/editor/props"
	"aego/editor/workspace"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

type Panel struct {
	reg          *Registry
	tx           *history.Transaction
	txDoc        document.ID
	collapsed    map[core.ID]bool
	scroll       float32
	content      float32
	search       string
	pick         string
	nodes        []ids.NodeID
	types        []schema.TypeID
	labels       map[string]string
	typeNames    map[schema.TypeID][2]string
	live         map[core.ID]schema.Value
	liveT        *scene.Transform2D
	euler        *props.EulerCache
	focusSearch  bool
	hoverField   fieldRef
	hoverType    schema.TypeID
	menuField    fieldRef
	rightUsed    bool
	fieldMenuID  core.ID
	bgMenuID     core.ID
	scriptNames  []string
	scriptSearch string
	focusScript  bool
}

const (
	Scope        = "inspector"
	CommandCopy  = "inspector.copy"
	CommandPaste = "inspector.paste"
)

type fieldRef struct {
	t  schema.TypeID
	f  schema.FieldID
	ok bool
}

func New(reg *Registry) *Panel {
	return &Panel{
		reg:       reg,
		collapsed: map[core.ID]bool{},
		labels:    map[string]string{},
		typeNames: map[schema.TypeID][2]string{},
		live:      map[core.ID]schema.Value{},
		euler:     props.NewEulerCache(),
	}
}

func (p *Panel) ID() string {
	return workspace.Inspector
}

func (p *Panel) Title() string {
	return "Inspector"
}

func (p *Panel) Scope() string {
	return Scope
}

func (p *Panel) Draw(ctx *context.Editor, r mathx.Rect) {
	ui := ctx.UI
	d := ctx.Doc()
	p.settle(ctx, d)
	p.hoverField, p.hoverType, p.rightUsed = fieldRef{}, 0, false
	p.fieldMenuID, p.bgMenuID = ui.ID("fieldmenu"), ui.ID("bgmenu")
	if d == nil {
		widgets.LabelDim(ui, "No scene open")
		return
	}
	s := d.Scene()
	p.nodes = p.nodes[:0]
	for _, id := range ctx.Selected() {
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
			p.nodes = append(p.nodes, id)
		}
	}
	if len(p.nodes) == 0 {
		widgets.LabelDim(ui, "Nothing selected")
		return
	}
	th := &ui.Theme
	if ui.MouseOver(ui.ID("scroll"), r) {
		if w := ui.WheelY(); w != 0 {
			p.scroll -= w * th.RowHeight * 3
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-r.H+th.Padding*2))
	cur := ui.Cursor()
	top := cur.Y
	cur.Y -= p.scroll
	p.header(ctx, d)
	p.transform(ctx, d)
	p.components(ctx, d)
	p.scripts(ctx, d)
	p.addComponent(ctx, d)
	p.addScript(ctx, d)
	p.content = cur.Y + p.scroll - top
	p.menus(ctx, d, r)
	p.dropScripts(ctx, r)
}

func (p *Panel) menus(ctx *context.Editor, d *document.SceneDocument, r mathx.Rect) {
	ui := ctx.UI
	if !p.rightUsed && ui.MousePressed(input.MouseRight) && ui.Hit(r) && !ui.MouseInOverlay() {
		widgets.OpenContextMenu(ui, p.bgMenuID)
	}
	if widgets.ContextMenu(ui, p.bgMenuID) {
		can := clipboard.Peek(ctx) == clipboard.KindComponent
		if widgets.MenuItem(ui, "Paste Component", ctx.Commands.Shortcut(CommandPaste), can, false) {
			if err := clipboard.PasteComponent(ctx, d, p.nodes); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			}
		}
		widgets.EndContextMenu(ui)
	}
	if widgets.ContextMenu(ui, p.fieldMenuID) {
		p.fieldMenu(ctx, d)
		widgets.EndContextMenu(ui)
	}
}

func (p *Panel) CanCopy() bool {
	return p.hoverField.ok || p.hoverType != 0
}

func (p *Panel) CopyHovered(ctx *context.Editor) error {
	d := ctx.Doc()
	if d == nil || len(p.nodes) == 0 {
		return nil
	}
	if p.hoverField.ok {
		if td, fd, ok := p.lookup(d, p.hoverField); ok && !p.copyTexture(ctx, d, td, fd) {
			p.copyValue(ctx, d, td, fd)
		}
		return nil
	}
	if p.hoverType != 0 {
		return clipboard.CopyComponent(ctx, d.Scene(), p.nodes[0], p.hoverType)
	}
	return nil
}

func (p *Panel) PasteHovered(ctx *context.Editor) error {
	d := ctx.Doc()
	if d == nil || len(p.nodes) == 0 {
		return nil
	}
	if p.hoverField.ok {
		if td, fd, ok := p.lookup(d, p.hoverField); ok {
			if v, ok := clipboard.ReadValue(ctx, fd); ok {
				return d.Props.Set(nil, p.nodes, td.ID, fd.ID, v)
			}
			if done, err := p.pasteAsset(ctx, d, td, fd); done {
				return err
			}
		}
	}
	if clipboard.Peek(ctx) == clipboard.KindComponent {
		return clipboard.PasteComponent(ctx, d, p.nodes)
	}
	ctx.Notify(context.LevelInfo, "The clipboard does not fit this field")
	return nil
}

func (p *Panel) settle(ctx *context.Editor, d *document.SceneDocument) {
	if p.tx == nil {
		return
	}
	ui := ctx.UI
	if d == nil || d.ID != p.txDoc || (!ui.MouseDown(input.MouseLeft) && ui.Focus() == 0) {
		p.tx.Commit()
		p.tx = nil
	}
}

func (p *Panel) apply(ctx *context.Editor, e widgets.Edit, label string, fn func(*history.Transaction) error) {
	h := ctx.History()
	if h == nil {
		return
	}
	if e.Begin && p.tx == nil {
		p.tx = h.Begin(label)
		p.txDoc = ctx.Doc().ID
	}
	if e.Changed {
		tx, own := p.tx, false
		if tx == nil {
			tx, own = h.Begin(label), true
		}
		if err := fn(tx); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
		if own {
			tx.Commit()
		}
	}
	if e.End && p.tx != nil {
		p.tx.Commit()
		p.tx = nil
	}
}

func humanize(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if r == '_' {
			b.WriteByte(' ')
			continue
		}
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		prev := rs[i-1]
		var next rune
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		if unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (p *Panel) labelOf(fd *schema.FieldDescriptor) string {
	if fd.Hints.DisplayName != "" {
		return fd.Hints.DisplayName
	}
	if l, ok := p.labels[fd.Name]; ok {
		return l
	}
	l := humanize(fd.Name)
	p.labels[fd.Name] = l
	return l
}

func triCheck(ui *core.Context, id core.ID, r mathx.Rect, state int) bool {
	th := &ui.Theme
	clicked := ui.Clicked(id, r)
	box := mathx.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}
	switch state {
	case 1:
		ui.Rect(box, th.Accent)
		widgets.CheckMark(ui, box, th.Background)
	case 2:
		ui.Rect(box, ui.WidgetColor(id))
		ui.Outline(box, th.Border)
		ui.Rect(mathx.Rect{X: box.X + 3, Y: box.Y + box.H/2 - 1, W: box.W - 6, H: 2}, th.Accent)
	default:
		ui.Rect(box, ui.WidgetColor(id))
		ui.Outline(box, th.Border)
	}
	return clicked
}

func (p *Panel) header(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	th := &ui.Theme
	ui.PushID("node")
	defer ui.PopID()
	c := p.beginCard(ctx, "node", "Node", "", "cat.node", "", false)
	defer p.endCard(ui, c)
	if !c.open {
		return
	}
	s := d.Scene()
	first := s.Handle(p.nodes[0])
	row := ui.Cursor().Next(0)
	state := 0
	if s.Enabled(first) {
		state = 1
	}
	for _, id := range p.nodes[1:] {
		if s.Enabled(s.Handle(id)) != (state == 1) {
			state = 2
			break
		}
	}
	if triCheck(ui, ui.ID("enabled"), mathx.Rect{X: row.X, Y: row.Y, W: row.H, H: row.H}, state) {
		if err := d.Props.SetEnabled(nil, p.nodes, state != 1); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	nr := mathx.Rect{X: row.X + row.H + 4, Y: row.Y, W: row.W - row.H - 4, H: row.H}
	if len(p.nodes) == 1 {
		name := s.Name(first)
		if _, committed := widgets.TextFieldRect(ui, ui.ID("name"), nr, &name, false); committed {
			if err := d.Props.Rename(nil, p.nodes[0], name); err != nil {
				ctx.Notify(context.LevelError, err.Error())
			}
		}
	} else {
		ui.TextIn(nr, strconv.Itoa(len(p.nodes))+" nodes", th.TextDim, core.AlignLeft)
	}
	mask := s.Layers(first)
	mixed := false
	for _, id := range p.nodes[1:] {
		if s.Layers(s.Handle(id)) != mask {
			mixed = true
			break
		}
	}
	if widgets.MaskField(ui, ui.ID("layers"), widgets.Field(ui, "Layers"), &mask, nil, mixed) {
		if err := d.Props.SetLayers(nil, p.nodes, mask); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
	}
	p.spaceRow(ctx, d)
}

func (p *Panel) editing(ui *core.Context, key core.ID) bool {
	a, f := ui.Active(), ui.Focus()
	if a == key || f == key || ui.PopupOpen(key) {
		return true
	}
	for i := 1; i <= 4; i++ {
		s := core.IDSeq(key, i)
		if a == s || f == s {
			return true
		}
	}
	return false
}

func (p *Panel) transform2D(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	ui.PushID("transform")
	defer ui.PopID()
	card := p.beginCard(ctx, "transform", "Transform", "", "cat.transform", "Reset", false)
	defer p.endCard(ui, card)
	if card.action {
		p.resetTransform(ctx, d)
	}
	if !card.open {
		return
	}
	posID, rotID, scaleID := ui.ID("pos"), ui.ID("rot"), ui.ID("scale")
	t, mixed := d.Props.CommonTransform(p.nodes)
	if p.liveT != nil && (p.editing(ui, posID) || p.editing(ui, rotID) || p.editing(ui, scaleID)) {
		t, mixed = *p.liveT, [5]bool{}
	} else {
		p.liveT = nil
	}
	keep := func(e widgets.Edit, nt scene.Transform2D) {
		if e.Begin || e.Changed {
			p.liveT = &nt
		}
		if e.End {
			p.liveT = nil
		}
	}

	pos := t.Position
	e := widgets.Vec2Field(ui, posID, widgets.Field(ui, "Position"), &pos, widgets.NumberOpts{Speed: 0.05}, mixed[0], mixed[1])
	if e.Any() {
		x, y := pos.X != t.Position.X, pos.Y != t.Position.Y
		nt := t
		nt.Position = pos
		keep(e, nt)
		p.apply(ctx, e, "Move", func(tx *history.Transaction) error {
			return d.Props.EditTransform(tx, p.nodes, func(tr *scene.Transform2D) {
				if x {
					tr.Position.X = pos.X
				}
				if y {
					tr.Position.Y = pos.Y
				}
			})
		})
	}

	deg := float64(mathx.Rad2Deg(t.Rotation))
	e = widgets.DragNumber(ui, rotID, widgets.Field(ui, "Rotation"), &deg, widgets.NumberOpts{Speed: 0.5, Prec: 2, Suffix: "°", Mixed: mixed[2]})
	if e.Any() {
		rad := mathx.Deg2Rad(float32(deg))
		nt := t
		nt.Rotation = rad
		keep(e, nt)
		p.apply(ctx, e, "Rotate", func(tx *history.Transaction) error {
			return d.Props.EditTransform(tx, p.nodes, func(tr *scene.Transform2D) {
				tr.Rotation = rad
			})
		})
	}

	sc := t.Scale
	e = widgets.Vec2Field(ui, scaleID, widgets.Field(ui, "Scale"), &sc, widgets.NumberOpts{Speed: 0.01}, mixed[3], mixed[4])
	if e.Any() {
		x, y := sc.X != t.Scale.X, sc.Y != t.Scale.Y
		nt := t
		nt.Scale = sc
		keep(e, nt)
		p.apply(ctx, e, "Scale", func(tx *history.Transaction) error {
			return d.Props.EditTransform(tx, p.nodes, func(tr *scene.Transform2D) {
				if x {
					tr.Scale.X = sc.X
				}
				if y {
					tr.Scale.Y = sc.Y
				}
			})
		})
	}
}

func (p *Panel) components(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	th := &ui.Theme
	s := d.Scene()
	reg := s.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	first := s.Handle(p.nodes[0])
	p.types = p.types[:0]
	scriptT := schema.HashName(scriptcomp.ScriptName)
	s.ComponentsOf(first, func(t schema.TypeID) {
		if (hasUnknown && t == unknown.ID) || t == scriptT {
			return
		}
		for _, id := range p.nodes[1:] {
			if _, ok := s.GetByType(s.Handle(id), t); !ok {
				return
			}
		}
		p.types = append(p.types, t)
	})
	for _, sec := range groupComponents(p.reg, reg, p.types) {
		r := ui.Cursor().Next(ui.LineHeight() + 4)
		ui.Text(sec.Category, mathx.Vec2{X: r.X + 2, Y: r.Y + 3}, th.TextDisabled)
		for _, t := range sec.Types {
			p.component(ctx, d, t)
		}
	}
	if hasUnknown && len(p.nodes) == 1 {
		if u, ok := scene.Get[sceneio.Unknown](s, first); ok {
			for _, c := range u.Components {
				widgets.LabelDim(ui, c.TypeName+" (not loaded, kept as is)")
			}
		}
	}
}

func (p *Panel) component(ctx *context.Editor, d *document.SceneDocument, t schema.TypeID) {
	ui := ctx.UI
	td, ok := d.Scene().Registry().ByID(t)
	if !ok {
		return
	}
	ui.PushRawID(core.ID(t))
	defer ui.PopID()
	meta := p.reg.Meta(td)
	c := p.beginCard(ctx, td.FullName, meta.DisplayName, meta.Badge, meta.Icon, "", true)
	c.typ = t
	if ui.Hot() == c.hid && ui.KeyPressed(input.KeyF1) && ctx.ShowDocs != nil {
		ctx.ShowDocs(td.FullName, meta.DisplayName)
	}
	defer func() { p.endCard(ui, c) }()
	menuID := ui.ID("menu")
	if ui.RightClicked(c.hid, c.hdr) {
		p.rightUsed = true
		widgets.OpenContextMenu(ui, menuID)
	} else if c.more {
		widgets.OpenContextMenu(ui, menuID)
	}
	if widgets.ContextMenu(ui, menuID) {
		p.componentMenu(ctx, d, td)
		widgets.EndContextMenu(ui)
	}
	if !c.open {
		return
	}
	def := func() { p.fields(ctx, d, td) }
	custom, ok := p.reg.Custom(t)
	if !ok {
		def()
		return
	}
	mark := ui.Mark()
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				ui.Restore(mark)
				msg := fmt.Sprint(rec)
				p.reg.Disable(t, msg)
				ctx.Notify(context.LevelError, "inspector for "+td.FullName+" crashed: "+msg)
			}
		}()
		custom.Draw(ctx, p.nodes, def)
	}()
}

func (p *Panel) remove(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor) {
	tx := d.History.Begin("Remove " + td.FullName)
	for _, id := range p.nodes {
		if err := tx.Exec(&commands.RemoveComponent{Node: id, Type: td.ID, Name: td.FullName}); err != nil {
			tx.Abort()
			ctx.Notify(context.LevelError, err.Error())
			return
		}
	}
	tx.Commit()
}

func (p *Panel) reset(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor) {
	tx := d.History.Begin("Reset " + td.FullName)
	for _, id := range p.nodes {
		if err := tx.Exec(&commands.RemoveComponent{Node: id, Type: td.ID, Name: td.FullName}); err != nil {
			tx.Abort()
			ctx.Notify(context.LevelError, err.Error())
			return
		}
		if err := tx.Exec(&commands.AddComponent{Node: id, Type: td.ID, Name: td.FullName, Values: p.reg.Defaults(td.ID)}); err != nil {
			tx.Abort()
			ctx.Notify(context.LevelError, err.Error())
			return
		}
	}
	tx.Commit()
}

func (p *Panel) blocked(s *scene.Scene, reg *schema.Registry, td *schema.TypeDescriptor) string {
	for _, name := range td.Requires {
		req, ok := reg.ByName(name)
		if !ok {
			return "needs " + name
		}
		for _, n := range p.nodes {
			if _, has := s.GetByType(s.Handle(n), req.ID); !has {
				return "needs " + p.reg.Meta(req).DisplayName
			}
		}
	}
	if td.SceneUnique && (len(p.nodes) > 1 || inScene(s, td.ID)) {
		return "one per scene"
	}
	return ""
}

func inScene(s *scene.Scene, t schema.TypeID) bool {
	for _, h := range s.PreOrder() {
		if _, ok := s.GetByType(h, t); ok && s.IsAlive(h) {
			return true
		}
	}
	return false
}

func (p *Panel) addComponent(ctx *context.Editor, d *document.SceneDocument) {
	ui := ctx.UI
	th := &ui.Theme
	s := d.Scene()
	reg := s.Registry()
	row := ui.Cursor().Next(th.RowHeight + 4)
	id := ui.ID("add")
	if ui.Pressed(id, row) {
		if ui.PopupOpen(id) {
			ui.ClosePopup(id)
		} else {
			ui.CloseAllPopups()
			ui.OpenPopup(id, mathx.Vec2{X: row.X, Y: row.Y + row.H})
			p.search = ""
			p.focusSearch = true
		}
	}
	ui.Rect(row, ui.WidgetColor(id))
	ui.Outline(row, th.AccentDim)
	ui.TextIn(row, "Add Component", th.Text, core.AlignCenter)
	if !ui.BeginPopup(id, row.W) {
		return
	}
	sid := ui.ID("addsearch")
	if p.focusSearch {
		p.focusSearch = false
		ui.SetFocus(sid)
		ui.EditBuffer(sid, p.search).SelectAll()
	}
	widgets.SearchField(ui, sid, ui.Cursor().Next(0), &p.search)
	q := strings.ToLower(strings.TrimSpace(p.search))
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	var candidates []schema.TypeID
	for _, td := range reg.Components() {
		if (hasUnknown && td.ID == unknown.ID) || td.FullName == scriptcomp.ScriptName {
			continue
		}
		if !d.Props.Fits(p.nodes, td.ID) {
			continue
		}
		missing := false
		for _, n := range p.nodes {
			if _, ok := s.GetByType(s.Handle(n), td.ID); !ok {
				missing = true
				break
			}
		}
		if !missing {
			continue
		}
		meta := p.reg.Meta(td)
		if q != "" && !strings.Contains(strings.ToLower(meta.DisplayName), q) && !strings.Contains(strings.ToLower(td.FullName), q) {
			continue
		}
		candidates = append(candidates, td.ID)
	}
	var first *schema.TypeDescriptor
	for _, sec := range groupComponents(p.reg, reg, candidates) {
		widgets.LabelDim(ui, sec.Category)
		for _, t := range sec.Types {
			td, ok := reg.ByID(t)
			if !ok {
				continue
			}
			label := p.reg.Meta(td).DisplayName
			if why := p.blocked(s, reg, td); why != "" {
				widgets.LabelDim(ui, label+" — "+why)
				continue
			}
			if first == nil {
				first = td
			}
			if widgets.Selectable(ui, core.IDSeq(id, int(t)), label, td == first && q != "") {
				p.add(ctx, d, td)
				ui.ClosePopup(id)
			}
		}
	}
	if len(candidates) == 0 {
		widgets.LabelDim(ui, "No components match")
	}
	if first != nil && q != "" && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter)) {
		p.add(ctx, d, first)
		ui.ClosePopup(id)
	}
	ui.EndPopup()
}

func (p *Panel) add(ctx *context.Editor, d *document.SceneDocument, td *schema.TypeDescriptor) {
	s := d.Scene()
	tx := d.History.Begin("Add " + td.FullName)
	for _, n := range p.nodes {
		if _, ok := s.GetByType(s.Handle(n), td.ID); ok {
			continue
		}
		if err := tx.Exec(&commands.AddComponent{Node: n, Type: td.ID, Name: td.FullName, Values: p.reg.Defaults(td.ID)}); err != nil {
			tx.Abort()
			ctx.Notify(context.LevelError, err.Error())
			return
		}
	}
	tx.Commit()
}
