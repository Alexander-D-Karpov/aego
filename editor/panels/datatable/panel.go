package datatable

import (
	"cmp"
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/dataedit"
	"aego/editor/core/services"
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

const ID = "datatable"

const (
	gutter  = float32(32)
	idWidth = float32(140)
	minCol  = float32(80)
)

const (
	actAdd = iota
	actDuplicate
	actDelete
	actUndo
	actRedo
	actExport
	actImport
	actSave
)

var actions = [...]i18n.Text{i18n.T("editor.datatable.action.add_row", "+ Row"), i18n.T("editor.datatable.action.duplicate", "Duplicate"), i18n.T("editor.datatable.action.delete", "Delete"), i18n.T("editor.datatable.action.undo", "Undo"), i18n.T("editor.datatable.action.redo", "Redo"), i18n.T("editor.datatable.action.export_csv", "Export CSV"), i18n.T("editor.datatable.action.import_csv", "Import CSV"), i18n.T("editor.datatable.action.save", "Save")}

type Deps struct {
	Data    *dataedit.Service
	Notices services.Notices
}

type Panel struct {
	d         Deps
	f         *services.Frame
	table     *dataedit.Table
	cols      []*schema.FieldDescriptor
	view      []int
	w         []float32
	sel       map[string]bool
	anchor    string
	filter    string
	sortCol   int
	desc      bool
	scroll    float32
	editRow   string
	editCol   int
	editStart bool
	editText  string
	step      bool
}

func New(d Deps) *Panel {
	return &Panel{d: d, sel: map[string]bool{}, sortCol: -1}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	if p.table == nil {
		return i18n.Tr("editor.datatable.title", "Table")
	}
	t := i18n.Tr("editor.datatable.title", "Table") + " · " + strings.TrimSuffix(path.Base(p.table.Path), path.Ext(p.table.Path))
	if p.table.Dirty() {
		t += " •"
	}
	return t
}

func (p *Panel) Open(id ids.AssetID) error {
	if p.d.Data == nil {
		return nil
	}
	t, err := p.d.Data.Table(id)
	if err != nil {
		return err
	}
	p.Show(t)
	return nil
}

func (p *Panel) Show(t *dataedit.Table) {
	if p.table == nil || t == nil || p.table.ID != t.ID {
		clear(p.sel)
		p.anchor, p.filter, p.sortCol, p.desc, p.scroll, p.editRow = "", "", -1, false, 0, ""
	}
	p.table = t
	p.cols = p.cols[:0]
	if t == nil {
		return
	}
	for i := range t.Type.Fields {
		fd := &t.Type.Fields[i]
		if fd.Flags.Has(schema.FlagEditor) && !fd.Hints.Hidden {
			p.cols = append(p.cols, fd)
		}
	}
}

func (p *Panel) SetSort(col int, desc bool) {
	p.sortCol, p.desc = col, desc
}

func (p *Panel) Select(rows ...string) {
	clear(p.sel)
	for _, r := range rows {
		p.sel[r] = true
	}
	if len(rows) > 0 {
		p.anchor = rows[0]
	}
}

func (p *Panel) notify(err error) {
	if err != nil && p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func (p *Panel) info(s string) {
	if p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelInfo, s)
	}
}

