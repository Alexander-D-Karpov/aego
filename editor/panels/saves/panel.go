package saves

import (
	"strconv"
	"strings"
	"time"

	"aego/contracts/persistence"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/save"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "saves"

type Deps struct {
	Store   func() *save.Store
	Service func() persistence.Service
	Label   func(id ids.AssetID) string
	Notices services.Notices
}

type Panel struct {
	d       Deps
	f       *services.Frame
	list    []save.SlotInfo
	at      float64
	sel     string
	doc     *save.Doc
	docSlot string
	docMod  time.Time
	docErr  error
	scroll  float32
	dscroll float32
	slot    string
	prefix  string
	typed   bool
}

func New(d Deps) *Panel {
	return &Panel{d: d, at: -10, slot: "slot1", prefix: "slot"}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return i18n.Tr("editor.saves.title", "Saves")
}

func (p *Panel) Refresh() {
	p.at = -10
}

func (p *Panel) Select(slot string) {
	p.sel = slot
}

func (p *Panel) notify(l context.Level, s string) {
	if p.d.Notices != nil {
		p.d.Notices.Notify(l, s)
	}
}

func (p *Panel) service() persistence.Service {
	if p.d.Service == nil {
		return nil
	}
	return p.d.Service()
}

func splitSlot(slot string) (string, int, bool) {
	i := len(slot)
	for i > 0 && slot[i-1] >= '0' && slot[i-1] <= '9' {
		i--
	}
	if i == len(slot) {
		return slot, 0, false
	}
	n, err := strconv.Atoi(slot[i:])
	return slot[:i], n, err == nil
}

// NextSlot is prefix followed by one more than the highest number already used with that prefix.
func NextSlot(prefix string, taken []string) string {
	if prefix == "" {
		prefix = "slot"
	}
	top := 0
	for _, t := range taken {
		if p, n, ok := splitSlot(t); ok && p == prefix {
			top = max(top, n)
		}
	}
	return prefix + strconv.Itoa(top+1)
}

func (p *Panel) taken() []string {
	out := make([]string, 0, len(p.list))
	for i := range p.list {
		out = append(out, p.list[i].Slot)
	}
	return out
}

func (p *Panel) exists(slot string) bool {
	for i := range p.list {
		if p.list[i].Slot == slot {
			return true
		}
	}
	return false
}

func (p *Panel) suggest() {
	if !p.typed {
		p.slot = NextSlot(p.prefix, p.taken())
	}
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	th := &ui.Theme
	var st *save.Store
	if p.d.Store != nil {
		st = p.d.Store()
	}
	if st == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.saves.saves_are_not_available_this", "Saves are not available: this run has no player profile"))
		return
	}
	if f.Now-p.at > 1 {
		p.list, p.at = st.List(), f.Now
		p.suggest()
	}
	p.toolbar(st)
	cur := ui.Cursor()
	area := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(th.RowHeight*4, r.Y+r.H-cur.Y)}
	list, side := area, mathx.Rect{}
	if area.W >= 640 {
		list.W = area.W * 0.6
		side = mathx.Rect{X: list.X + list.W + th.Spacing*2 + 1, Y: area.Y, W: area.W - list.W - th.Spacing*2 - 1, H: area.H}
		ui.Rect(mathx.Rect{X: list.X + list.W + th.Spacing, Y: area.Y, W: 1, H: area.H}, th.Border)
	} else {
		list.H = max(th.RowHeight*3, area.H*0.45)
		side = mathx.Rect{X: area.X, Y: list.Y + list.H + th.Spacing*2 + 1, W: area.W, H: area.Y + area.H - list.Y - list.H - th.Spacing*2 - 1}
		ui.Rect(mathx.Rect{X: area.X, Y: list.Y + list.H + th.Spacing, W: area.W, H: 1}, th.Border)
	}
	p.slots(list)
	p.details(st, side)
	cur.Y = area.Y + area.H
}

func (p *Panel) slots(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	hdr := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}
	p.header(hdr)
	body := mathx.Rect{X: r.X, Y: hdr.Y + hdr.H, W: r.W, H: max(0, r.H-hdr.H)}
	widgets.VirtualList(ui, ui.ID("slots"), body, len(p.list), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.row(&p.list[i], rr)
	})
	if len(p.list) == 0 {
		ui.TextIn(body, i18n.Tr("editor.saves.no_saves_yet", "No saves yet"), th.TextDisabled, core.AlignCenter)
	}
}

