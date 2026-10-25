// Package addpalette fills the palette widget for Add Component and Create and applies the pick (D344).
package addpalette

import (
	"slices"
	"strconv"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/templates"
	"aego/editor/ext/inspect"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets/palette"
)

type Mode uint8

const (
	Components Mode = iota
	Create
	modeCount
)

const (
	confirmOver = 10
	maxRecent   = 8
	scriptsCat  = "Scripts"
)

type kind uint8

const (
	kindComp kind = iota
	kindScript
	kindTemplate
	kindAction
)

// Action is a Create item that is not a node template, such as a new clip for the selected node.
type Action struct {
	Key      string
	Name     string
	Category string
	Icon     string
	Doc      string
	Run      func(parent ids.NodeID) error
}

type entry struct {
	kind  kind
	td    *schema.TypeDescriptor
	tpl   templates.Template
	act   Action
	nodes []ids.NodeID
	skip  []string
}

type Deps struct {
	Ctx       *context.Editor
	Meta      *inspect.Registry
	Templates *templates.Registry
	Root      string
	Actions   func() []Action
}

type Service struct {
	d       Deps
	pal     *palette.Palette
	mode    Mode
	nodes   []ids.NodeID
	doc     *document.SceneDocument
	parent  ids.NodeID
	place   templates.Place
	created func([]ids.NodeID)
	at      *mathx.Vec2
	entries []entry
	items   []palette.Item
	store   store
	desc    desc
	loaded  bool
}

func New(d Deps) *Service {
	s := &Service{d: d, pal: palette.New(core.IDOf("addpalette"))}
	s.pal.CategoryIcon = CategoryIcon
	s.pal.Describe = s.describe
	return s
}

func (s *Service) Active() bool              { return s.pal.Active() }
func (s *Service) Palette() *palette.Palette { return s.pal }
func (s *Service) Mode() Mode                { return s.mode }

// AddComponent opens the palette for the selected nodes; at anchors it under a button, nil centres it.
func (s *Service) AddComponent(nodes []ids.NodeID, at *mathx.Vec2) {
	d := s.d.Ctx.Doc()
	if d == nil || len(nodes) == 0 {
		return
	}
	s.mode, s.doc, s.nodes, s.at = Components, d, slices.Clone(nodes), at
	s.open()
}

// CreateNode opens the palette of templates; the node goes under parent or at the place.
func (s *Service) CreateNode(parent ids.NodeID, pl templates.Place, at *mathx.Vec2, created func([]ids.NodeID)) {
	d := s.d.Ctx.Doc()
	if d == nil || s.d.Templates == nil {
		return
	}
	s.mode, s.doc, s.nodes, s.at = Create, d, nil, at
	s.parent, s.place, s.created = parent, pl, created
	s.open()
}

func (s *Service) open() {
	ui := s.d.Ctx.UI
	ui.CloseAllPopups()
	s.load()
	s.collect()
	s.pal.Show(ui)
	s.apply()
}

func (s *Service) Close() {
	s.pal.Hide(s.d.Ctx.UI)
	s.doc = nil
}

func (s *Service) apply() {
	st := &s.store.modes[s.mode]
	s.pal.Recent = s.pal.Recent[:0]
	for _, k := range st.recent {
		if i := s.index(k); i >= 0 {
			s.pal.Recent = append(s.pal.Recent, i)
		}
	}
	clear(s.pal.Open)
	for _, c := range st.open {
		s.pal.Open[c] = true
	}
	for i := range s.items {
		s.items[i].Starred = slices.Contains(st.favs, s.items[i].Key)
		s.items[i].Uses = s.store.uses[s.items[i].Key]
	}
	s.pal.SetItems(s.items)
	s.desc.item = -2
}

func (s *Service) index(key string) int {
	for i := range s.items {
		if s.items[i].Key == key {
			return i
		}
	}
	return -1
}

