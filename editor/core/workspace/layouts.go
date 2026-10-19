package workspace

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/editor/core/state"
	"aego/format/aetext"
	"aego/ui/dock"
)

const (
	Design      = "design"
	Programming = "programming"
	Art         = "art"
	Audio       = "audio"
	UserPrefix  = "user."
	userOrder   = 1000
	fileVersion = 1
)

type Preset struct {
	ID          string
	Title       string
	Order       int
	Slots       map[string]Slot
	HideToolbar bool
	Owner       string
	User        bool
	Base        string
}

type layout struct {
	tree    []byte
	toolbar bool
	set     bool
}

func (w *Workspace) warn(msg string, err error) {
	if w.log != nil {
		w.log.Warn(msg, log.F("err", err.Error()))
	}
}

func slotIn(p *Preset, d Desc) Slot {
	if p != nil && p.Slots != nil {
		if s, ok := p.Slots[d.Panel.ID()]; ok {
			return s
		}
	}
	return d.Slot
}

func (w *Workspace) RegisterPreset(p Preset) error {
	if p.ID == "" {
		return errs.New(errs.EditorPanel, "workspace needs an id")
	}
	if w.preset(p.ID) != nil {
		return errs.New(errs.EditorPanel, "duplicate workspace id", log.F("workspace", p.ID))
	}
	if p.Title == "" {
		p.Title = p.ID
	}
	p.Slots = maps.Clone(p.Slots)
	w.presets = append(w.presets, p)
	slices.SortStableFunc(w.presets, func(a, b Preset) int { return a.Order - b.Order })
	if w.cur == "" {
		w.cur = w.presets[0].ID
	}
	return nil
}

func (w *Workspace) RemovePreset(id string) {
	i := w.index(id)
	if i < 0 {
		return
	}
	w.presets = slices.Delete(w.presets, i, i+1)
	delete(w.layouts, id)
	if w.cur == id {
		w.cur = ""
		if len(w.presets) > 0 {
			w.cur = w.presets[0].ID
		}
		w.apply()
	}
	w.pending = true
}

func (w *Workspace) RemovePresets(owner string) {
	if owner == "" {
		return
	}
	for i := len(w.presets) - 1; i >= 0; i-- {
		if p := w.presets[i]; p.Owner == owner {
			w.RemovePreset(p.ID)
		}
	}
}

func (w *Workspace) index(id string) int {
	for i := range w.presets {
		if w.presets[i].ID == id {
			return i
		}
	}
	return -1
}

func (w *Workspace) preset(id string) *Preset {
	if i := w.index(id); i >= 0 {
		return &w.presets[i]
	}
	return nil
}

func (w *Workspace) Presets() []Preset {
	return w.presets
}

func (w *Workspace) Current() Preset {
	if p := w.preset(w.cur); p != nil {
		return *p
	}
	return Preset{ID: w.cur, Title: "Default"}
}

func (w *Workspace) layoutOf(id string) *layout {
	l := w.layouts[id]
	if l == nil {
		l = &layout{}
		w.layouts[id] = l
	}
	return l
}

func (w *Workspace) layoutFor(p *Preset) *dock.Tree {
	var cols [slotCount][]string
	for _, d := range w.sorted() {
		if s := slotIn(p, d); s != SlotNone {
			cols[s] = append(cols[s], d.Panel.ID())
		}
	}
	return dock.Columns(cols[SlotLeft], cols[SlotBrowser], cols[SlotCenter], cols[SlotBottom], cols[SlotRight])
}

func (w *Workspace) Toolbar() bool {
	if l := w.layouts[w.cur]; l != nil && l.set {
		return l.toolbar
	}
	if p := w.preset(w.cur); p != nil {
		return !p.HideToolbar
	}
	return true
}

func (w *Workspace) SetToolbar(on bool) {
	l := w.layoutOf(w.cur)
	l.toolbar, l.set = on, true
	w.pending = true
}

func (w *Workspace) Switch(id string) bool {
	if w.preset(id) == nil {
		return false
	}
	if id == w.cur && w.applied {
		return true
	}
	w.stash()
	w.cur = id
	w.apply()
	w.pending = true
	return true
}

func (w *Workspace) stash() {
	if w.applied {
		w.layoutOf(w.cur).tree = w.Tree.Save()
	}
}

func (w *Workspace) apply() {
	p := w.preset(w.cur)
	var t *dock.Tree
	if l := w.layouts[w.cur]; l != nil && l.tree != nil {
		lt, err := dock.Load(l.tree)
		if err != nil {
			w.warn("workspace layout is unreadable, using its default", err)
		} else {
			t = lt
		}
	}
	if t == nil {
		t = w.layoutFor(p)
	}
	w.Tree = t
	w.bind()
	w.retain()
	if len(w.Tree.Panels()) == 0 {
		w.Tree = w.layoutFor(p)
		w.bind()
		w.retain()
	}
	w.applied = true
}

func (w *Workspace) ToggleMaximize(id string) bool {
	if id == "" {
		id = w.focused
	}
	if id == "" || !w.Tree.Has(id) {
		return false
	}
	return w.Tree.ToggleMaximize(id)
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteByte('_')
		}
	}
	if out := strings.Trim(b.String(), "_"); out != "" {
		return out
	}
	return "workspace"
}