func (p *Panel) readOnly() bool {
	return p.d.Data != nil && p.d.Data.InPlay()
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	th := &ui.Theme
	if p.d.Data != nil && p.table != nil {
		if t, err := p.d.Data.Table(p.table.ID); err == nil && t != p.table {
			p.Show(t)
		}
	}
	if p.table == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.datatable.open_a_aetable_asset_to", "Open a .aetable asset to edit it here"))
		return
	}
	ro := p.readOnly()
	p.toolbar(ro)
	if ro {
		widgets.LabelDim(ui, i18n.Tr("editor.datatable.tables_are_read_only_during", "Tables are read-only during Play"))
	}
	for i, s := range p.table.Issues {
		if i == 3 {
			widgets.LabelDim(ui, i18n.Tr("editor.datatable.v_more_issue_s", "{v} more issue(s)", i18n.Str("v", strconv.Itoa(len(p.table.Issues)-3))))
			break
		}
		p.issue(s)
	}
	p.build()
	cur := ui.Cursor()
	hdr := cur.Next(0)
	w := p.widths(hdr.W)
	p.header(hdr, w)
	body := mathx.Rect{X: hdr.X, Y: cur.Y, W: hdr.W, H: max(0, r.Y+r.H-cur.Y)}
	widgets.VirtualList(ui, ui.ID("rows"), body, len(p.view), th.RowHeight, &p.scroll, func(i int, rr mathx.Rect) {
		p.row(p.view[i], rr, w, ro)
	})
	if len(p.view) == 0 {
		msg := i18n.Tr("editor.datatable.the_table_is_empty_add", "The table is empty, add a row")
		if p.filter != "" {
			msg = i18n.Tr("editor.datatable.no_rows_match_the_filter", "No rows match the filter")
		}
		ui.TextIn(body, msg, th.TextDisabled, core.AlignCenter)
	}
}

func (p *Panel) issue(s string) {
	ui := p.f.UI
	th := &ui.Theme
	r := ui.Cursor().Next(0)
	widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "status.warning", th.Warn)
	ui.PushClip(r)
	ui.Text(s, mathx.Vec2{X: r.X + 20, Y: ui.TextY(r)}, th.TextDim)
	ui.PopClip()
}

func (p *Panel) toolbar(ro bool) {
	ui := p.f.UI
	bar := ui.Cursor().Next(0)
	var widths [len(actions)]float32
	total := float32(0)
	for i, a := range actions {
		widths[i] = ui.TextSize(a.String()).X + 14
		total += widths[i] + 4
	}
	sw := max(100, bar.W-total)
	widgets.SearchField(ui, ui.ID("filter"), mathx.Rect{X: bar.X, Y: bar.Y, W: sw, H: bar.H}, &p.filter)
	x := bar.X + sw + 4
	base := ui.ID("tool")
	for i, a := range actions {
		if widgets.ButtonRect(ui, core.IDSeq(base, i+1), mathx.Rect{X: x, Y: bar.Y, W: widths[i], H: bar.H}, a.String()) {
			p.act(i, ro)
		}
		x += widths[i] + 4
	}
}

func (p *Panel) selected() []int {
	var out []int
	for i := range p.table.Rows {
		if p.sel[p.table.Rows[i].ID] {
			out = append(out, i)
		}
	}
	return out
}

func (p *Panel) act(a int, ro bool) {
	t := p.table
	if ro && a != actExport {
		p.info("Tables are read-only during Play")
		return
	}
	switch a {
	case actAdd:
		i, err := t.AddRow()
		if err != nil {
			p.notify(err)
			return
		}
		p.filter = ""
		p.Select(t.Rows[i].ID)
	case actDuplicate:
		made, err := t.Duplicate(p.selected())
		if err != nil {
			p.notify(err)
			return
		}
		ids := make([]string, len(made))
		for k, i := range made {
			ids[k] = t.Rows[i].ID
		}
		if len(ids) > 0 {
			p.Select(ids...)
		}
	case actDelete:
		p.notify(t.Delete(p.selected()))
		clear(p.sel)
	case actUndo:
		p.notify(t.Undo())
	case actRedo:
		p.notify(t.Redo())
	case actExport:
		if p.d.Data == nil {
			return
		}
		rel, err := p.d.Data.ExportCSV(t)
		if err != nil {
			p.notify(err)
			return
		}
		p.info("Exported " + rel)
	case actImport:
		if p.d.Data == nil {
			return
		}
		rel, err := p.d.Data.ImportCSV(t)
		if err != nil {
			p.notify(err)
			return
		}
		p.info("Imported " + rel)
	case actSave:
		if p.d.Data != nil {
			p.notify(p.d.Data.SaveTable(t))
		}
	}
}

func (p *Panel) widths(total float32) []float32 {
	n := len(p.cols)
	p.w = p.w[:0]
	p.w = append(p.w, idWidth)
	if n > 0 {
		cw := max(minCol, (total-gutter-idWidth)/float32(n))
		for i := 0; i < n; i++ {
			p.w = append(p.w, cw)
		}
	}
	return p.w
}

