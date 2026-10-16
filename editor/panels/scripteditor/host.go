package scripteditor

import (
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	edws "aego/editor/core/workspace"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/script/diag"
	"aego/script/langsvc"
	"aego/script/workspace"
	"aego/ui/core"
	"aego/ui/dock"
)

const (
	Scope         = "code"
	historyMax    = 64
	countsEvery   = 1.0
	refreshEvery  = 1.0
	AutosaveDelay = 1.0
	saveRetry     = 5.0
)

const (
	CommandFindInScripts  = "code.find_in_scripts"
	CommandBack           = "code.back"
	CommandForward        = "code.forward"
	CommandInlineErrors   = "code.inline_errors"
	CommandProblems       = "code.problems"
	CommandInlayHints     = "code.inlay_hints"
	CommandMinimap        = "code.minimap"
	CommandSplitRight     = "code.split_right"
	CommandSplitDown      = "code.split_down"
	CommandAutosave       = "file.autosave"
	CommandImplementation = "code.implementation"
	CommandRainbow        = "code.rainbow_brackets"
	CommandHighlightBlock = "code.highlight_block"
)

func PanelID(rel string) string {
	return "script:" + rel
}

type spot struct {
	path string
	off  int
}

type Deps struct {
	Docs      services.Documents
	Selection services.Selection
	Commands  *command.Registry
	Notices   services.Notices
	Nav       services.Navigation
	Problems  services.Problems
	Scripts   context.ScriptHost
	Viewport  *services.Viewport
	Now       func() float64
	Submit    func(func())
	OnMain    func(func())
}

type autosaveMark struct {
	rev uint64
	at  float64
}

type Host struct {
	lang       *langsvc.Service
	ws         *workspace.Workspace
	d          Deps
	dock       *edws.Workspace
	panels     map[string]*Panel
	clones     map[string]*Panel
	nextClone  int
	active     *Panel
	find       *FindPanel
	queue      []func()
	checkAt    float64
	refreshAt  float64
	refreshing bool
	back       []spot
	fwd        []spot
	live       []context.ScriptProblem
	liveRev    uint64
	liveSnap   *workspace.Snapshot
	symSnap    *workspace.Snapshot
	syms       []context.SearchItem
	counts     map[string]int
	countsAt   float64
	edits      map[*document.ScriptDocument]autosaveMark
	Inline     bool
	Hints      bool
	Minimap    bool
	Rainbow    bool
	Block      bool
	Autosave   bool
	Virtual    func(path string) (*document.ScriptDocument, error)
}

func NewHost(lang *langsvc.Service) *Host {
	h := &Host{
		lang:     lang,
		ws:       lang.Workspace(),
		panels:   map[string]*Panel{},
		clones:   map[string]*Panel{},
		counts:   map[string]int{},
		edits:    map[*document.ScriptDocument]autosaveMark{},
		Inline:   true,
		Hints:    true,
		Minimap:  true,
		Rainbow:  true,
		Block:    true,
		Autosave: true,
	}
	h.find = newFindPanel(h)
	h.Virtual = h.virtual
	return h
}

func (h *Host) docs() *document.Manager {
	return h.d.Docs.Documents()
}

func (h *Host) Attach(d Deps, w *edws.Workspace) {
	h.d, h.dock = d, w
	if err := w.Register(edws.Desc{Panel: h.find, Near: edws.Console, Order: 240}); err != nil {
		h.d.Notices.Notify(context.LevelError, err.Error())
	}
	h.registerCommands()
	for _, rec := range h.docs().FindScriptRecovery() {
		doc, err := h.docs().RestoreScriptRecovery(rec)
		if err != nil {
			h.d.Notices.Notify(context.LevelWarn, err.Error())
			continue
		}
		h.show(doc)
		h.d.Notices.Notify(context.LevelInfo, "Restored unsaved changes of "+doc.Path)
	}
}

func (h *Host) toggle(v *bool) func() error {
	return func() error {
		*v = !*v
		return nil
	}
}

