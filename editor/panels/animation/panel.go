package animation

import (
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"aego/anim"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/context"
	"aego/editor/core/history"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/mutate"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	ID           = "animation"
	playerName   = "animation.AnimationPlayer"
	animatorName = "animation.Animator"
	nameW        = float32(220)
	rulerH       = float32(22)
	laneH        = float32(18)
	pad          = float32(10)
	keyR         = float32(5)
	markLife     = 0.8
)

const (
	dragNone uint8 = iota
	dragKeys
	dragScrub
	dragEvent
)

var (
	interps = [...]string{"bezier", "linear", "step"}
)

type Deps struct {
	Anim             *animedit.Registry
	Docs             services.Documents
	Selection        services.Selection
	Notices          services.Notices
	Files            animedit.Files
	PathOf           func(ids.AssetID) (string, bool)
	Events           func() []string
	Thumbs           context.ThumbSource
	AssetRev         func() uint64
	EditSourceArt    func(ids.AssetID) error
	CanEditSourceArt func(ids.AssetID) bool
}

type candKey struct {
	node ids.NodeID
	ver  uint64
	set  bool
}

type labelKey struct {
	t, length, fps float32
	loop           anim.LoopMode
}

type drag struct {
	kind  uint8
	start float32
	delta float32
	event int
}

type shown struct {
	name string
	t    float32
	at   float64
}

type frame struct {
	x0, w, length, step, t0, span float32
}

func (g frame) x(t float32) float32 { return g.x0 + (t-g.t0)/g.span*g.w }
func (g frame) t(x float32) float32 { return g.t0 + (x-g.x0)/g.w*g.span }

func (g frame) snap(t float32) float32 {
	return mathx.Clamp(float32(math.Round(float64(t/g.step)))*g.step, -g.length, g.length)
}

type Panel struct {
	rowRects  []trackRect
	d         Deps
	f         *services.Frame
	svc       *animedit.Service
	clip      *anim.Clip
	view      animedit.View
	sel       map[animedit.KeyRef]bool
	drag      drag
	scroll    float32
	cands     []string
	names     []string
	candKey   candKey
	label     string
	labelOf   labelKey
	ticks     []string
	tickOf    [2]float32
	pick      int
	failed    string
	marks     []shown
	buf       []animedit.Mark
	menuT     float32
	menuEv    int
	pressed   bool
	t0        float32
	span      float32
	curves    bool
	ctrack    int
	lane      int
	cst       *widgets.CurveState
	cbase     curve.Curve
	ccur      curve.Curve
	cof       curveKey
	ctx       *history.Transaction
	thumbs    map[ids.AssetID]texture.Region
	viewOf    string
	keyMenuID core.ID
	rev       uint64
}

func New(d Deps) *Panel {
	return &Panel{d: d, sel: map[animedit.KeyRef]bool{}, ctrack: -1, cst: widgets.NewCurveState(), thumbs: map[ids.AssetID]texture.Region{}}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	if p.svc != nil && p.svc.Mode() == animedit.Record {
		return i18n.Tr("editor.animation.title_rec", "Animation (Rec)")
	}
	return i18n.Tr("editor.animation.title", "Animation")
}

func (p *Panel) notify(err error) {
	if err != nil && p.d.Notices != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
	}
}