func label(fd *schema.FieldDescriptor) string {
	if fd.Hints.DisplayName != "" {
		return fd.Hints.DisplayName
	}
	return inspect.Humanize(fd.Name)
}

func (p *Panel) header(r mathx.Rect, w []float32) {
	ui := p.f.UI
	ui.Rect(r, ui.Theme.CardHeader)
	ui.PushClip(r)
	x := r.X + gutter
	p.headCell(0, "ID", mathx.Rect{X: x, Y: r.Y, W: w[0], H: r.H})
	x += w[0]
	for i, fd := range p.cols {
		p.headCell(i+1, label(fd), mathx.Rect{X: x, Y: r.Y, W: w[i+1], H: r.H})
		x += w[i+1]
	}
	ui.PopClip()
}

func (p *Panel) headCell(col int, text string, r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	id := core.IDSeq(ui.ID("head"), col+1)
	if ui.Clicked(id, r) {
		switch {
		case p.sortCol != col:
			p.sortCol, p.desc = col, false
		case !p.desc:
			p.desc = true
		default:
			p.sortCol, p.desc = -1, false
		}
	}
	if ui.Hot() == id {
		ui.Rect(r, th.WidgetHot)
	}
	ui.PushClip(r)
	ui.Text(text, mathx.Vec2{X: r.X + 4, Y: ui.TextY(r)}, th.Text)
	if p.sortCol == col {
		mark := "ui.chevron_up"
		if p.desc {
			mark = "ui.chevron_down"
		}
		widgets.IconAt(ui, mathx.Rect{X: r.X + 6 + ui.TextSize(text).X, Y: float32(int(r.Y + (r.H-16)/2)), W: 16, H: 16}, mark, th.TextDim)
	}
	ui.PopClip()
}

func (p *Panel) build() {
	t := p.table
	p.view = p.view[:0]
	q := strings.ToLower(strings.TrimSpace(p.filter))
	for i := range t.Rows {
		if q == "" || p.matches(i, q) {
			p.view = append(p.view, i)
		}
	}
	if p.sortCol < 0 || p.sortCol > len(p.cols) {
		return
	}
	sort.SliceStable(p.view, func(x, y int) bool {
		c := p.compare(p.view[x], p.view[y])
		if p.desc {
			return c > 0
		}
		return c < 0
	})
}

func (p *Panel) matches(i int, q string) bool {
	if strings.Contains(strings.ToLower(p.table.Rows[i].ID), q) {
		return true
	}
	for _, fd := range p.cols {
		if strings.Contains(strings.ToLower(p.text(p.table.Value(i, fd), fd)), q) {
			return true
		}
	}
	return false
}

func (p *Panel) compare(a, b int) int {
	t := p.table
	if p.sortCol == 0 {
		return strings.Compare(t.Rows[a].ID, t.Rows[b].ID)
	}
	fd := p.cols[p.sortCol-1]
	return compareValues(t.Value(a, fd), t.Value(b, fd))
}

func compareValues(a, b schema.Value) int {
	switch a.Kind {
	case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64, schema.KindEnum:
		return cmp.Compare(a.AsFloat(), b.AsFloat())
	case schema.KindString:
		return strings.Compare(a.S, b.S)
	case schema.KindAssetRef:
		return strings.Compare(a.Asset.String(), b.Asset.String())
	case schema.KindList:
		return cmp.Compare(len(a.List), len(b.List))
	}
	return 0
}

func (p *Panel) text(v schema.Value, fd *schema.FieldDescriptor) string {
	switch v.Kind {
	case schema.KindBool:
		return strconv.FormatBool(v.I != 0)
	case schema.KindInt32, schema.KindInt64:
		return strconv.FormatInt(v.I, 10)
	case schema.KindFloat32, schema.KindFloat64:
		return widgets.FormatNumber(v.F, widgets.NumberOpts{})
	case schema.KindString:
		return v.S
	case schema.KindEnum:
		if n, ok := fd.EnumNameOf(v.I); ok {
			return n
		}
		return strconv.FormatInt(v.I, 10)
	case schema.KindAssetRef:
		if v.Asset.IsZero() {
			return i18n.Tr("editor.datatable.none", "None")
		}
		if p.d.Data != nil {
			return p.d.Data.AssetLabel(v.Asset)
		}
		return v.Asset.Short()
	case schema.KindColor:
		return widgets.HexColor(v.C)
	case schema.KindList:
		return "[" + strconv.Itoa(len(v.List)) + "]"
	}
	return "…"
}