func (p *Panel) button(id core.ID, r mathx.Rect, label string, enabled bool) bool {
	ui := p.f.UI
	th := &ui.Theme
	if !enabled {
		ui.Box(r, th.Radius.Button, th.Widget, th.Border)
		ui.TextIn(r, label, th.TextDisabled, core.AlignCenter)
		return false
	}
	return widgets.ButtonRect(ui, id, r, label)
}

func (p *Panel) toolbar(st *save.Store) {
	ui := p.f.UI
	th := &ui.Theme
	bar := ui.Cursor().Next(0)
	slot := strings.TrimSpace(p.slot)
	over := p.exists(slot)
	live := p.service() != nil
	saveLabel := i18n.Tr("editor.saves.save", "Save")
	if over {
		saveLabel = i18n.Tr("editor.saves.overwrite", "Overwrite")
	}
	sw := ui.TextSize(i18n.Tr("editor.saves.overwrite", "Overwrite")).X + 14
	rw := ui.TextSize(i18n.Tr("editor.saves.refresh", "Refresh")).X + 14
	fw := max(80, bar.W-sw-rw-8)
	field := mathx.Rect{X: bar.X, Y: bar.Y, W: fw, H: bar.H}
	changed, committed := widgets.TextFieldRect(ui, ui.ID("slot"), field, &p.slot, false)
	if changed {
		p.typed = strings.TrimSpace(p.slot) != ""
	}
	x := field.X + fw + 4
	if p.button(ui.ID("write"), mathx.Rect{X: x, Y: bar.Y, W: sw, H: bar.H}, saveLabel, live && slot != "") || (live && committed && slot != "") {
		p.saveNow()
	}
	if p.button(ui.ID("refresh"), mathx.Rect{X: x + sw + 4, Y: bar.Y, W: rw, H: bar.H}, i18n.Tr("editor.saves.refresh", "Refresh"), true) {
		p.Refresh()
	}
	line := ui.Cursor().Next(0)
	note, col := st.Dir(), th.TextDim
	switch {
	case !live:
		note += " · saving works during Play"
	case over:
		note, col = i18n.Tr("editor.saves.slot_already_exists_and_will", "{slot} already exists and will be replaced", i18n.Str("slot", slot)), th.Warn
	}
	ui.PushClip(line)
	ui.Text(note, mathx.Vec2{X: line.X, Y: ui.TextY(line)}, col)
	ui.PopClip()
}

func (p *Panel) saveNow() {
	svc := p.service()
	if svc == nil {
		p.notify(context.LevelInfo, i18n.Note("editor.saves.saving_works_during_play", "Saving works during Play"))
		return
	}
	slot := strings.TrimSpace(p.slot)
	if err := svc.Save(slot); err != nil {
		p.notify(context.LevelError, err.Error())
		return
	}
	p.sel = slot
	if pre, _, ok := splitSlot(slot); ok {
		p.prefix = pre
	}
	p.typed = false
	p.slot = NextSlot(p.prefix, append(p.taken(), slot))
	p.Refresh()
	p.notify(context.LevelInfo, i18n.Note("editor.saves.saving_to_slot", "Saving to {slot}", i18n.Str("slot", slot)))
}

var columns = [...]i18n.Text{i18n.T("editor.saves.col.slot", "Slot"), i18n.T("editor.saves.col.scene", "Scene"), i18n.T("editor.saves.col.saved", "Saved"), i18n.T("editor.saves.col.playtime", "Playtime"), i18n.T("editor.saves.col.objects", "Objects"), i18n.T("editor.saves.col.size", "Size")}

var colAlign = [len(columns)]core.Align{core.AlignLeft, core.AlignLeft, core.AlignLeft, core.AlignRight, core.AlignRight, core.AlignRight}

var (
	wide    = []int{0, 1, 2, 3, 4, 5}
	compact = []int{0, 2, 3, 5}
	weights = [len(columns)]float32{1.1, 1.6, 1.5, 0.8, 0.7, 0.7}
)