func (h *Host) registerCommands() {
	hasActive := func() bool { return h.active != nil }
	list := []command.Desc{
		{ID: CommandFindInScripts, Name: "Find in Scripts", Category: "Scripts", Order: 100,
			Shortcut: command.Key(input.KeyF, command.ModPrimary|command.ModShift),
			Run: func() error {
				h.FindInScripts(h.query())
				return nil
			}},
		{ID: CommandBack, Name: "Back", Category: "Scripts", Order: 200,
			Shortcut: command.Key(input.KeyLeft, command.ModPrimary|command.ModAlt),
			Enabled:  func() bool { return len(h.back) > 0 },
			Run: func() error {
				h.Back()
				return nil
			}},
		{ID: CommandForward, Name: "Forward", Category: "Scripts", Order: 210,
			Shortcut: command.Key(input.KeyRight, command.ModPrimary|command.ModAlt),
			Enabled:  func() bool { return len(h.fwd) > 0 },
			Run: func() error {
				h.Forward()
				return nil
			}},
		{ID: CommandImplementation, Name: "Go to Implementation", Category: "Scripts", Order: 220,
			Shortcut: command.Key(input.KeyB, command.ModPrimary|command.ModAlt), Enabled: hasActive,
			Run: func() error {
				p := h.active
				h.queue = append(h.queue, func() {
					if h.owns(p) {
						p.implementation(h.d.Viewport.UI)
					}
				})
				return nil
			}},
		{ID: CommandProblems, Name: "Problems", Category: "Scripts", Order: 300,
			Run: func() error {
				h.d.Nav.Open(context.ProblemsAt(""))
				return nil
			}},
		{ID: CommandSplitRight, Name: "Split Right", Category: "Scripts", Order: 350, Enabled: hasActive,
			Run: func() error {
				h.split(h.active, false)
				return nil
			}},
		{ID: CommandSplitDown, Name: "Split Down", Category: "Scripts", Order: 360, Enabled: hasActive,
			Run: func() error {
				h.split(h.active, true)
				return nil
			}},
		{ID: CommandInlineErrors, Name: "Inline Error Messages", Category: "Scripts", Order: 400,
			Checked: func() bool { return h.Inline }, Run: h.toggle(&h.Inline)},
		{ID: CommandInlayHints, Name: "Parameter Name Hints", Category: "Scripts", Order: 410,
			Checked: func() bool { return h.Hints }, Run: h.toggle(&h.Hints)},
		{ID: CommandMinimap, Name: "Minimap", Category: "Scripts", Order: 420,
			Checked: func() bool { return h.Minimap }, Run: h.toggle(&h.Minimap)},
		{ID: CommandRainbow, Name: "Rainbow Brackets", Category: "Scripts", Order: 430,
			Checked: func() bool { return h.Rainbow }, Run: h.toggle(&h.Rainbow)},
		{ID: CommandHighlightBlock, Name: "Highlight Current Block", Category: "Scripts", Order: 440,
			Checked: func() bool { return h.Block }, Run: h.toggle(&h.Block)},
		{ID: CommandAutosave, Name: "Autosave Scripts", Category: "File", Order: 250,
			Checked: func() bool { return h.Autosave }, Run: h.toggle(&h.Autosave)},
	}
	for _, d := range list {
		if err := h.d.Commands.Register(d); err != nil {
			h.d.Notices.Notify(context.LevelError, err.Error())
		}
	}
}

func (h *Host) Open(rel string, line int) {
	h.Goto(context.At(rel, line, 1))
}

func (h *Host) OpenAt(rel string, off int) {
	h.Goto(context.AtOffset(rel, off))
}

func (h *Host) Goto(l context.Location) {
	h.queue = append(h.queue, func() { h.goTo(l, true) })
}

func (h *Host) document(p string) (*document.ScriptDocument, error) {
	if pn := h.panels[p]; pn != nil {
		return pn.doc, nil
	}
	if strings.HasPrefix(p, context.StubPrefix) || strings.HasPrefix(p, context.GoPrefix) {
		if h.Virtual == nil {
			return nil, errs.New(errs.EditorDocument, "library sources are not available")
		}
		return h.Virtual(p)
	}
	return h.docs().OpenScript(document.Clean(p))
}

