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
	slot    string
}

func New(d Deps) *Panel {
	return &Panel{d: d, at: -10, slot: "slot1"}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	return "Saves"
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

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	th := &ui.Theme
	var st *save.Store
	if p.d.Store != nil {
		st = p.d.Store()
	}
	if st == nil {
		widgets.LabelDim(ui, "Saves are not available: this run has no player profile")
		return
	}
	if f.Now-p.at > 1 {
		p.list, p.at = st.List(), f.Now
	}
	p.toolbar(st)
	widgets.LabelDim(ui, st.Dir())
	cur := ui.Cursor()
	hdr := cur.Next(0)
	p.header(hdr)
	listH := max(th.RowHeight*3, (r.Y+r.H-cur.Y)*0.5)
	body := mathx.Rect{X: hdr.X, Y: cur.Y, W: hdr.W, H: listH}
	widgets.VirtualList(ui, ui.ID("slots"), body, len(p.list), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.row(&p.list[i], rr)
	})
	if len(p.list) == 0 {
		ui.TextIn(body, "No saves yet", th.TextDisabled, core.AlignCenter)
	}
	cur.Y = body.Y + body.H + th.Spacing
	p.details(st)
}

func (p *Panel) toolbar(st *save.Store) {
	ui := p.f.UI
	bar := ui.Cursor().Next(0)
	labels := [...]string{"Save", "Refresh"}
	var w [len(labels)]float32
	total := float32(0)
	for i, l := range labels {
		w[i] = ui.TextSize(l).X + 14
		total += w[i] + 4
	}
	fw := max(80, bar.W-total)
	widgets.TextFieldRect(ui, ui.ID("slot"), mathx.Rect{X: bar.X, Y: bar.Y, W: fw, H: bar.H}, &p.slot, false)
	x := bar.X + fw + 4
	base := ui.ID("tool")
	for i, l := range labels {
		if widgets.ButtonRect(ui, core.IDSeq(base, i+1), mathx.Rect{X: x, Y: bar.Y, W: w[i], H: bar.H}, l) {
			switch i {
			case 0:
				p.saveNow()
			case 1:
				p.Refresh()
			}
		}
		x += w[i] + 4
	}
}

func (p *Panel) saveNow() {
	svc := p.service()
	if svc == nil {
		p.notify(context.LevelInfo, "Saving works during Play")
		return
	}
	slot := strings.TrimSpace(p.slot)
	if err := svc.Save(slot); err != nil {
		p.notify(context.LevelError, err.Error())
		return
	}
	p.sel = slot
	p.Refresh()
	p.notify(context.LevelInfo, "Saving to "+slot)
}

var columns = [...]string{"Slot", "Scene", "Saved", "Playtime", "Objects", "Size"}

func (p *Panel) cells(r mathx.Rect) [len(columns)]mathx.Rect {
	var out [len(columns)]mathx.Rect
	weights := [len(columns)]float32{1.2, 1.6, 1.4, 0.9, 0.7, 0.7}
	total := float32(0)
	for _, w := range weights {
		total += w
	}
	x := r.X
	for i, w := range weights {
		cw := r.W * w / total
		out[i] = mathx.Rect{X: x + 4, Y: r.Y, W: cw - 8, H: r.H}
		x += cw
	}
	return out
}

func (p *Panel) header(r mathx.Rect) {
	ui := p.f.UI
	ui.Rect(r, ui.Theme.CardHeader)
	for i, c := range p.cells(r) {
		ui.PushClip(c)
		ui.Text(columns[i], mathx.Vec2{X: c.X, Y: ui.TextY(c)}, ui.Theme.Text)
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
	for i, c := range p.cells(rr) {
		ui.PushClip(c)
		ui.Text(vals[i], mathx.Vec2{X: c.X, Y: ui.TextY(c)}, col)
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

func (p *Panel) details(st *save.Store) {
	ui := p.f.UI
	info := p.selected()
	if info == nil {
		widgets.LabelDim(ui, "Select a save to see what it holds")
		return
	}
	if p.docSlot != info.Slot || !p.docMod.Equal(info.Mod) {
		p.doc, p.docErr = st.Load(info.Slot)
		p.docSlot, p.docMod = info.Slot, info.Mod
	}
	row := ui.Cursor().Next(0)
	w := (row.W - 4) / 2
	if widgets.ButtonRect(ui, ui.ID("load"), mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, "Load") {
		p.load(info.Slot)
	}
	if widgets.ButtonRect(ui, ui.ID("delete"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w, H: row.H}, "Delete") {
		p.remove(st, info.Slot)
	}
	if p.docErr != nil {
		widgets.LabelWrapped(ui, p.docErr.Error(), true)
		return
	}
	d := p.doc
	m := &d.Meta
	widgets.Label(ui, "Scene: "+p.scene(m.Scene))
	widgets.Label(ui, "Game: "+orDash(m.Game)+"   Engine: "+orDash(m.Engine))
	widgets.Label(ui, "Saved: "+m.Date.Local().Format(time.DateTime)+"   Playtime: "+Playtime(m.Playtime))
	widgets.Label(ui, "Scene nodes: "+strconv.Itoa(len(d.Nodes))+"   Spawned: "+strconv.Itoa(len(d.Spawned))+
		"   Destroyed: "+strconv.Itoa(len(d.Destroyed))+"   Next id: "+strconv.FormatUint(m.Counter+1, 10))
	if len(d.Global) > 0 {
		var keys []string
		for _, f := range d.Global {
			keys = append(keys, f.Name)
		}
		widgets.Label(ui, "Global: "+strings.Join(keys, ", "))
	}
	for _, sec := range d.Sections {
		widgets.LabelDim(ui, "Section "+sec.ID+" v"+strconv.Itoa(sec.Version))
	}
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
		p.notify(context.LevelInfo, "Start Play to load a save")
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
		Title:   "Delete save",
		Message: "Delete the save in slot " + slot + "?",
		Buttons: []string{"Delete", "Cancel"},
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