type cell struct {
	col int
	r   mathx.Rect
}

func (p *Panel) cells(r mathx.Rect, out []cell) []cell {
	show := wide
	if r.W < 720 {
		show = compact
	}
	total := float32(0)
	for _, c := range show {
		total += weights[c]
	}
	x := r.X
	for _, c := range show {
		cw := r.W * weights[c] / total
		out = append(out, cell{col: c, r: mathx.Rect{X: x + 4, Y: r.Y, W: cw - 8, H: r.H}})
		x += cw
	}
	return out
}

func (p *Panel) header(r mathx.Rect) {
	ui := p.f.UI
	ui.Rect(r, ui.Theme.CardHeader)
	var buf [len(columns)]cell
	for _, c := range p.cells(r, buf[:0]) {
		ui.PushClip(c.r)
		ui.TextIn(c.r, columns[c.col].String(), ui.Theme.Text, colAlign[c.col])
		ui.PopClip()
	}
}

func Playtime(sec float64) string {
	d := time.Duration(sec * float64(time.Second)).Round(time.Second)
	h := int(d / time.Hour)
	m := int(d/time.Minute) % 60
	s := int(d/time.Second) % 60
	pad := func(n int) string {
		if n < 10 {
			return "0" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	if h > 0 {
		return strconv.Itoa(h) + ":" + pad(m) + ":" + pad(s)
	}
	return strconv.Itoa(m) + ":" + pad(s)
}

func size(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " B"
	}
	return strconv.FormatFloat(float64(n)/1024, 'f', 1, 64) + " KB"
}

func (p *Panel) scene(id ids.AssetID) string {
	if id.IsZero() {
		return "—"
	}
	if p.d.Label != nil {
		return p.d.Label(id)
	}
	return id.Short()
}

func (p *Panel) row(info *save.SlotInfo, rr mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	id := core.IDOf("saves.row/" + info.Slot)
	if ui.Pressed(id, rr) {
		p.sel = info.Slot
	}
	if ui.DoubleClicked(id) {
		p.load(info.Slot)
	}
	switch {
	case p.sel == info.Slot:
		ui.Rect(rr, th.Selection)
	case ui.Hot() == id:
		ui.Rect(rr, th.WidgetHot)
	}
	col := th.Text
	vals := [len(columns)]string{info.Slot, p.scene(info.Meta.Scene), info.Meta.Date.Local().Format("2006-01-02 15:04"),
		Playtime(info.Meta.Playtime), strconv.Itoa(info.Meta.Objects), size(info.Size)}
	if info.Err != nil {
		col = th.Error
		vals = [len(columns)]string{info.Slot, "unreadable", info.Mod.Local().Format("2006-01-02 15:04"), "", "", size(info.Size)}
	}
	var buf [len(columns)]cell
	for _, c := range p.cells(rr, buf[:0]) {
		ui.PushClip(c.r)
		ui.TextIn(c.r, vals[c.col], col, colAlign[c.col])
		ui.PopClip()
	}
}

func (p *Panel) selected() *save.SlotInfo {
	for i := range p.list {
		if p.list[i].Slot == p.sel {
			return &p.list[i]
		}
	}
	return nil
}

func (p *Panel) details(st *save.Store, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	if r.W <= 0 || r.H <= 0 {
		return
	}
	info := p.selected()
	if info == nil {
		ui.TextIn(mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}, i18n.Tr("editor.saves.select_a_save", "Select a save to see what it holds"), th.TextDim, core.AlignLeft)
		return
	}
	if p.docSlot != info.Slot || !p.docMod.Equal(info.Mod) {
		p.doc, p.docErr = st.Load(info.Slot)
		p.docSlot, p.docMod = info.Slot, info.Mod
	}
	ui.PushClip(r)
	defer ui.PopClip()
	row := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight}
	labels := [...]string{i18n.Tr("editor.saves.load", "Load"), i18n.Tr("editor.saves.overwrite", "Overwrite"), i18n.Tr("editor.saves.delete", "Delete")}
	live := p.service() != nil
	base := ui.ID("detail")
	x := row.X
	for i, l := range labels {
		w := ui.TextSize(l).X + 14
		if p.button(core.IDSeq(base, i+1), mathx.Rect{X: x, Y: row.Y, W: w, H: row.H}, l, live || i == 2) {
			switch i {
			case 0:
				p.load(info.Slot)
			case 1:
				p.slot, p.typed = info.Slot, true
				p.saveNow()
			case 2:
				p.remove(st, info.Slot)
			}
		}
		x += w + 4
	}
	y := row.Y + row.H + th.Spacing
	if p.docErr != nil {
		ui.TextIn(mathx.Rect{X: r.X, Y: y, W: r.W, H: th.RowHeight}, p.docErr.Error(), th.Error, core.AlignLeft)
		return
	}
	d := p.doc
	m := &d.Meta
	kv := [][2]string{
		{i18n.Tr("editor.saves.col.slot", "Slot"), info.Slot},
		{i18n.Tr("editor.saves.col.scene", "Scene"), p.scene(m.Scene)},
		{i18n.Tr("editor.saves.col.saved", "Saved"), m.Date.Local().Format(time.DateTime)},
		{i18n.Tr("editor.saves.col.playtime", "Playtime"), Playtime(m.Playtime)},
		{i18n.Tr("editor.saves.col.game", "Game"), orDash(m.Game)},
		{i18n.Tr("editor.saves.col.engine", "Engine"), orDash(m.Engine)},
		{i18n.Tr("editor.saves.col.objects", "Objects"), i18n.Tr("editor.saves.objects_value", "{scene} scene, {spawned} spawned, {destroyed} destroyed", i18n.Int("scene", int64(len(d.Nodes))), i18n.Int("spawned", int64(len(d.Spawned))), i18n.Int("destroyed", int64(len(d.Destroyed))))},
		{i18n.Tr("editor.saves.col.next_id", "Next id"), strconv.FormatUint(m.Counter+1, 10)},
	}
	if len(d.Global) > 0 {
		keys := make([]string, 0, len(d.Global))
		for _, f := range d.Global {
			keys = append(keys, f.Name)
		}
		kv = append(kv, [2]string{i18n.Tr("editor.saves.col.global", "Global"), strings.Join(keys, ", ")})
	}
	for _, sec := range d.Sections {
		kv = append(kv, [2]string{i18n.Tr("editor.saves.col.section", "Section"), sec.ID + " v" + strconv.Itoa(sec.Version)})
	}
	lw := widgets.LabelWidth(r.W)
	body := mathx.Rect{X: r.X, Y: y, W: r.W, H: max(0, r.Y+r.H-y)}
	widgets.VirtualList(ui, ui.ID("details"), body, len(kv), th.RowHeight+th.Spacing, &p.dscroll, func(i int, rr mathx.Rect) {
		lr := mathx.Rect{X: rr.X, Y: rr.Y, W: lw - 4, H: rr.H}
		vr := mathx.Rect{X: rr.X + lw, Y: rr.Y, W: rr.W - lw, H: rr.H}
		ui.Text(kv[i][0], mathx.Vec2{X: lr.X, Y: ui.TextY(lr)}, th.TextDim)
		ui.Text(kv[i][1], mathx.Vec2{X: vr.X, Y: ui.TextY(vr)}, th.Text)
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (p *Panel) load(slot string) {
	svc := p.service()
	if svc == nil {
		p.notify(context.LevelInfo, i18n.Note("editor.saves.start_play_to_load_a", "Start Play to load a save"))
		return
	}
	if err := svc.Load(slot); err != nil {
		p.notify(context.LevelError, err.Error())
	}
}

func (p *Panel) remove(st *save.Store, slot string) {
	if p.d.Notices == nil {
		return
	}
	p.d.Notices.Ask(context.Question{
		Title:   i18n.Tr("editor.saves.delete_save", "Delete save"),
		Message: i18n.Tr("editor.saves.delete_the_save_in_slot", "Delete the save in slot {slot}?", i18n.Str("slot", slot)),
		Buttons: []string{i18n.Tr("editor.saves.delete", "Delete"), i18n.Tr("editor.saves.cancel", "Cancel")},
		Then: func(choice int) {
			if choice != 0 {
				return
			}
			if err := st.Delete(slot); err != nil {
				p.notify(context.LevelError, err.Error())
			}
			p.sel = ""
			p.Refresh()
		},
	})
}
