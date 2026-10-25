package scripteditor

import (
	"path"
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/code"
	"aego/editor/core/code/highlight"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/script/diag"
	"aego/script/langsvc"
	"aego/script/source"
	"aego/script/workspace"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	debounce   = 0.15
	hoverDelay = 0.5
	noteTime   = 4.0
	analyzing  = "The code is still being analyzed, try again in a moment"
	buildTip   = "The scripts do not build; click to see the errors in Problems. Hot reload keeps the latest build that worked"
)

type compState struct {
	open  bool
	start int
	items []langsvc.Item
	view  []int
	sel   int
}

type hoverState struct {
	off     int
	since   float64
	done    bool
	ok      bool
	info    langsvc.Hover
	hasDiag bool
	dg      diag.Diagnostic
	builtin bool
}

type findState struct {
	open      bool
	replace   bool
	matchCase bool
	focus     bool
	query     string
	repl      string
}

type mark struct {
	rev    uint64
	off    int
	anchor int
	snap   *workspace.Snapshot
}

type Panel struct {
	host      *Host
	doc       *document.ScriptDocument
	fid       core.ID
	clone     bool
	idx       int
	own       code.Selection
	ownRev    uint64
	ownSet    bool
	primVer   uint64
	mini      mapView
	scrollX   float32
	scrollY   float32
	lastRev   uint64
	sentRev   uint64
	editedAt  float64
	askedAt   float64
	waiting   bool
	snap      *workspace.Snapshot
	snapRev   uint64
	semAt     uint64
	semVer    uint64
	marksAt   uint64
	diags     []diag.Diagnostic
	marks     map[int]diag.Severity
	sem       []highlight.Sem
	occ       []highlight.Sem
	occWrite  []bool
	occCur    int
	occT      settle
	brKey     mark
	brA       int
	brB       int
	brState   code.BracketState
	block     code.Block
	blockOK   bool
	blockKey  mark
	guides    []int
	guideAt   int
	views     map[int]*lineView
	comp      compState
	hover     hoverState
	find      findState
	outline   outlineState
	peek      peekState
	sig       sigState
	sigKey    mark
	inlays    map[int][]inlay
	inlayKey  mark
	inlayVer  uint64
	selStack  []code.Selection
	selTop    code.Selection
	crumbs    []crumb
	crumbKey  mark
	lens      []lens
	lensKey   mark
	note      string
	noteAt    float64
	blinkAt   float64
	flashLine int
	flashAt   float64
	conflict  bool
	dragging  bool
	reveal    bool
	wantFocus bool
	area      mathx.Rect
	caret     mathx.Vec2
	lineH     float32
	viewLines int
	line      []byte
	pad       float32
	pid       string
}

func newPanel(h *Host, d *document.ScriptDocument) *Panel {
	return &Panel{
		host:      h,
		doc:       d,
		fid:       core.IDOf("code/" + d.Path),
		lastRev:   ^uint64(0),
		sentRev:   ^uint64(0),
		marksAt:   ^uint64(0),
		marks:     map[int]diag.Severity{},
		views:     map[int]*lineView{},
		inlays:    map[int][]inlay{},
		hover:     hoverState{off: -1},
		flashLine: -1,
		occCur:    -1,
	}
}

func (p *Panel) ID() string {
	if p.pid == "" {
		p.pid = PanelID(p.doc.Path)
		if p.clone {
			p.pid += "#" + strconv.Itoa(p.idx)
		}
	}
	return p.pid
}

func (p *Panel) Title() string {
	return p.host.tabTitle(p.doc.Path)
}

func (p *Panel) Scope() string {
	return Scope
}

func (p *Panel) file() (source.FileID, bool) {
	return p.host.ws.FileID(p.doc.Path)
}

func (p *Panel) synced() bool {
	return p.snap != nil && p.snapRev == p.doc.Code.Rev()
}

func (p *Panel) resync() {
	p.lastRev, p.sentRev, p.marksAt = ^uint64(0), ^uint64(0), ^uint64(0)
	p.snap, p.diags, p.sem = nil, nil, nil
	p.clearOcc()
	p.occT = settle{}
	p.brKey, p.blockKey, p.crumbKey, p.lensKey, p.inlayKey = mark{}, mark{}, mark{}, mark{}, mark{}
	p.crumbs, p.lens, p.selStack = p.crumbs[:0], p.lens[:0], p.selStack[:0]
	clear(p.inlays)
	p.inlayVer++
	p.semVer++
	clear(p.views)
	p.comp.open = false
	p.peek.open = false
	p.sig.open = false
	p.outline.open = false
	p.conflict = false
}