func (s *Service) collect() {
	s.entries, s.items = s.entries[:0], s.items[:0]
	switch s.mode {
	case Components:
		s.pal.Title = "Add Component"
		if n := len(s.nodes); n > 1 {
			s.pal.Title += " to " + strconv.Itoa(n) + " nodes"
		}
		s.pal.Hint = "Enter add · Shift+Enter add more · Ctrl+D favorite · Esc close"
		s.components()
		s.scripts()
	case Create:
		s.pal.Title = "Create"
		s.pal.Hint = "Enter create · Shift+Enter create more · Ctrl+D favorite · Esc close"
		s.templates()
	}
}

func (s *Service) components() {
	sc := s.doc.Scene()
	reg := sc.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	list := slices.Clone(reg.Components())
	slices.SortStableFunc(list, func(a, b *schema.TypeDescriptor) int { return s.d.Meta.Rank(a) - s.d.Meta.Rank(b) })
	for _, td := range list {
		if (hasUnknown && td.ID == unknown.ID) || td.FullName == scriptcomp.ScriptName || td.Hidden {
			continue
		}
		meta := s.d.Meta.Meta(td)
		e := entry{kind: kindComp, td: td}
		why := s.fit(sc, td, &e)
		s.entries = append(s.entries, e)
		s.items = append(s.items, palette.Item{Key: "comp:" + td.FullName, Label: meta.DisplayName, Category: meta.Category, Icon: meta.Icon, Detail: meta.Badge, Disabled: why})
	}
}

// fit finds the nodes td can go on and the reason none can.
func (s *Service) fit(sc *scene.Scene, td *schema.TypeDescriptor, e *entry) string {
	why := ""
	note := func(n ids.NodeID, r string) {
		if why == "" {
			why = r
		}
		e.skip = append(e.skip, sc.Name(sc.Handle(n))+" ("+r+")")
	}
	if td.SceneUnique && (len(s.nodes) > 1 || inScene(sc, td.ID)) {
		return "one per scene"
	}
	for _, n := range s.nodes {
		h := sc.Handle(n)
		switch {
		case h.IsZero():
			continue
		case has(sc, h, td.ID):
			note(n, "already on the node")
		case !sc.Accepts(h, td.ID):
			note(n, "needs a "+spaceName(td.Space)+" node")
		default:
			if other, bad := mutate.Conflict(sc, td, h); bad {
				note(n, displayName(td, s.d.Meta)+" and "+displayName(other, s.d.Meta)+" drive the same node")
				continue
			}
			if _, err := mutate.Required(sc, td, h); err != nil {
				note(n, err.Error())
				continue
			}
			e.nodes = append(e.nodes, n)
		}
	}
	if len(e.nodes) > 0 {
		return ""
	}
	if why == "" {
		why = "no node fits"
	}
	return why
}

func (s *Service) scripts() {
	sc := s.doc.Scene()
	for _, td := range sc.Registry().Scripts() {
		if td.Hidden {
			continue
		}
		e := entry{kind: kindScript, td: td}
		for _, n := range s.nodes {
			if c, ok := scriptcomp.ScriptsOf(sc, sc.Handle(n)); ok && c.Index(td.FullName) >= 0 {
				e.skip = append(e.skip, sc.Name(sc.Handle(n))+" (already on the node)")
				continue
			}
			e.nodes = append(e.nodes, n)
		}
		why := ""
		if len(e.nodes) == 0 {
			why = "already on the node"
		}
		full := td.FullName
		short, pkg := full, ""
		if i := strings.LastIndexByte(full, '.'); i >= 0 {
			short, pkg = full[i+1:], strings.ReplaceAll(full[:i], ".", "/")
		}
		s.entries = append(s.entries, e)
		s.items = append(s.items, palette.Item{Key: "script:" + full, Label: inspect.Humanize(short), Category: scriptsCat, Detail: pkg, Icon: "file.script", Disabled: why})
	}
}