func owner(s *scene.Scene, h scene.NodeHandle) (scene.NodeHandle, *schema.TypeDescriptor) {
	reg := s.Registry()
	for n := h; !n.IsZero() && n != s.Root() && s.IsAlive(n); n = s.Parent(n) {
		for _, name := range [...]string{playerName, animatorName} {
			if td, ok := reg.ByName(name); ok {
				if _, has := s.GetByType(n, td.ID); has {
					return n, td
				}
			}
		}
	}
	return scene.NodeHandle{}, nil
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	p.f = f
	ui := f.UI
	d := p.d.Docs.Doc()
	switch {
	case d == nil || p.d.Anim == nil:
		widgets.LabelDim(ui, i18n.Tr("editor.animation.open_a_scene_to_animate", "Open a scene to animate its nodes"))
		return
	case d.IsPlay():
		widgets.LabelDim(ui, i18n.Tr("editor.animation.clips_are_edited_outside_play", "Clips are edited outside Play"))
		return
	}
	s := d.Scene()
	svc := p.d.Anim.For(d)
	if svc != p.svc {
		p.svc, p.clip, p.candKey = svc, nil, candKey{}
		clear(p.sel)
	}
	from := svc.Target()
	if sel := p.d.Selection.Selected(); len(sel) > 0 {
		from = sel[0]
	}
	node, td := owner(s, s.Handle(from))
	if node.IsZero() {
		widgets.LabelDim(ui, i18n.Tr("editor.animation.select_a_node_with_an", "Select a node with an Animation Player or an Animator, or Create → Animation → Clip"))
		return
	}
	p.pickClip(s, node, td, d.History.Version())
	if svc.Clip() == nil {
		widgets.LabelDim(ui, i18n.Tr("editor.animation.the_node_has_no_clip", "The node has no clip yet: set the Clip of its Animation Player or the clips of its controller"))
		return
	}
	if p.d.AssetRev != nil {
		if rev := p.d.AssetRev(); rev != p.rev {
			p.rev = rev
			clear(p.thumbs)
		}
	}
	if svc.ClipPath() != p.viewOf {
		p.viewOf = svc.ClipPath()
		p.resetView()
	}
	if svc.Clip() != p.clip {
		p.clip = svc.Clip()
		p.view = svc.View()
		p.prune()
	}
	p.collectMarks()
	p.toolbar()
	if sk := svc.Skipped(); len(sk) > 0 && svc.Mode() != animedit.Normal {
		widgets.LabelDim(ui, i18n.Tr("editor.animation.not_previewed_sk", "Not previewed: {sk}", i18n.Str("sk", sk[0])))
	}
	top := ui.Cursor().Y
	body := mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
	p.timeline(body)
	if svc.Mode() == animedit.Record {
		ui.Outline(r, ui.Theme.Record)
	}
}