func (p *Panel) push() uint64 {
	c := p.doc.Code
	if c.Rev() == p.sentRev {
		return p.host.lang.Current()
	}
	p.sentRev = c.Rev()
	p.host.ws.SetOverlay(p.doc.Path, c.Bytes())
	return p.host.lang.Invalidate()
}

func (p *Panel) ask(now float64) {
	p.waiting, p.askedAt = false, now
	gen := p.push()
	rev := p.sentRev
	file, ok := p.file()
	if !ok {
		return
	}
	p.host.lang.Analyze(gen, file, func(r langsvc.Result) { p.receive(rev, r) })
}

func (p *Panel) receive(rev uint64, r langsvc.Result) {
	file, ok := p.file()
	if !ok || r.Snap == nil || (p.snap != nil && rev < p.snapRev) {
		return
	}
	p.host.setLive(r.Snap)
	p.snap, p.snapRev, p.semAt = r.Snap, rev, rev
	p.diags = p.diags[:0]
	for _, d := range r.Diagnostics {
		if d.Primary.File == file && d.Severity != diag.Info {
			p.diags = append(p.diags, d)
		}
	}
	p.sem = p.sem[:0]
	for _, t := range r.Tokens {
		p.sem = append(p.sem, highlight.Sem{Start: int(t.Span.Start), End: int(t.Span.End), Kind: semKind(t.Kind)})
	}
	p.semVer++
	p.marksAt = ^uint64(0)
	p.remap()
}

func (p *Panel) remap() {
	c := p.doc.Code
	rev := c.Rev()
	if p.semAt == rev {
		return
	}
	n := 0
	for _, d := range p.diags {
		s, e, ok := c.MapSpan(p.semAt, int(d.Primary.Start), int(d.Primary.End))
		if !ok {
			continue
		}
		d.Primary.Start, d.Primary.End = uint32(s), uint32(e)
		p.diags[n] = d
		n++
	}
	p.diags = p.diags[:n]
	m := 0
	for _, t := range p.sem {
		s, e, ok := c.MapSpan(p.semAt, t.Start, t.End)
		if !ok || e-s != t.End-t.Start {
			continue
		}
		p.sem[m] = highlight.Sem{Start: s, End: e, Kind: t.Kind}
		m++
	}
	p.sem = p.sem[:m]
	p.semAt = rev
	p.semVer++
	p.marksAt = ^uint64(0)
}

func (p *Panel) updateMarks() {
	c := p.doc.Code
	if p.marksAt == c.Rev() {
		return
	}
	p.marksAt = c.Rev()
	clear(p.marks)
	for _, d := range p.diags {
		l := c.Line(int(d.Primary.Start))
		if s, has := p.marks[l]; !has || d.Severity < s {
			p.marks[l] = d.Severity
		}
	}
}

func (p *Panel) sync(now float64) {
	if p.doc.ReadOnly {
		return
	}
	if p.clone {
		p.follow()
		p.remap()
		return
	}
	c := p.doc.Code
	if c.Rev() != p.lastRev {
		p.lastRev = c.Rev()
		p.editedAt = now
		p.waiting = true
	}
	switch {
	case p.waiting && (now-p.editedAt >= debounce || p.askedAt == 0):
		p.ask(now)
	case !p.waiting && !p.synced() && now-p.askedAt > 1:
		p.ask(now)
	}
	p.remap()
}

func (p *Panel) snapOffset() (int, bool) {
	if p.snap == nil {
		return 0, false
	}
	return p.doc.Code.Unmap(p.snapRev, p.doc.Code.Cursor())
}

func (p *Panel) diagAt(off int) (diag.Diagnostic, bool) {
	var best diag.Diagnostic
	found := false
	for _, d := range p.diags {
		s, e := int(d.Primary.Start), int(d.Primary.End)
		if e <= s {
			e = s + 1
		}
		if off < s || off >= e {
			continue
		}
		size := e - s
		bs := int(best.Primary.End) - int(best.Primary.Start)
		if !found || d.Severity < best.Severity || (d.Severity == best.Severity && size < bs) {
			best, found = d, true
		}
	}
	return best, found
}

func (p *Panel) noteWhere(n diag.Note) string {
	if p.snap == nil || !n.Span.Valid() {
		return ""
	}
	sm := p.snap.Sources
	line, _ := sm.LineCol(n.Span.File, n.Span.Start)
	return " (" + path.Base(sm.Path(n.Span.File)) + ":" + strconv.Itoa(line) + ")"
}

func (p *Panel) flash(now float64, msg string) {
	p.note, p.noteAt = msg, now
}

func (p *Panel) closePopups() {
	p.comp.open = false
	p.peek.open = false
	p.sig.open = false
	p.outline.open = false
	p.hover = hoverState{off: -1}
	p.note = ""
}