func (w *Workspace) users() int {
	n := 0
	for _, p := range w.presets {
		if p.User {
			n++
		}
	}
	return n
}

func (w *Workspace) SaveAs(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errs.New(errs.EditorPanel, "a workspace needs a name")
	}
	cur := w.Current()
	base := cur.ID
	if cur.User {
		base = cur.Base
	}
	id := UserPrefix + slug(title)
	for n := 2; w.preset(id) != nil; n++ {
		id = UserPrefix + slug(title) + "_" + strconv.Itoa(n)
	}
	toolbar := w.Toolbar()
	p := Preset{ID: id, Title: title, Order: userOrder + w.users(), Slots: cur.Slots, HideToolbar: !toolbar, User: true, Base: base}
	if err := w.RegisterPreset(p); err != nil {
		return "", err
	}
	w.stash()
	l := w.layoutOf(id)
	l.tree, l.toolbar, l.set = w.Tree.Save(), toolbar, true
	w.cur = id
	w.pending = true
	return id, nil
}

func (w *Workspace) Rename(id, title string) error {
	p := w.preset(id)
	if p == nil {
		return errs.New(errs.EditorPanel, "unknown workspace", log.F("workspace", id))
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errs.New(errs.EditorPanel, "a workspace needs a name")
	}
	p.Title = title
	w.pending = true
	return nil
}

func (w *Workspace) Delete(id string) error {
	p := w.preset(id)
	if p == nil || !p.User {
		return errs.New(errs.EditorPanel, "only workspaces you saved can be deleted", log.F("workspace", id))
	}
	if w.cur == id {
		w.Switch(p.Base)
	}
	w.RemovePreset(id)
	return nil
}

func (w *Workspace) encode() []byte {
	root := &aetext.Node{Head: "workspaces"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(fileVersion)})
	for _, p := range w.presets {
		n := &aetext.Node{Head: "workspace", Label: p.ID}
		n.Attrs = append(n.Attrs, aetext.Field{Name: "title", Value: aetext.String(p.Title)})
		if p.User {
			n.Attrs = append(n.Attrs,
				aetext.Field{Name: "user", Value: aetext.Bool(true)},
				aetext.Field{Name: "base", Value: aetext.String(p.Base)})
		}
		if l := w.layouts[p.ID]; l != nil {
			if l.set {
				n.Attrs = append(n.Attrs, aetext.Field{Name: "toolbar", Value: aetext.Bool(l.toolbar)})
			}
			if l.tree != nil {
				if d, err := aetext.Parse(l.tree, state.LayoutFile); err == nil {
					n.Blocks = append(n.Blocks, d)
				}
			}
		}
		root.Blocks = append(root.Blocks, n)
	}
	for _, f := range w.foreign {
		root.Blocks = append(root.Blocks, f.Clone())
	}
	return aetext.Write(root)
}

func (w *Workspace) decode(data []byte) error {
	doc, err := aetext.Parse(data, state.WorkspacesFile)
	if err != nil {
		return err
	}
	if doc.Head != "workspaces" {
		return errs.New(errs.UIBadLayout, "not a workspaces file", log.F("head", doc.Head))
	}
	if v, ok := doc.Attr("version"); ok && int(v.AsInt()) > fileVersion {
		return errs.New(errs.UIBadLayout, "workspaces were written by a newer editor", log.F("version", v.AsInt()))
	}
	w.foreign = w.foreign[:0]
	doc.EachBlock("workspace", func(n *aetext.Node) {
		if n.Label == "" {
			return
		}
		p := w.preset(n.Label)
		if p == nil {
			p = w.restoreUser(n)
		}
		if p == nil {
			w.foreign = append(w.foreign, n.Clone())
			return
		}
		if v, ok := n.Attr("title"); ok && v.Str != "" {
			p.Title = v.Str
		}
		l := w.layoutOf(p.ID)
		if v, ok := n.Attr("toolbar"); ok {
			l.toolbar, l.set = v.Bool, true
		}
		if b := n.Block("dock", ""); b != nil {
			l.tree = aetext.Write(b)
		}
	})
	return nil
}

func (w *Workspace) restoreUser(n *aetext.Node) *Preset {
	v, ok := n.Attr("user")
	if !ok || !v.Bool {
		return nil
	}
	p := Preset{ID: n.Label, Order: userOrder + w.users(), User: true}
	if b, ok := n.Attr("base"); ok {
		p.Base = b.Str
	}
	if base := w.preset(p.Base); base != nil {
		p.Slots = base.Slots
	}
	if w.RegisterPreset(p) != nil {
		return nil
	}
	return w.preset(p.ID)
}

func (w *Workspace) migrate(data []byte) {
	id := Design
	if w.preset(id) == nil {
		id = w.cur
	}
	w.layoutOf(id).tree = data
	if err := state.SaveWorkspaces(w.root, w.encode()); err != nil {
		w.warn("cannot save the imported layout", err)
		return
	}
	if err := state.RemoveLayout(w.root); err != nil {
		w.warn("cannot remove the old layout file", err)
	}
}