func (p *Panel) candidates(s *scene.Scene, node scene.NodeHandle, td *schema.TypeDescriptor) []string {
	field := "clip"
	if td.FullName == animatorName {
		field = "controller"
	}
	fd, ok := td.Field(field)
	if !ok {
		return nil
	}
	v, err := s.GetProperty(node, td.ID, fd.ID)
	if err != nil || v.Asset.IsZero() {
		return nil
	}
	rel, ok := p.d.PathOf(v.Asset)
	if !ok {
		return nil
	}
	if field == "clip" {
		return []string{rel}
	}
	data, err := p.d.Files.Read(rel)
	if err != nil {
		return nil
	}
	cd, err := mutate.LoadCtrl(data, rel)
	if err != nil {
		return nil
	}
	g, err := cd.Graph()
	if err != nil {
		return nil
	}
	var out []string
	for _, st := range g.States {
		if c, ok := p.d.PathOf(st.Clip); ok && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// PickClip shows a clip of the node's list, for screenshots and tests.
func (p *Panel) PickClip(rel string) bool {
	if i := slices.Index(p.cands, rel); i >= 0 {
		p.pick = i
		return true
	}
	return false
}

func (p *Panel) pickClip(s *scene.Scene, node scene.NodeHandle, td *schema.TypeDescriptor, ver uint64) {
	svc := p.svc
	id := s.ID(node)
	if svc.Target() != id {
		svc.SetTarget(id)
	}
	if key := (candKey{node: id, ver: ver, set: true}); key != p.candKey {
		p.candKey = key
		if list := p.candidates(s, node, td); !slices.Equal(list, p.cands) {
			p.cands, p.pick = list, 0
			p.names = p.names[:0]
			for _, c := range list {
				p.names = append(p.names, strings.TrimSuffix(path.Base(c), path.Ext(c)))
			}
			if i := slices.Index(list, svc.ClipPath()); i >= 0 {
				p.pick = i
			}
		}
	}
	if p.pick >= len(p.cands) {
		return
	}
	want := p.cands[p.pick]
	if svc.ClipPath() == want || p.failed == want {
		return
	}
	if err := svc.SetClip(want); err != nil {
		p.failed = want
		p.notify(err)
		return
	}
	p.failed = ""
}

func (p *Panel) collectMarks() {
	p.buf = p.svc.TakeMarks(p.buf)
	for _, m := range p.buf {
		p.marks = append(p.marks, shown{name: m.Name, t: m.T, at: p.f.Now})
	}
	n := 0
	for _, m := range p.marks {
		if p.f.Now-m.at < markLife {
			p.marks[n] = m
			n++
		}
	}
	p.marks = p.marks[:n]
}

func (p *Panel) prune() {
	for k := range p.sel {
		keep := false
		if k.Track >= 0 && k.Track < len(p.clip.Tracks) {
			for _, key := range p.clip.Tracks[k.Track].Keys {
				if key.T == k.T {
					keep = true
					break
				}
			}
		}
		if !keep {
			delete(p.sel, k)
		}
	}
}

func clock(t, fps float32) string {
	n := int(math.Round(float64(fps)))
	if n <= 0 {
		n = 60
	}
	total := int(math.Round(float64(t) * float64(n)))
	sec, fr := total/n, total%n
	return strconv.Itoa(sec/60) + ":" + pad2(sec%60) + "." + pad2(fr)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func (p *Panel) toolbar() {
	ui := p.f.UI
	th := &ui.Theme
	svc := p.svc
	bar := ui.Cursor().Next(0)
	x := bar.X
	base := ui.ID("tool")
	n := 0
	icon := func(name, tip string, on bool) bool {
		r := mathx.Rect{X: x, Y: bar.Y, W: bar.H, H: bar.H}
		x += bar.H + 2
		n++
		hit := widgets.IconButton(ui, core.IDSeq(base, n), r, name, tip)
		if on {
			ui.Rect(mathx.Rect{X: r.X + 2, Y: r.Y + r.H - 2, W: r.W - 4, H: 2}, th.Accent)
		}
		return hit
	}
	if len(p.cands) > 1 {
		cr := mathx.Rect{X: x, Y: bar.Y, W: 180, H: bar.H}
		x += 184
		if widgets.Combo(ui, ui.ID("clip"), cr, &p.pick, p.names, false) {
			p.failed = ""
		}
	} else {
		name := strings.TrimSuffix(path.Base(svc.ClipPath()), path.Ext(svc.ClipPath()))
		ui.Text(name, mathx.Vec2{X: x, Y: ui.TextY(bar)}, th.Text)
		x += ui.TextSize(name).X + 10
	}
	labeled := func(name, label string, tint mathx.Color, on bool) bool {
		w := ui.TextSize(label).X + 36
		r := mathx.Rect{X: x, Y: bar.Y, W: w, H: bar.H}
		x += w + 4
		n++
		hit := widgets.ButtonRect(ui, core.IDSeq(base, n), r, "")
		widgets.IconAt(ui, mathx.Rect{X: r.X + 8, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, name, tint)
		ui.Text(label, mathx.Vec2{X: r.X + 28, Y: ui.TextY(r)}, th.Text)
		if on {
			ui.Rect(mathx.Rect{X: r.X + 2, Y: r.Y + r.H - 2, W: r.W - 4, H: 2}, th.Accent)
		}
		return hit
	}
	mode := svc.Mode()
	if labeled("anim.preview", i18n.Tr("editor.animation.preview", "Preview"), th.Text, mode != animedit.Normal) {
		if mode == animedit.Normal {
			svc.SetMode(animedit.Preview)
		} else {
			svc.SetMode(animedit.Normal)
		}
	}
	if labeled("anim.record", i18n.Tr("editor.animation.rec", "Rec"), ui.Theme.Record, mode == animedit.Record) {
		if mode == animedit.Record {
			svc.SetMode(animedit.Preview)
		} else {
			svc.SetMode(animedit.Record)
		}
	}
	if labeled("anim.key", i18n.Tr("editor.animation.auto_key", "Auto Key"), th.Text, svc.AutoKey) {
		svc.AutoKey = !svc.AutoKey
	}
	if labeled("ui.curve", i18n.Tr("editor.animation.curves", "Curves"), th.Text, p.curves) {
		p.curves = !p.curves
	}
	if icon("anim.to_start", i18n.Tr("editor.animation.go_to_start", "Go to start"), false) {
		svc.SetMode(max(mode, animedit.Preview))
		svc.Pause()
		svc.SetTime(0)
	}
	play, tip := "play.start", i18n.Tr("editor.animation.play_space", "Play (Space)")
	if svc.Playing() {
		play, tip = "play.pause", i18n.Tr("editor.animation.pause_space", "Pause (Space)")
	}
	if icon(play, tip, svc.Playing()) {
		if svc.Playing() {
			svc.Pause()
		} else {
			svc.Play()
		}
	}
	if icon("anim.to_end", i18n.Tr("editor.animation.go_to_end", "Go to end"), false) {
		svc.SetMode(max(mode, animedit.Preview))
		svc.Pause()
		svc.SetTime(p.clip.Length)
	}
	if k := (labelKey{t: svc.Time(), length: p.clip.Length, fps: p.clip.FPS, loop: p.clip.Loop}); k != p.labelOf || p.label == "" {
		p.labelOf = k
		p.label = clock(k.t, k.fps) + " / " + clock(k.length, k.fps) + "  " + k.loop.String()
	}
	ui.Text(p.label, mathx.Vec2{X: x + 6, Y: ui.TextY(bar)}, th.TextDim)
}

func (p *Panel) step() float32 {
	if fps := p.clip.FPS; fps > 0 {
		return 1 / fps
	}
	return 1.0 / 60
}

func (p *Panel) timeline(r mathx.Rect) {
	ui := p.f.UI
	th := &ui.Theme
	area := mathx.Rect{X: r.X + nameW, Y: r.Y, W: max(r.W-nameW, 40), H: r.H}
	g := p.frame(area)
	p.ruler(mathx.Rect{X: area.X, Y: r.Y, W: area.W, H: rulerH}, g)
	lane := mathx.Rect{X: area.X, Y: r.Y + rulerH, W: area.W, H: laneH}
	widgets.IconAt(ui, mathx.Rect{X: r.X + 6, Y: lane.Y + (lane.H-16)/2, W: 16, H: 16}, "anim.event", th.TextDim)
	ui.Text(i18n.Tr("editor.animation.events", "Events"), mathx.Vec2{X: r.X + 26, Y: ui.TextY(lane)}, th.TextDim)
	p.events(lane, g)
	body := mathx.Rect{X: r.X, Y: lane.Y + laneH, W: r.W, H: max(0, r.Y+r.H-lane.Y-laneH)}
	p.pressed = false
	dope := mathx.Rect{X: area.X, Y: body.Y, W: area.W, H: body.H}
	list := body
	if p.curves {
		list.W = nameW
	}
	keep := p.scroll
	p.rowRects = p.rowRects[:0]
	widgets.VirtualList(ui, ui.ID("rows"), list, len(p.view.Rows), th.RowHeight, &p.scroll, func(i int, row mathx.Rect) {
		p.row(i, row, g)
	})
	if area.Contains(ui.Mouse()) {
		p.scroll = keep
	}
	if p.curves {
		p.curveView(dope, g)
		ui.PushClip(area)
		x := g.x(p.svc.Time())
		ui.Rect(mathx.Rect{X: x - 0.75, Y: area.Y, W: 1.5, H: rulerH + laneH}, th.Accent)
		ui.PopClip()
		p.dragging(g)
		return
	}
	bg := ui.ID("dope")
	if !p.pressed && ui.Pressed(bg, dope) && !ui.Mods().Has(input.ModShift) {
		clear(p.sel)
	}
	p.keyMenuID = ui.ID("keymenu")
	if ui.RightClicked(bg, dope) {
		widgets.OpenContextMenu(ui, p.keyMenuID)
	}
	p.keyMenu()
	p.dragging(g)
	ui.PushClip(area)
	x := g.x(p.svc.Time())
	ui.Rect(mathx.Rect{X: x - 0.75, Y: area.Y, W: 1.5, H: area.H}, th.Accent)
	ui.PopClip()
}

func niceStep(g frame) float32 {
	raw := 80 * g.span / g.w
	for _, s := range [...]float32{1.0 / 60, 1.0 / 30, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60} {
		if s >= raw {
			return s
		}
	}
	return 60
}

func (p *Panel) ruler(r mathx.Rect, g frame) {
	ui := p.f.UI
	th := &ui.Theme
	ui.Rect(r, th.CardHeader)
	ui.PushClip(r)
	st := niceStep(g)
	if of := [2]float32{st, g.length}; of != p.tickOf || p.ticks == nil {
		p.tickOf, p.ticks = of, p.ticks[:0]
		for i := 0; float32(i)*st <= g.length+1e-4; i++ {
			p.ticks = append(p.ticks, widgets.FormatNumber(float64(float32(i)*st), widgets.NumberOpts{Prec: 2}))
		}
	}
	first := max(0, int(g.t0/st))
	for i := first; i < len(p.ticks); i++ {
		label := p.ticks[i]
		if float32(i)*st > g.t0+g.span {
			break
		}
		x := g.x(float32(i) * st)
		ui.Rect(mathx.Rect{X: x, Y: r.Y + r.H - 6, W: 1, H: 6}, th.TextDim)
		ui.Text(label, mathx.Vec2{X: x + 2, Y: r.Y + 1}, th.TextDim)
	}
	ui.PopClip()
	if ui.Pressed(ui.ID("ruler"), r) {
		svc := p.svc
		if svc.Mode() == animedit.Normal {
			svc.SetMode(animedit.Preview)
		}
		svc.Pause()
		p.drag = drag{kind: dragScrub}
		ui.CaptureMouse(ui.ID("drag"))
		svc.SetTime(g.snap(g.t(ui.Mouse().X)))
	}
}

func (p *Panel) events(r mathx.Rect, g frame) {
	ui := p.f.UI
	th := &ui.Theme
	faint := th.Border
	faint.A *= 0.4
	ui.Rect(r, faint)
	ui.PushClip(r)
	base := ui.ID("event")
	hit := false
	for i, e := range p.view.Events {
		t := e.T
		if p.drag.kind == dragEvent && p.drag.event == i {
			t = mathx.Clamp(t+p.drag.delta, 0, g.length)
		}
		x := g.x(t)
		box := mathx.Rect{X: x - 5, Y: r.Y, W: 10, H: r.H}
		id := core.IDSeq(base, i+1)
		if ui.Pressed(id, box) {
			hit = true
			p.drag = drag{kind: dragEvent, start: g.t(ui.Mouse().X), event: i}
			ui.CaptureMouse(ui.ID("drag"))
		}
		if ui.RightClicked(id, box) {
			hit = true
			p.menuEv = i
			widgets.OpenContextMenu(ui, ui.ID("eventmenu"))
		}
		col := th.Events[min(int(e.Mode), len(th.Events)-1)]
		ui.Rect(mathx.Rect{X: x - 1, Y: r.Y + 2, W: 2, H: r.H - 4}, col)
		ui.Rect(mathx.Rect{X: x - 4, Y: r.Y + 2, W: 8, H: 4}, col)
		widgets.TooltipAfter(ui, id, e.Name, 0.3)
	}
	for _, m := range p.marks {
		a := float32(1 - (p.f.Now-m.at)/markLife)
		col := th.Events[anim.EventPreview]
		col.A *= a
		ui.Text(m.name, mathx.Vec2{X: g.x(m.t) + 6, Y: r.Y + 1}, col)
	}
	ui.PopClip()
	lane := ui.ID("events")
	if !hit && ui.Pressed(lane, r) && ui.DoubleClicked(lane) {
		p.menuT = mathx.Clamp(g.snap(g.t(ui.Mouse().X)), 0, g.length)
		widgets.OpenContextMenu(ui, ui.ID("addevent"))
	}
	if widgets.ContextMenu(ui, ui.ID("addevent")) {
		var names []string
		if p.d.Events != nil {
			names = p.d.Events()
		}
		for _, n := range names {
			if widgets.MenuItem(ui, n, "", true, false) {
				p.notify(p.svc.EditClip(i18n.Tr("editor.history.add_event", "Add Event"), mutate.AnimEventAdd(strconv.FormatFloat(float64(p.menuT), 'g', -1, 32), n)))
			}
		}
		if len(names) == 0 {
			widgets.MenuItem(ui, i18n.Tr("editor.animation.declare_events_in_project_settings", "Declare events in Project Settings → Animation"), "", false, false)
		}
		widgets.EndContextMenu(ui)
	}
	if widgets.ContextMenu(ui, ui.ID("eventmenu")) {
		if widgets.MenuItem(ui, i18n.Tr("editor.animation.remove_event", "Remove Event"), "", true, false) {
			p.notify(p.svc.EditClip(i18n.Tr("editor.history.remove_event", "Remove Event"), mutate.AnimEventRemove(p.view.Events[p.menuEv].Index)))
		}
		widgets.EndContextMenu(ui)
	}
}

func diamond(ui *core.Context, c mathx.Vec2, r float32, col mathx.Color, square bool) {
	if square {
		ui.Rect(mathx.Rect{X: c.X - r + 1, Y: c.Y - r + 1, W: 2*r - 2, H: 2*r - 2}, col)
		return
	}
	for y := -r; y <= r; y++ {
		w := r - float32(math.Abs(float64(y)))
		ui.Rect(mathx.Rect{X: c.X - w, Y: c.Y + y, W: 2 * w, H: 1}, col)
	}
}

func (p *Panel) row(i int, rr mathx.Rect, g frame) {
	ui := p.f.UI
	th := &ui.Theme
	row := &p.view.Rows[i]
	name := mathx.Rect{X: rr.X, Y: rr.Y, W: nameW, H: rr.H}
	dope := mathx.Rect{X: rr.X + nameW, Y: rr.Y, W: rr.W - nameW, H: rr.H}
	col := th.Text
	if row.Track < 0 {
		col = th.TextDim
		ui.Rect(rr, th.CardHeader)
	}
	if p.curves && row.Track >= 0 {
		nid := core.IDSeq(ui.ID("name"), row.Track+1)
		if ui.Pressed(nid, name) && p.svc.Lanes(row.Track) > 0 {
			p.ctrack, p.lane = row.Track, 0
		}
		switch {
		case row.Track == p.ctrack:
			ui.Rect(name, th.Selection)
		case p.svc.Lanes(row.Track) == 0:
			col = th.TextDisabled
		}
	}
	ui.PushClip(name)
	ui.Text(row.Label, mathx.Vec2{X: rr.X + 6 + float32(row.Depth)*14, Y: ui.TextY(rr)}, col)
	ui.PopClip()
	if row.Track < 0 || p.curves {
		return
	}
	p.rowRects = append(p.rowRects, trackRect{track: row.Track, r: dope})
	p.dropSprites(row.Track, dope, g)
	ui.PushClip(dope)
	base := core.IDSeq(ui.ID("key"), row.Track+1)
	for k, t := range row.Keys {
		ref := animedit.KeyRef{Track: row.Track, T: t}
		sel := p.sel[ref]
		x := g.x(t)
		if sel && p.drag.kind == dragKeys {
			x = g.x(mathx.Clamp(t+p.drag.delta, 0, g.length))
		}
		c := mathx.Vec2{X: x, Y: rr.Y + rr.H/2}
		id := core.IDSeq(base, k+1)
		if ui.Pressed(id, mathx.Rect{X: c.X - keyR - 1, Y: rr.Y, W: 2*keyR + 2, H: rr.H}) {
			p.pressKey(ref, g)
		}
		kc := th.Text
		switch {
		case sel:
			kc = th.Accent
		case ui.Hot() == id:
			kc = th.TextDim
		}
		if k < len(row.Assets) && p.thumb(row, k, g) {
			reg := p.thumbOf(row.Assets[k])
			box := mathx.Rect{X: c.X - 8, Y: c.Y - 8, W: 16, H: 16}
			if !reg.IsZero() {
				ui.List().Texture(reg, box, mathx.White)
				if sel || ui.Hot() == id {
					ui.Outline(box, kc)
				}
				continue
			}
		}
		diamond(ui, c, keyR, kc, row.Step)
	}
	ui.PopClip()
}

func (p *Panel) pressKey(ref animedit.KeyRef, g frame) {
	ui := p.f.UI
	p.pressed = true
	switch {
	case ui.Mods().Has(input.ModShift):
		if p.sel[ref] {
			delete(p.sel, ref)
			return
		}
		p.sel[ref] = true
	case !p.sel[ref]:
		clear(p.sel)
		p.sel[ref] = true
	}
	p.drag = drag{kind: dragKeys, start: g.t(ui.Mouse().X)}
	ui.CaptureMouse(ui.ID("drag"))
}

func (p *Panel) selected() []animedit.KeyRef {
	out := make([]animedit.KeyRef, 0, len(p.sel))
	for k := range p.sel {
		out = append(out, k)
	}
	return out
}

func (p *Panel) dragging(g frame) {
	ui := p.f.UI
	if p.drag.kind == dragNone {
		return
	}
	if ui.MouseDown(input.MouseLeft) {
		t := g.t(ui.Mouse().X)
		switch p.drag.kind {
		case dragScrub:
			p.svc.SetTime(mathx.Clamp(g.snap(t), 0, g.length))
		default:
			p.drag.delta = g.snap(t - p.drag.start)
		}
		return
	}
	d := p.drag
	p.drag = drag{}
	ui.ReleaseMouse()
	if d.delta == 0 {
		return
	}
	switch d.kind {
	case dragKeys:
		moved, err := p.svc.MoveKeys(p.selected(), d.delta)
		if err != nil {
			p.notify(err)
			return
		}
		clear(p.sel)
		for _, k := range moved {
			p.sel[k] = true
		}
	case dragEvent:
		e := p.view.Events[d.event]
		to := mathx.Clamp(e.T+d.delta, 0, g.length)
		p.notify(p.svc.EditClip(i18n.Tr("editor.history.move_event", "Move Event"), mutate.AnimEventMove(e.Index, strconv.FormatFloat(float64(to), 'g', -1, 32))))
	}
}

func (p *Panel) keyMenu() {
	ui := p.f.UI
	if !widgets.ContextMenu(ui, ui.ID("keymenu")) {
		return
	}
	has := len(p.sel) > 0
	if id := p.selectedSprite(); !id.IsZero() && p.d.EditSourceArt != nil && p.d.CanEditSourceArt != nil && p.d.CanEditSourceArt(id) {
		if widgets.MenuItem(ui, i18n.Tr("editor.art.edit_source", "Edit Source Art"), "", true, false) {
			p.notify(p.d.EditSourceArt(id))
		}
	}
	if widgets.MenuItem(ui, i18n.Tr("editor.animation.copy_keys", "Copy Keys"), "Ctrl+C", has, false) {
		p.CopyKeys()
	}
	if widgets.MenuItem(ui, i18n.Tr("editor.animation.paste_keys", "Paste Keys"), "Ctrl+V", true, false) {
		p.notify(p.PasteKeys())
	}
	if widgets.MenuItem(ui, i18n.Tr("editor.animation.set_from_scene", "Set from Scene"), "", has, false) {
		p.svc.SetMode(max(p.svc.Mode(), animedit.Preview))
		p.notify(p.svc.SetFromScene(p.selected()))
	}
	if widgets.MenuItem(ui, i18n.Tr("editor.animation.delete_keys", "Delete Keys"), i18n.Tr("editor.animation.del", "Del"), has, false) {
		p.notify(p.DeleteKeys())
	}
	for _, in := range interps {
		if widgets.MenuItem(ui, i18n.Tr("editor.animation.interpolation_in", "Interpolation: {in}", i18n.Str("in", in)), "", has, false) {
			p.notify(p.svc.SetInterp(p.selected(), in))
		}
	}
	widgets.EndContextMenu(ui)
}

func (p *Panel) selectedSprite() ids.AssetID {
	if len(p.sel) != 1 {
		return ids.AssetID{}
	}
	for key := range p.sel {
		for _, row := range p.view.Rows {
			if row.Track != key.Track {
				continue
			}
			for i, t := range row.Keys {
				if t == key.T && i < len(row.Assets) {
					return row.Assets[i]
				}
			}
		}
	}
	return ids.AssetID{}
}

// Select replaces the key selection, for tests and other tools.
func (p *Panel) Select(keys ...animedit.KeyRef) {
	clear(p.sel)
	for _, k := range keys {
		p.sel[k] = true
	}
}

// KeyMenuOpen reports whether the key menu is open, for tests.
func (p *Panel) KeyMenuOpen() bool {
	return p.f != nil && p.keyMenuID != 0 && p.f.UI.PopupOpen(p.keyMenuID)
}

// dropSprites keys a sprite track with the sprites dragged onto it, the first where they land.
func (p *Panel) dropSprites(track int, r mathx.Rect, g frame) {
	c := p.svc.Clip()
	if c == nil || track >= len(c.Tracks) || c.Tracks[track].Kind != schema.KindAssetRef || p.f.DnD == nil || !p.f.DnD.Active() {
		return
	}
	ui := p.f.UI
	var frames []ids.AssetID
	pl, ok := p.f.DnD.Payload()
	if !ok {
		return
	}
	switch v := pl.Data.(type) {
	case context.AssetPayload:
		if pl.Kind == context.KindAsset && context.AssetMatches("texture", v.Kind) {
			frames = append(frames, v.ID)
		}
	case context.AssetsPayload:
		for _, it := range v.Items {
			if context.AssetMatches("texture", it.Kind) {
				frames = append(frames, it.ID)
			}
		}
	}
	if len(frames) == 0 {
		return
	}
	if _, ok := p.f.DnD.Target(ui, r, pl.Kind); ok {
		at := g.snap(mathx.Clamp(g.t(ui.Mouse().X), 0, g.length))
		p.notify(p.svc.KeySprites(track, at, frames))
	}
}

type trackRect struct {
	track int
	r     mathx.Rect
}

// TrackRect is where the keys of track i were drawn on the last frame.
func (p *Panel) TrackRect(i int) (mathx.Rect, bool) {
	for _, t := range p.rowRects {
		if t.track == i {
			return t.r, true
		}
	}
	return mathx.Rect{}, false
}