func (h *Host) goTo(l context.Location, record bool) {
	if h.dock == nil {
		return
	}
	d, err := h.document(l.Path)
	if err != nil {
		h.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	if record {
		h.mark()
	}
	p := h.show(d)
	c := d.Code
	p.restoreSel()
	jump := true
	switch {
	case l.Offset >= 0:
		c.SetCursor(l.Offset, false)
	case l.Line > 0:
		c.SetCursor(c.Offset(l.Line-1, max(l.Col-1, 0)), false)
	default:
		jump = false
	}
	p.saveSel()
	p.wantFocus = true
	if jump {
		p.reveal = true
		p.flashLine, p.flashAt = c.Line(c.Cursor()), h.d.Now()
	}
	h.active = p
}

func (h *Host) byID(id string) *Panel {
	if p := h.clones[id]; p != nil {
		return p
	}
	for _, p := range h.panels {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

func (h *Host) query() string {
	if h.owns(h.active) {
		return h.active.selectionQuery()
	}
	return ""
}

func (h *Host) owns(p *Panel) bool {
	return p != nil && (h.panels[p.doc.Path] == p || h.clones[p.ID()] == p)
}

func (h *Host) current() (spot, bool) {
	p := h.active
	if !h.owns(p) {
		return spot{}, false
	}
	return spot{path: p.doc.Path, off: p.cursor()}, true
}

func (h *Host) mark() {
	s, ok := h.current()
	if !ok {
		return
	}
	if n := len(h.back); n > 0 && h.back[n-1] == s {
		return
	}
	h.back = append(h.back, s)
	if len(h.back) > historyMax {
		h.back = append(h.back[:0], h.back[1:]...)
	}
	h.fwd = h.fwd[:0]
}

func (h *Host) step(from, to *[]spot) {
	n := len(*from)
	if n == 0 {
		return
	}
	s := (*from)[n-1]
	*from = (*from)[:n-1]
	if cur, ok := h.current(); ok {
		*to = append(*to, cur)
	}
	h.goTo(context.AtOffset(s.path, s.off), false)
}

func (h *Host) Back() {
	h.queue = append(h.queue, func() { h.step(&h.back, &h.fwd) })
}

func (h *Host) Forward() {
	h.queue = append(h.queue, func() { h.step(&h.fwd, &h.back) })
}

func (h *Host) show(d *document.ScriptDocument) *Panel {
	if p := h.panels[d.Path]; p != nil {
		h.dock.Show(p.ID())
		return p
	}
	p := newPanel(h, d)
	if err := h.dock.Register(edws.Desc{Panel: p, Hidden: true}); err != nil {
		h.d.Notices.Notify(context.LevelError, err.Error())
		return p
	}
	h.panels[d.Path] = p
	if target := h.dock.Tree.FindPanel(edws.Scene); target != 0 {
		h.dock.Tree.DockPanel(p.ID(), target, dock.SideCenter)
	}
	h.dock.Show(p.ID())
	return p
}

func (h *Host) split(p *Panel, down bool) {
	if p == nil {
		return
	}
	h.queue = append(h.queue, func() {
		if !h.owns(p) {
			return
		}
		h.nextClone++
		c := newPanel(h, p.doc)
		c.clone, c.idx = true, h.nextClone
		c.fid = core.IDOf("code/" + c.ID())
		c.own, c.ownRev, c.ownSet = p.own, p.ownRev, p.ownSet
		c.scrollY = p.scrollY
		if err := h.dock.Register(edws.Desc{Panel: c, Hidden: true}); err != nil {
			h.d.Notices.Notify(context.LevelError, err.Error())
			return
		}
		h.clones[c.ID()] = c
		side := dock.SideRight
		if down {
			side = dock.SideBottom
		}
		h.dock.Tree.DockPanel(c.ID(), h.dock.Tree.FindPanel(p.ID()), side)
		h.dock.Show(c.ID())
		c.wantFocus = true
	})
}

func (h *Host) requestClose(p *Panel) {
	h.queue = append(h.queue, func() {
		if p.clone || p.doc.ReadOnly || !p.doc.Dirty() {
			h.close(p)
			return
		}
		h.d.Notices.Ask(context.Question{
			Title:   "Unsaved script",
			Message: p.doc.Path + " has unsaved changes.",
			Buttons: []string{"Save", "Discard", "Cancel"},
			Then: func(choice int) {
				switch choice {
				case 0:
					if err := h.docs().SaveScript(p.doc); err != nil {
						h.d.Notices.Notify(context.LevelError, err.Error())
						return
					}
					h.queue = append(h.queue, func() { h.close(p) })
				case 1:
					h.queue = append(h.queue, func() { h.close(p) })
				}
			},
		})
	})
}

func (h *Host) drop(p *Panel) {
	h.dock.Unregister(p.ID())
	p.dispose()
	if h.active == p {
		h.active = nil
	}
}

func (h *Host) close(p *Panel) {
	if p.clone {
		if h.clones[p.ID()] == p {
			delete(h.clones, p.ID())
			h.drop(p)
		}
		return
	}
	if h.panels[p.doc.Path] != p {
		return
	}
	for id, c := range h.clones {
		if c.doc == p.doc {
			delete(h.clones, id)
			h.drop(c)
		}
	}
	delete(h.panels, p.doc.Path)
	delete(h.edits, p.doc)
	h.drop(p)
	if !p.doc.ReadOnly {
		h.docs().CloseScript(p.doc)
		h.ws.ClearOverlay(p.doc.Path)
		h.lang.Invalidate()
	}
	if len(h.panels) == 0 {
		h.live, h.liveSnap = nil, nil
		h.liveRev++
	}
}

func (h *Host) eachView(d *document.ScriptDocument, fn func(*Panel)) {
	if p := h.panels[d.Path]; p != nil {
		fn(p)
	}
	for _, c := range h.clones {
		if c.doc == d {
			fn(c)
		}
	}
}

func (h *Host) Tick(now float64) {
	if h.dock == nil {
		return
	}
	q := h.queue
	h.queue = nil
	for _, fn := range q {
		fn()
	}
	h.find.tick(now)
	h.refreshCounts(now)
	h.refresh(now)
	h.autosave(now)
	if now < h.checkAt {
		return
	}
	h.checkAt = now + 1
	reloaded, conflicts := h.docs().CheckExternalScripts()
	for _, d := range reloaded {
		h.eachView(d, func(p *Panel) { p.resync() })
		h.d.Notices.Notify(context.LevelInfo, "Reloaded "+d.Path)
	}
	for _, d := range conflicts {
		h.eachView(d, func(p *Panel) { p.conflict = true })
	}
}

func (h *Host) refresh(now float64) {
	if h.refreshing || now < h.refreshAt {
		return
	}
	h.refreshAt = now + refreshEvery
	if s := h.lang.Latest(); s != nil && s.Gen == h.ws.Version() {
		return
	}
	h.refreshing = true
	h.d.Submit(func() {
		h.lang.Refresh()
		h.d.OnMain(func() { h.refreshing = false })
	})
}

func (h *Host) buildStatus() (string, bool) {
	if h.d.Scripts == nil {
		return "", false
	}
	gen, failed := h.d.Scripts.Status()
	n := 0
	ps, _ := h.d.Scripts.Problems()
	for _, pr := range ps {
		if pr.Error {
			n++
		}
	}
	s := "Scripts: build " + strconv.FormatUint(gen, 10)
	switch {
	case n > 0:
		return s + " · " + strconv.Itoa(n) + " errors", true
	case failed:
		return s + " · latest failed", true
	}
	return s + " · ok", false
}

func (h *Host) ensure() {
	if h.lang.Latest() == nil {
		h.lang.Refresh()
	}
}

func (h *Host) blocked(d *document.ScriptDocument) bool {
	p := h.panels[d.Path]
	return d.ReadOnly || (p != nil && p.conflict)
}

func (h *Host) saveDoc(d *document.ScriptDocument, now float64) bool {
	if err := h.docs().SaveScript(d); err != nil {
		h.d.Notices.Notify(context.LevelError, "autosave failed: "+err.Error())
		h.edits[d] = autosaveMark{rev: d.Code.Rev(), at: now + saveRetry}
		return false
	}
	delete(h.edits, d)
	return true
}

func (h *Host) autosave(now float64) {
	if !h.Autosave {
		return
	}
	for _, d := range h.docs().Scripts() {
		if !d.Dirty() || h.blocked(d) {
			delete(h.edits, d)
			continue
		}
		rev := d.Code.Rev()
		m, ok := h.edits[d]
		if !ok || m.rev != rev {
			h.edits[d] = autosaveMark{rev: rev, at: now}
			continue
		}
		if now-m.at >= AutosaveDelay {
			h.saveDoc(d, now)
		}
	}
}

func (h *Host) FlushAutosave() int {
	if h.dock == nil || !h.Autosave {
		return 0
	}
	now, n := h.d.Now(), 0
	for _, d := range h.docs().Scripts() {
		if d.Dirty() && !h.blocked(d) && h.saveDoc(d, now) {
			n++
		}
	}
	return n
}

func (h *Host) Caret(id string) (int, int, bool) {
	p := h.byID(id)
	if p == nil {
		return 0, 0, false
	}
	l, c := p.doc.Code.LineCol(p.cursor())
	return l + 1, c + 1, true
}

func (h *Host) setLive(snap *workspace.Snapshot) {
	if snap == nil || snap == h.liveSnap || (h.liveSnap != nil && snap.Gen < h.liveSnap.Gen) {
		return
	}
	h.liveSnap = snap
	h.live = h.live[:0]
	sm := snap.Sources
	for _, d := range snap.Diagnostics {
		if d.Severity == diag.Info || !d.Primary.Valid() {
			continue
		}
		line, col := sm.LineCol(d.Primary.File, d.Primary.Start)
		h.live = append(h.live, context.ScriptProblem{
			Package: snap.Packages[d.Primary.File],
			Path:    sm.Path(d.Primary.File),
			Line:    line,
			Col:     col,
			Code:    d.Code,
			Error:   d.Severity == diag.Error,
			Message: d.Message,
			Help:    diag.HelpText(d.Code),
		})
	}
	h.liveRev++
}

func (h *Host) LiveProblems() ([]context.ScriptProblem, uint64) {
	if len(h.panels) == 0 || h.liveSnap == nil {
		return nil, 0
	}
	return h.live, h.liveRev
}

func (h *Host) toContext(l langsvc.Location, snap *workspace.Snapshot) context.Location {
	if p := h.panels[l.Path]; p != nil && snap != nil && p.snap == snap {
		if s, _, ok := p.doc.Code.MapSpan(p.snapRev, int(l.Span.Start), int(l.Span.End)); ok {
			return context.AtOffset(l.Path, s)
		}
	}
	if snap != nil && l.Span.Valid() {
		line, col := snap.Sources.LineCol(l.Span.File, l.Span.Start)
		return context.At(l.Path, line, col)
	}
	return context.AtOffset(l.Path, int(l.Span.Start))
}

func (h *Host) Declaration(typeName, field string) (context.Location, bool) {
	h.ensure()
	l, ok := h.lang.ScriptDeclaration(typeName, field)
	if !ok {
		return context.Location{}, false
	}
	return h.toContext(l, h.lang.Latest()), true
}

func (h *Host) FindUsages(typeName, field string) {
	h.ensure()
	locs, snap := h.lang.ScriptUsages(typeName, field)
	title := typeName[strings.LastIndexByte(typeName, '.')+1:]
	if field != "" {
		title += "." + field
	}
	h.showUsages(title, locs, snap)
}

func (h *Host) showUsages(title string, locs []langsvc.Location, snap *workspace.Snapshot) {
	var ms []workspace.Match
	if snap != nil {
		sm := snap.Sources
		for _, l := range locs {
			line, col := sm.LineCol(l.Span.File, l.Span.Start)
			ls := sm.Offset(l.Span.File, line, 1)
			ms = append(ms, workspace.Match{
				Path:   l.Path,
				Line:   line,
				Col:    col,
				Offset: int(l.Span.Start - ls),
				Len:    int(l.Span.Len()),
				Text:   sm.Line(l.Span.File, line),
			})
		}
	}
	h.find.showUsages("Usages of "+title, ms)
	h.dock.Show(FindID)
}

func (h *Host) FindInScripts(q string) {
	h.find.start(q)
	h.dock.Show(FindID)
}

func (h *Host) Symbols() []context.SearchItem {
	h.ensure()
	snap := h.lang.Latest()
	if snap == nil || snap == h.symSnap {
		return h.syms
	}
	h.symSnap = snap
	h.syms = h.syms[:0]
	for _, s := range h.lang.WorkspaceSymbols() {
		detail := s.Path
		if s.Container != "" {
			detail = s.Container + " · " + s.Path
		}
		h.syms = append(h.syms, context.SearchItem{
			Label:  s.Name,
			Detail: detail,
			Kind:   s.Kind.String(),
			Loc:    context.At(s.Path, s.Line, s.Col),
		})
	}
	return h.syms
}

func (h *Host) refreshCounts(now float64) {
	if now < h.countsAt {
		return
	}
	h.countsAt = now + countsEvery
	clear(h.counts)
	for _, d := range h.docs().Docs() {
		scene.Store[scriptcomp.ScriptComponent](d.Scene()).EachAll(func(_ scene.NodeHandle, sc *scriptcomp.ScriptComponent) {
			for i := range sc.Attachments {
				h.counts[sc.Attachments[i].TypeName]++
			}
		})
	}
}

func attached(d *document.SceneDocument, full string) []ids.NodeID {
	s := d.Scene()
	var out []ids.NodeID
	scene.Store[scriptcomp.ScriptComponent](s).EachAll(func(n scene.NodeHandle, sc *scriptcomp.ScriptComponent) {
		if sc.Index(full) >= 0 && s.IsAlive(n) {
			out = append(out, s.ID(n))
		}
	})
	return out
}

func (h *Host) selectAttached(full string) {
	docs := h.docs()
	var target *document.SceneDocument
	var nodes []ids.NodeID
	if d := docs.Edit(); d != nil {
		if n := attached(d, full); len(n) > 0 {
			target, nodes = d, n
		}
	}
	if target == nil {
		for _, d := range docs.Docs() {
			if n := attached(d, full); len(n) > 0 {
				target, nodes = d, n
				break
			}
		}
	}
	if target == nil {
		h.d.Notices.Notify(context.LevelInfo, "No open scene uses "+full)
		return
	}
	docs.SetActive(target.ID)
	h.d.Selection.Select(nodes...)
	h.dock.Show(edws.Hierarchy)
}

func (h *Host) Renamed(oldRel, newRel string) {
	moved := func(p string) (string, bool) {
		switch {
		case p == oldRel:
			return newRel, true
		case strings.HasPrefix(p, oldRel+"/"):
			return newRel + strings.TrimPrefix(p, oldRel), true
		}
		return "", false
	}
	type move struct {
		from, to string
		p        *Panel
	}
	var list []move
	for path, pn := range h.panels {
		if np, ok := moved(path); ok {
			list = append(list, move{path, np, pn})
		}
	}
	for _, mv := range list {
		delete(h.panels, mv.from)
		h.panels[mv.to] = mv.p
		h.ws.ClearOverlay(mv.from)
		mv.p.resync()
	}
	for _, s := range [][]spot{h.back, h.fwd} {
		for i := range s {
			if np, ok := moved(s[i].path); ok {
				s[i].path = np
			}
		}
	}
	if len(list) > 0 {
		h.lang.Invalidate()
	}
}

func (h *Host) GotoSymbol(stub, symbol string) {
	h.queue = append(h.queue, func() {
		h.ensure()
		line := 1
		if st, ok := h.lang.Stub(stub); ok {
			if l, ok := st.Lines[symbol]; ok {
				line = l + 1
			}
		}
		h.goTo(context.At(stub, line, 1), true)
	})
}