func (p *Panel) viewPos(rid string) int {
	for k, i := range p.view {
		if p.table.Rows[i].ID == rid {
			return k
		}
	}
	return -1
}

func (p *Panel) press(rid string) {
	mods := p.f.UI.Mods()
	switch {
	case mods.Primary():
		if p.sel[rid] {
			delete(p.sel, rid)
		} else {
			p.sel[rid] = true
		}
		p.anchor = rid
	case mods.Has(input.ModShift) && p.anchor != "":
		a, b := p.viewPos(p.anchor), p.viewPos(rid)
		if a < 0 || b < 0 {
			p.Select(rid)
			return
		}
		clear(p.sel)
		for k := min(a, b); k <= max(a, b); k++ {
			p.sel[p.table.Rows[p.view[k]].ID] = true
		}
	default:
		p.Select(rid)
	}
}

func (p *Panel) targets(i int) []int {
	if rid := p.table.Rows[i].ID; p.sel[rid] && len(p.sel) > 1 {
		return p.selected()
	}
	return []int{i}
}

func (p *Panel) set(i int, fd *schema.FieldDescriptor, v schema.Value, step bool) {
	p.notify(p.table.Set(p.targets(i), fd, v, step))
}

func (p *Panel) row(i int, rr mathx.Rect, w []float32, ro bool) {
	ui := p.f.UI
	th := &ui.Theme
	rid := p.table.Rows[i].ID
	if p.sel[rid] {
		ui.Rect(rr, th.Selection)
	}
	ui.PushID(rid)
	defer ui.PopID()
	gut := mathx.Rect{X: rr.X, Y: rr.Y, W: gutter, H: rr.H}
	if ui.Pressed(ui.ID("gutter"), gut) {
		p.press(rid)
	}
	ui.TextIn(gut, strconv.Itoa(i+1), th.TextDisabled, core.AlignCenter)
	x := rr.X + gutter
	p.idCell(i, mathx.Rect{X: x, Y: rr.Y, W: w[0], H: rr.H}, ro)
	x += w[0]
	for c, fd := range p.cols {
		ui.PushIDInt(c + 1)
		p.cell(i, c+1, fd, mathx.Rect{X: x + 2, Y: rr.Y + 1, W: w[c+1] - 4, H: rr.H - 2}, ro)
		ui.PopID()
		x += w[c+1]
	}
}

func (p *Panel) beginEdit(rid string, col int, text string) {
	p.editRow, p.editCol, p.editText, p.editStart = rid, col, text, true
}

func (p *Panel) textEdit(r mathx.Rect, commit func(string) error) {
	ui := p.f.UI
	fid := ui.ID("edit")
	if p.editStart {
		p.editStart = false
		ui.SetFocus(fid)
		ui.EditBuffer(fid, p.editText).SelectAll()
	}
	if _, done := widgets.TextFieldRect(ui, fid, r, &p.editText, false); done {
		p.notify(commit(p.editText))
		p.editRow = ""
		return
	}
	if ui.Focus() != fid {
		p.editRow = ""
	}
}

func (p *Panel) idCell(i int, r mathx.Rect, ro bool) {
	ui := p.f.UI
	th := &ui.Theme
	rid := p.table.Rows[i].ID
	if p.editRow == rid && p.editCol == 0 {
		p.textEdit(r, func(s string) error {
			if err := p.table.Rename(i, s); err != nil {
				return err
			}
			next := p.table.Rows[i].ID
			if p.sel[rid] {
				delete(p.sel, rid)
				p.sel[next] = true
			}
			if p.anchor == rid {
				p.anchor = next
			}
			return nil
		})
		return
	}
	id := ui.ID("id")
	if ui.Pressed(id, r) {
		p.press(rid)
		if ui.DoubleClicked(id) && !ro {
			p.beginEdit(rid, 0, rid)
		}
	}
	ui.PushClip(r)
	ui.Text(rid, mathx.Vec2{X: r.X + 4, Y: ui.TextY(r)}, th.Text)
	ui.PopClip()
}