func (s *Service) templates() {
	for _, sec := range templates.Layout(s.d.Templates, s.place) {
		cat := sec.Title
		if cat == "" {
			cat = "Node"
		}
		for _, t := range sec.Items {
			s.entries = append(s.entries, entry{kind: kindTemplate, tpl: t})
			s.items = append(s.items, palette.Item{Key: "tpl:" + t.ID, Label: t.Name, Category: cat, Icon: t.Icon})
		}
	}
	if s.d.Actions == nil || s.place.At2 != nil || s.place.At3 != nil {
		return
	}
	for _, a := range s.d.Actions() {
		s.entries = append(s.entries, entry{kind: kindAction, act: a})
		s.items = append(s.items, palette.Item{Key: "act:" + a.Key, Label: a.Name, Category: a.Category, Icon: a.Icon})
	}
}

// Draw shows the open palette and applies a pick.
func (s *Service) Draw() {
	if !s.pal.Active() {
		return
	}
	ui := s.d.Ctx.UI
	if s.d.Ctx.Doc() != s.doc {
		s.Close()
		return
	}
	res := s.pal.Draw(ui, s.rect(ui))
	if s.mode == Components {
		s.desc.keys(ui)
	}
	switch {
	case res.Close:
		s.Close()
	case res.Star >= 0:
		s.star(res.Star)
	case res.Pick >= 0:
		s.pick(res.Pick, res.Keep)
	}
}

func (s *Service) rect(ui *core.Context) mathx.Rect {
	sz := ui.Size()
	w := min(700, sz.X-40)
	h := min(440, sz.Y-60)
	r := mathx.Rect{X: (sz.X - w) / 2, Y: max(20, sz.Y*0.12), W: w, H: h}
	if s.at != nil {
		r.X = mathx.Clamp(s.at.X, 8, max(8, sz.X-w-8))
		r.Y = mathx.Clamp(s.at.Y, 8, max(8, sz.Y-h-8))
	}
	return r
}

func (s *Service) star(i int) {
	st := &s.store.modes[s.mode]
	k := s.items[i].Key
	if j := slices.Index(st.favs, k); j >= 0 {
		st.favs = slices.Delete(st.favs, j, j+1)
	} else {
		st.favs = append(st.favs, k)
	}
	s.items[i].Starred = !s.items[i].Starred
	s.save()
	s.pal.Refresh()
}

func (s *Service) used(i int) {
	st := &s.store.modes[s.mode]
	it := &s.items[i]
	if j := slices.Index(st.recent, it.Key); j >= 0 {
		st.recent = slices.Delete(st.recent, j, j+1)
	}
	st.recent = slices.Insert(st.recent, 0, it.Key)
	if len(st.recent) > maxRecent {
		st.recent = st.recent[:maxRecent]
	}
	if !slices.Contains(st.open, it.Category) {
		st.open = append(st.open, it.Category)
	}
	if s.store.uses == nil {
		s.store.uses = map[string]int{}
	}
	s.store.uses[it.Key]++
	s.save()
}

func (s *Service) pick(i int, keep bool) {
	if s.items[i].Disabled != "" {
		return
	}
	e := s.entries[i]
	ctx := s.d.Ctx
	doc := s.doc
	var run func() error
	label := s.items[i].Label
	switch e.kind {
	case kindTemplate:
		run = func() error {
			created, err := templates.InstantiatePlace(ctx, e.tpl, s.parent, -1, s.place)
			if err == nil && s.created != nil {
				s.created(created)
			}
			return err
		}
	case kindScript:
		run = func() error { return attach(doc, e) }
	case kindAction:
		parent := s.parent
		run = func() error { return e.act.Run(parent) }
	default:
		picks, extra := slices.Clone(s.desc.picks), s.desc.extras()
		run = func() error { return s.add(doc, e, picks, extra) }
	}
	s.used(i)
	done := func() {
		if err := run(); err != nil {
			ctx.Notify(context.LevelError, err.Error())
		}
		if keep && s.pal.Active() {
			q := s.pal.Query()
			s.collect()
			s.apply()
			s.pal.SetQuery(q)
		}
	}
	if !keep {
		s.Close()
	}
	if e.kind == kindTemplate || e.kind == kindAction || len(e.nodes) <= confirmOver {
		done()
		return
	}
	ctx.Ask(context.Question{
		Title:   "Add " + label,
		Message: "Add " + label + " changes " + strconv.Itoa(len(e.nodes)) + " nodes.",
		Buttons: []string{"Apply", "Cancel"},
		Then: func(choice int) {
			if choice == 0 {
				done()
			}
		},
	})
}

