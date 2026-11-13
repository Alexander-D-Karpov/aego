package scripteditor

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/script/workspace"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	FindID    = "find"
	findDelay = 0.25
	findLimit = 2000
)

type findRow struct {
	file  bool
	path  string
	count int
	match int
}

type FindPanel struct {
	host      *Host
	query     string
	repl      string
	opts      workspace.GrepOptions
	title     string
	usages    bool
	matches   []workspace.Match
	rows      []findRow
	files     int
	collapsed map[string]bool
	truncated bool
	err       string
	dirty     bool
	editedAt  float64
	gen       uint64
	running   bool
	focus     bool
	scroll    float32
	sel       int
}

func newFindPanel(h *Host) *FindPanel {
	return &FindPanel{host: h, collapsed: map[string]bool{}, sel: -1}
}

func (f *FindPanel) ID() string {
	return FindID
}

func (f *FindPanel) Title() string {
	if f.usages {
		return i18n.Tr("editor.scripteditor.usages", "Usages")
	}
	return i18n.Tr("editor.scripteditor.find", "Find")
}

func (f *FindPanel) start(q string) {
	f.usages, f.title = false, ""
	if q != "" {
		f.query = q
	}
	f.focus = true
	f.changed(-1)
}

func (f *FindPanel) changed(now float64) {
	f.usages = false
	f.dirty = true
	f.editedAt = now
}

// A replaced source invalidates both displayed locations and a search whose
// owner-thread callback may still be queued. Ordinary searches are restarted;
// usages must be requested again against a fresh analysis snapshot.
func (f *FindPanel) sourceReplaced() {
	f.gen++
	f.running, f.truncated = false, false
	f.matches, f.rows = nil, nil
	f.files, f.sel, f.scroll = 0, -1, 0
	f.err, f.title = "", ""
	f.changed(-1)
}

func (f *FindPanel) showUsages(title string, ms []workspace.Match) {
	f.gen++
	f.usages, f.title = true, title
	f.running, f.dirty, f.truncated, f.err = false, false, false, ""
	f.matches = ms
	f.scroll, f.sel = 0, -1
	f.build()
}

func (f *FindPanel) tick(now float64) {
	if !f.dirty || f.usages || (f.editedAt >= 0 && now-f.editedAt < findDelay) {
		return
	}
	f.dirty = false
	f.gen++
	gen := f.gen
	if f.query == "" {
		f.matches, f.truncated, f.err, f.running = nil, false, "", false
		f.build()
		return
	}
	q, o, ws := f.query, f.opts, f.host.ws
	f.running = true
	d := f.host.d
	d.Submit(func() {
		ms, trunc, err := ws.Grep(q, o, findLimit)
		d.OnMain(func() {
			if gen != f.gen {
				return
			}
			f.running = false
			f.matches, f.truncated, f.err = ms, trunc, ""
			if err != nil {
				f.err = err.Error()
			}
			f.scroll, f.sel = 0, -1
			f.build()
		})
	})
}