func inline(fd *schema.FieldDescriptor) bool {
	switch fd.Kind {
	case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64, schema.KindString:
		return true
	case schema.KindEnum:
		return len(fd.Enum) > 0
	}
	return false
}

func numOpts(fd *schema.FieldDescriptor, isInt bool) widgets.NumberOpts {
	o := widgets.NumberOpts{Int: isInt, Min: fd.Min, Max: fd.Max, HasMin: fd.HasMin, HasMax: fd.HasMax, Speed: fd.Hints.Step, Unit: fd.Hints.Unit}
	if o.Speed == 0 {
		switch {
		case fd.HasMin && fd.HasMax && fd.Max > fd.Min:
			o.Speed = (fd.Max - fd.Min) / 200
		case isInt:
			o.Speed = 0.1
		default:
			o.Speed = 0.05
		}
	}
	return o
}

func numValue(k schema.Kind, f float64) schema.Value {
	switch k {
	case schema.KindInt32:
		return schema.Int32(int32(f))
	case schema.KindInt64:
		return schema.Int64(int64(f))
	case schema.KindFloat32:
		return schema.Float32(float32(f))
	}
	return schema.Float64(f)
}

func (p *Panel) cell(i, col int, fd *schema.FieldDescriptor, r mathx.Rect, ro bool) {
	ui := p.f.UI
	th := &ui.Theme
	rid := p.table.Rows[i].ID
	v := p.table.Value(i, fd)
	if ro || fd.Flags.Has(schema.FlagReadOnly) || !inline(fd) {
		ui.PushClip(r)
		ui.Text(p.text(v, fd), mathx.Vec2{X: r.X + 2, Y: ui.TextY(r)}, th.TextDim)
		ui.PopClip()
		return
	}
	id := ui.ID("cell")
	switch fd.Kind {
	case schema.KindBool:
		box := mathx.Rect{X: r.X + 2, Y: r.Y + 2, W: r.H - 4, H: r.H - 4}
		if ui.Clicked(id, box) {
			p.set(i, fd, schema.Bool(v.I == 0), true)
		}
		if v.I != 0 {
			ui.Rect(box, th.Accent)
			widgets.CheckMark(ui, box, th.Background)
		} else {
			ui.Rect(box, ui.WidgetColor(id))
			ui.Outline(box, th.Border)
		}
	case schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64:
		isInt := fd.Kind == schema.KindInt32 || fd.Kind == schema.KindInt64
		f := v.AsFloat()
		e := widgets.DragNumber(ui, id, r, &f, numOpts(fd, isInt))
		if e.Begin {
			p.step = true
		}
		if e.Changed {
			p.set(i, fd, numValue(fd.Kind, f), p.step)
			p.step = false
		}
	case schema.KindString:
		if p.editRow == rid && p.editCol == col {
			p.textEdit(r, func(s string) error {
				return p.table.Set(p.targets(i), fd, schema.String(s), true)
			})
			return
		}
		if ui.Pressed(id, r) {
			p.press(rid)
			if ui.DoubleClicked(id) {
				p.beginEdit(rid, col, v.S)
			}
		}
		ui.PushClip(r)
		ui.Text(v.S, mathx.Vec2{X: r.X + 2, Y: ui.TextY(r)}, th.Text)
		ui.PopClip()
	case schema.KindEnum:
		names := make([]string, len(fd.Enum))
		cur := -1
		for k, ev := range fd.Enum {
			names[k] = ev.Name
			if ev.Value == v.I {
				cur = k
			}
		}
		if widgets.Combo(ui, id, r, &cur, names, false) && cur >= 0 {
			p.set(i, fd, schema.Enum(fd.Enum[cur].Value), true)
		}
	}
}