func (p *Panel) save() {
	if err := p.host.docs().SaveScript(p.doc); err != nil {
		p.host.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	p.host.d.Notices.Notify(context.LevelInfo, "Saved "+p.doc.Path)
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	p.restoreSel()
	defer p.saveSel()
	p.sync(ui.Time)
	if p.wantFocus {
		ui.SetFocus(p.fid)
		p.wantFocus = false
	}
	if ui.Focus() == p.fid {
		p.host.active = p
	}
	zen := p.inZen()
	y := r.Y
	if !zen {
		bar := mathx.Rect{X: r.X, Y: y, W: r.W, H: th.RowHeight}
		p.drawBar(ui, bar)
		y += bar.H
	}
	if p.conflict {
		b := mathx.Rect{X: r.X, Y: y, W: r.W, H: th.RowHeight + 4}
		p.drawConflict(ui, b)
		y += b.H
	}
	if p.find.open {
		h := th.RowHeight + 4
		if p.find.replace {
			h = 2*th.RowHeight + 8
		}
		fr := mathx.Rect{X: r.X, Y: y, W: r.W, H: h}
		p.drawFind(ui, fr)
		y += fr.H
	}
	if !zen {
		cr := mathx.Rect{X: r.X, Y: y, W: r.W, H: th.RowHeight - 2}
		p.drawCrumbs(ui, cr)
		y += cr.H
	}
	p.area = mathx.Rect{X: r.X, Y: y, W: r.W, H: max(0, r.Y+r.H-y)}
	p.drawText(ui, p.area)
	p.drawPopups(ui)
}

func (p *Panel) drawBar(ui *core.Context, bar mathx.Rect) {
	th := &ui.Theme
	ui.Rect(bar, th.Header)
	label := p.doc.Path
	if p.doc.Dirty() {
		label += " " + document.DirtyMark
	}
	errors, warns := 0, 0
	for _, d := range p.diags {
		if d.Severity == diag.Error {
			errors++
		} else {
			warns++
		}
	}
	right := bar.X + bar.W - th.Padding
	status, failed := p.host.buildStatus()
	if p.doc.ReadOnly || !failed {
		status = ""
	}
	sw := ui.TextSize(status).X
	sr := mathx.Rect{X: right - sw - 8, Y: bar.Y, W: sw + 8, H: bar.H}
	if status == "" {
		sr = mathx.Rect{X: right, Y: bar.Y}
	} else {
		sid := core.IDSeq(p.fid, 4)
		if ui.Clicked(sid, sr) {
			p.host.d.Nav.Open(context.ProblemsAt(""))
		}
		widgets.Tooltip(ui, sid, buildTip)
	}
	col := th.Error
	ui.PushClip(mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, right-bar.X-4), H: bar.H})
	ty := ui.TextY(bar)
	ui.Text(label, mathx.Vec2{X: bar.X + th.Padding, Y: ty}, th.Text)
	if p.doc.Note != "" {
		ui.Text(p.doc.Note, mathx.Vec2{X: bar.X + th.Padding + ui.TextSize(label).X + 16, Y: ty}, th.Warn)
	}
	if errors > 0 || warns > 0 {
		local := strconv.Itoa(errors) + " errors, " + strconv.Itoa(warns) + " warnings"
		lc := th.Warn
		if errors > 0 {
			lc = th.Error
		}
		ui.Text(local, mathx.Vec2{X: sr.X - ui.TextSize(local).X - 16, Y: ty}, lc)
	}
	ui.Text(status, mathx.Vec2{X: sr.X + 4, Y: ty}, col)
	ui.PopClip()
}

func (p *Panel) drawConflict(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	bg := th.Warn
	bg.A = 0.18
	ui.Rect(r, bg)
	ui.Text("The file changed on disk while you were editing it", mathx.Vec2{X: r.X + th.Padding, Y: ui.TextY(r)}, th.Warn)
	bw := float32(96)
	keep := mathx.Rect{X: r.X + r.W - bw - 4, Y: r.Y + 2, W: bw, H: r.H - 4}
	reload := mathx.Rect{X: keep.X - bw - 4, Y: keep.Y, W: bw, H: keep.H}
	if widgets.ButtonRect(ui, core.IDSeq(p.fid, 2), reload, "Reload") {
		if err := p.host.docs().ReloadScript(p.doc); err != nil {
			p.host.d.Notices.Notify(context.LevelError, err.Error())
		}
		p.resync()
	}
	if widgets.ButtonRect(ui, core.IDSeq(p.fid, 3), keep, "Keep mine") {
		p.host.docs().KeepScript(p.doc)
		p.conflict = false
	}
}