// add puts td, what it requires and the checked suggestions on every node it fits, as one History step.
func (s *Service) add(d *document.SceneDocument, e entry, picks []int, extra []*schema.TypeDescriptor) error {
	if d == nil {
		return nil
	}
	sc := d.Scene()
	env := commands.WithOnAdd(commands.EnvFor(sc), s.d.Ctx.OnAdd)
	tx := d.History.Begin("Add " + e.td.FullName)
	for _, n := range e.nodes {
		h := sc.Handle(n)
		req, err := mutate.RequiredPick(sc, e.td, h, picks)
		if err != nil {
			tx.Abort()
			return err
		}
		list := append(req, e.td)
		for _, x := range extra {
			if more, err := mutate.Required(sc, x, h); err == nil {
				list = append(list, more...)
				list = append(list, x)
			}
		}
		for _, t := range list {
			if has(sc, sc.Handle(n), t.ID) || !sc.Accepts(sc.Handle(n), t.ID) {
				continue
			}
			if err := tx.Exec(commands.NewMutation("Add "+t.FullName, env, commands.AddComp(sc, n, t, s.d.Meta.Defaults(t.ID)))); err != nil {
				tx.Abort()
				return err
			}
		}
	}
	tx.Commit()
	return nil
}

func attach(d *document.SceneDocument, e entry) error {
	if d == nil {
		return nil
	}
	tx := d.History.Begin("Add " + e.td.FullName)
	for _, n := range e.nodes {
		if err := tx.Exec(&commands.AddScript{Node: n, TypeName: e.td.FullName}); err != nil {
			tx.Abort()
			return err
		}
	}
	tx.Commit()
	return nil
}

func has(sc *scene.Scene, h scene.NodeHandle, t schema.TypeID) bool {
	_, ok := sc.GetByType(h, t)
	return ok
}

func inScene(sc *scene.Scene, t schema.TypeID) bool {
	for _, h := range sc.PreOrder() {
		if has(sc, h, t) && sc.IsAlive(h) {
			return true
		}
	}
	return false
}

func spaceName(k schema.SpaceKind) string {
	if k == schema.Space3D {
		return "3D"
	}
	return "2D"
}

func displayName(td *schema.TypeDescriptor, m *inspect.Registry) string {
	return m.Meta(td).DisplayName
}

// Rect is where the open palette draws, for the goldens.
func (s *Service) Rect() mathx.Rect {
	return s.rect(s.d.Ctx.UI)
}

// categoryIcons are the icons of the palette categories, the same set the documentation sidebar uses for module groups.
var categoryIcons = map[string]string{
	"Engine":    "cat.transform",
	"Node":      "cat.node",
	"Rendering": "cat.rendering",
	"2D":        "cat.rendering",
	"3D":        "cat.3d",
	"Camera":    "cat.camera",
	"Physics":   "comp.rigidbody",
	"Audio":     "cat.audio",
	"Animation": "anim.graph",
	"Gameplay":  "cat.game",
	"Game":      "cat.game",
	"Saving":    "comp.persist",
	"Spawning":  "file.prefab",
	"Prefabs":   "file.prefab",
	"Input":     "panel.input_devices",
	"Debug":     "panel.debug_draw",
	"Settings":  "cat.settings",
	"Data":      "file.data",
	scriptsCat:  "cat.scripts",
}

// CategoryIcon is the icon of a palette category, or "" when it has none.
func CategoryIcon(category string) string {
	return categoryIcons[category]
}