func (f *FindPanel) build() {
	sort.SliceStable(f.matches, func(i, j int) bool {
		a, b := f.matches[i], f.matches[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	f.rows = f.rows[:0]
	f.files = 0
	for i := 0; i < len(f.matches); {
		j := i
		for j < len(f.matches) && f.matches[j].Path == f.matches[i].Path {
			j++
		}
		p := f.matches[i].Path
		f.rows = append(f.rows, findRow{file: true, path: p, count: j - i})
		f.files++
		if !f.collapsed[p] {
			for k := i; k < j; k++ {
				f.rows = append(f.rows, findRow{path: p, match: k})
			}
		}
		i = j
	}
	if f.sel >= len(f.rows) {
		f.sel = -1
	}
}

func (f *FindPanel) status() string {
	switch {
	case f.err != "":
		return f.err
	case f.running:
		return i18n.Tr("editor.scripteditor.searching", "Searching…")
	case f.usages:
		return i18n.Tr("editor.scripteditor.title_matches_usage_s_in", "{title}: {matches} usage(s) in {files} file(s)", i18n.Str("title", f.title), i18n.Str("matches", strconv.Itoa(len(f.matches))), i18n.Str("files", strconv.Itoa(f.files)))
	case f.query == "":
		return i18n.Tr("editor.scripteditor.type_to_search_in_all", "Type to search in all scripts")
	}
	s := strconv.Itoa(len(f.matches)) + " match(es) in " + strconv.Itoa(f.files) + " file(s)"
	if f.truncated {
		s += ", showing the first " + strconv.Itoa(findLimit)
	}
	return s
}

func (f *FindPanel) Draw(fr *services.Frame, r mathx.Rect) {
	ui := fr.UI
	th := &ui.Theme
	bar := ui.Cursor().Next(0)
	qid := ui.ID("query")
	if f.focus {
		ui.SetFocus(qid)
		ui.EditBuffer(qid, f.query).SelectAll()
		f.focus = false
	}
	enter := ui.Focus() == qid && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter))
	tw := float32(34)
	fw := max(80, bar.W-3*(tw+4))
	if changed, _ := widgets.TextFieldRect(ui, qid, mathx.Rect{X: bar.X, Y: bar.Y, W: fw, H: bar.H}, &f.query, true); changed {
		f.changed(ui.Time)
	}
	if enter {
		f.changed(-1)
		ui.SetFocus(qid)
	}
	toggles := [...]struct {
		label string
		tip   string
		on    *bool
	}{
		{"Aa", i18n.Tr("editor.scripteditor.match_case", "Match case"), &f.opts.Case},
		{"W", i18n.Tr("editor.scripteditor.whole_word", "Whole word"), &f.opts.Word},
		{".*", i18n.Tr("editor.scripteditor.regular_expression", "Regular expression"), &f.opts.Regexp},
	}
	x := bar.X + fw + 4
	for i, t := range toggles {
		id := core.IDSeq(qid, 10+i)
		if widgets.ToggleButton(ui, id, mathx.Rect{X: x, Y: bar.Y, W: tw, H: bar.H}, t.label, t.on) {
			f.changed(-1)
		}
		widgets.Tooltip(ui, id, t.tip)
		x += tw + 4
	}
	rrow := ui.Cursor().Next(0)
	bw := float32(100)
	widgets.TextFieldRect(ui, ui.ID("repl"), mathx.Rect{X: rrow.X, Y: rrow.Y, W: max(60, rrow.W-bw-4), H: rrow.H}, &f.repl, true)
	if widgets.ButtonRect(ui, ui.ID("replall"), mathx.Rect{X: rrow.X + rrow.W - bw, Y: rrow.Y, W: bw, H: rrow.H}, i18n.Tr("editor.scripteditor.replace_all", "Replace All")) {
		f.replaceAll()
	}
	col := th.TextDim
	if f.err != "" {
		col = th.Error
	}
	sr := ui.Cursor().Next(0)
	ui.PushClip(sr)
	ui.Text(f.status(), mathx.Vec2{X: sr.X, Y: ui.TextY(sr)}, col)
	ui.PopClip()
	top := sr.Y + sr.H + th.Spacing
	list := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	widgets.VirtualList(ui, ui.ID("list"), list, len(f.rows), th.RowHeight, &f.scroll, func(i int, rr mathx.Rect) {
		f.drawRow(ui, i, rr)
	})
}

func (f *FindPanel) drawRow(ui *core.Context, i int, rr mathx.Rect) {
	th := &ui.Theme
	row := f.rows[i]
	if row.file {
		id := core.IDOf("find.file/" + row.path)
		if ui.Pressed(id, rr) {
			f.collapsed[row.path] = !f.collapsed[row.path]
			f.build()
		}
		if ui.Hot() == id {
			ui.Rect(rr, th.WidgetHot)
		}
		widgets.Chevron(ui, mathx.Rect{X: rr.X, Y: rr.Y, W: th.Indent, H: rr.H}, !f.collapsed[row.path], th.TextDim)
		x := rr.X + th.Indent + 4
		name := path.Base(row.path)
		ui.Text(name, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.Text)
		x += ui.TextSize(name).X + 10
		ui.Text(path.Dir(row.path)+"  ("+strconv.Itoa(row.count)+")", mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
		return
	}
	m := f.matches[row.match]
	id := core.IDSeq(ui.ID("match"), i)
	if ui.Pressed(id, rr) {
		f.sel = i
		f.host.Goto(context.At(m.Path, m.Line, m.Col))
	}
	switch {
	case f.sel == i:
		ui.Rect(rr, th.SelectionDim)
	case ui.Hot() == id:
		ui.Rect(rr, th.WidgetHot)
	}
	x := rr.X + th.Indent*2
	pos := strconv.Itoa(m.Line) + ":" + strconv.Itoa(m.Col)
	ui.Text(pos, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.TextDim)
	x += max(ui.TextSize(pos).X+10, 56)
	text := strings.TrimLeft(m.Text, " \t")
	off := max(0, m.Offset-(len(m.Text)-len(text)))
	end := min(len(text), off+m.Len)
	off = min(off, end)
	ui.PushClip(mathx.Rect{X: x, Y: rr.Y, W: max(0, rr.X+rr.W-x), H: rr.H})
	hx := x + ui.TextSize(text[:off]).X
	ui.Rect(mathx.Rect{X: hx, Y: rr.Y + 2, W: ui.TextSize(text[off:end]).X, H: rr.H - 4}, th.Code.Occurrence)
	ui.Text(text, mathx.Vec2{X: x, Y: ui.TextY(rr)}, th.Text)
	ui.PopClip()
}

func (f *FindPanel) replaceAll() {
	if f.usages || f.query == "" || len(f.matches) == 0 {
		return
	}
	re, err := workspace.Pattern(f.query, f.opts)
	if err != nil {
		f.host.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	var paths []string
	seen := map[string]bool{}
	for _, m := range f.matches {
		if !seen[m.Path] {
			seen[m.Path] = true
			paths = append(paths, m.Path)
		}
	}
	total, files := 0, 0
	for _, p := range paths {
		d, err := f.host.docs().OpenScript(p)
		if err != nil {
			f.host.d.Notices.Notify(context.LevelError, err.Error())
			continue
		}
		c := d.Code
		src := c.Bytes()
		var rs [][2]int
		var texts [][]byte
		for _, m := range re.FindAllSubmatchIndex(src, -1) {
			if m[1] == m[0] {
				continue
			}
			t := []byte(f.repl)
			if f.opts.Regexp {
				t = re.Expand(nil, []byte(f.repl), src, m)
			}
			rs = append(rs, [2]int{m[0], m[1]})
			texts = append(texts, t)
		}
		if n := c.ReplaceRanges(rs, texts); n > 0 {
			total += n
			files++
			f.host.ws.SetOverlay(d.Path, c.Bytes())
		}
	}
	f.host.lang.Invalidate()
	f.host.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.scripteditor.replaced_total_occurrence_s_in", "Replaced {total} occurrence(s) in {files} file(s), Save All writes them", i18n.Str("total", strconv.Itoa(total)), i18n.Str("files", strconv.Itoa(files))))
	f.changed(-1)
}
