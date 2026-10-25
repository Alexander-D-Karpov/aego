package command

import (
	"runtime"
	"slices"
	"sort"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/platform/input"
)

type Mods uint8

const (
	ModPrimary Mods = 1 << iota
	ModShift
	ModAlt
)

type Shortcut struct {
	Key  input.Scancode
	Mods Mods
}

func Key(k input.Scancode, m Mods) Shortcut {
	return Shortcut{Key: k, Mods: m}
}

func (s Shortcut) IsZero() bool {
	return s.Key == input.KeyUnknown
}

func (s Shortcut) Format(goos string) string {
	if s.IsZero() {
		return ""
	}
	var b strings.Builder
	mac := goos == "darwin"
	if s.Mods&ModPrimary != 0 {
		if mac {
			b.WriteString("⌘")
		} else {
			b.WriteString("Ctrl+")
		}
	}
	if s.Mods&ModShift != 0 {
		if mac {
			b.WriteString("⇧")
		} else {
			b.WriteString("Shift+")
		}
	}
	if s.Mods&ModAlt != 0 {
		if mac {
			b.WriteString("⌥")
		} else {
			b.WriteString("Alt+")
		}
	}
	b.WriteString(s.Key.String())
	return b.String()
}

func (s Shortcut) String() string {
	return s.Format(runtime.GOOS)
}

func FromInput(m input.Mods) Mods {
	var out Mods
	if m.Primary() {
		out |= ModPrimary
	}
	if m.Has(input.ModShift) {
		out |= ModShift
	}
	if m.Has(input.ModAlt) {
		out |= ModAlt
	}
	return out
}

func ParseShortcut(s string) (Shortcut, error) {
	parts := strings.Split(s, "+")
	var sc Shortcut
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i < len(parts)-1 {
			switch strings.ToLower(p) {
			case "primary", "ctrl", "cmd", "command":
				sc.Mods |= ModPrimary
			case "shift":
				sc.Mods |= ModShift
			case "alt", "option":
				sc.Mods |= ModAlt
			default:
				return Shortcut{}, errs.New(errs.EditorCommand, "unknown modifier", log.F("modifier", p))
			}
			continue
		}
		for k := input.Scancode(1); k < input.ScancodeCount; k++ {
			if strings.EqualFold(k.String(), p) {
				sc.Key = k
				break
			}
		}
		if sc.Key == input.KeyUnknown {
			return Shortcut{}, errs.New(errs.EditorCommand, "unknown key", log.F("key", p))
		}
	}
	return sc, nil
}

func isModifier(k input.Scancode) bool {
	return k >= input.KeyLCtrl && k <= input.KeyRGUI
}

type Desc struct {
	ID       string
	Name     string
	Category string
	Scope    string
	Shortcut Shortcut
	Alt      []Shortcut
	Hint     string
	Icon     string
	Tip      string
	Order    int
	Label    func() string
	Checked  func() bool
	Enabled  func() bool
	Run      func() error
}

type Registry struct {
	OnError func(id string, err error)
	byID    map[string]*Desc
	order   []string
	cats    []string
	catsOK  bool
}

func New() *Registry {
	return &Registry{byID: map[string]*Desc{}}
}

func (r *Registry) Register(d Desc) error {
	if d.ID == "" || d.Run == nil {
		return errs.New(errs.EditorCommand, "command needs an id and a run function", log.F("command", d.ID))
	}
	if _, dup := r.byID[d.ID]; dup {
		return errs.New(errs.EditorCommand, "duplicate command id", log.F("command", d.ID))
	}
	if err := r.checkConflict(d.ID, d.Keys(), d.Scope); err != nil {
		return err
	}
	if d.Name == "" {
		d.Name = d.ID
	}
	cp := d
	cp.Alt = slices.Clone(d.Alt)
	r.byID[d.ID] = &cp
	r.order = append(r.order, d.ID)
	r.catsOK = false
	return nil
}

func (r *Registry) checkConflict(id string, keys []Shortcut, scope string) error {
	for _, sc := range keys {
		for _, other := range r.byID {
			if other.ID != id && other.Scope == scope && other.Matches(sc) {
				return errs.New(errs.EditorCommand, "shortcut is already bound",
					log.F("command", id), log.F("other", other.ID), log.F("shortcut", sc.String()))
			}
		}
	}
	return nil
}

func (r *Registry) Rebind(id string, sc Shortcut) error {
	d, ok := r.byID[id]
	if !ok {
		return errs.New(errs.EditorCommand, "unknown command", log.F("command", id))
	}
	if !sc.IsZero() {
		if err := r.checkConflict(id, []Shortcut{sc}, d.Scope); err != nil {
			return err
		}
	}
	d.Shortcut = sc
	return nil
}

func (r *Registry) Shortcut(id string) string {
	if d, ok := r.byID[id]; ok {
		return d.Shortcut.String()
	}
	return ""
}

func (r *Registry) Category(cat string) []Desc {
	var out []Desc
	for _, id := range r.order {
		if d := r.byID[id]; d.Category == cat {
			out = append(out, *d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func (r *Registry) HasCategory(cat string) bool {
	for _, id := range r.order {
		if r.byID[id].Category == cat {
			return true
		}
	}
	return false
}

// Categories lists the categories in registration order; the list is built again only after commands change.
func (r *Registry) Categories() []string {
	if r.catsOK {
		return r.cats
	}
	seen := map[string]bool{}
	r.cats = r.cats[:0]
	for _, id := range r.order {
		c := r.byID[id].Category
		if c != "" && !seen[c] {
			seen[c] = true
			r.cats = append(r.cats, c)
		}
	}
	r.catsOK = true
	return r.cats
}

func (r *Registry) Unregister(id string) {
	if _, ok := r.byID[id]; !ok {
		return
	}
	delete(r.byID, id)
	r.catsOK = false
	for i, o := range r.order {
		if o == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			return
		}
	}
}

func (r *Registry) Get(id string) (Desc, bool) {
	d, ok := r.byID[id]
	if !ok {
		return Desc{}, false
	}
	return *d, true
}

func (r *Registry) Enabled(id string) bool {
	d, ok := r.byID[id]
	return ok && (d.Enabled == nil || d.Enabled())
}

func (r *Registry) Run(id string) error {
	d, ok := r.byID[id]
	if !ok {
		return errs.New(errs.EditorCommand, "unknown command", log.F("command", id))
	}
	if d.Enabled != nil && !d.Enabled() {
		return errs.New(errs.EditorCommand, "command is disabled", log.F("command", id))
	}
	err := d.Run()
	if err != nil && r.OnError != nil {
		r.OnError(id, err)
	}
	return err
}

func (r *Registry) Lookup(sc Shortcut, scope string) (Desc, bool) {
	var global *Desc
	for _, id := range r.order {
		d := r.byID[id]
		if !d.Matches(sc) {
			continue
		}
		if scope != "" && d.Scope == scope {
			return *d, true
		}
		if d.Scope == "" && global == nil {
			global = d
		}
	}
	if global != nil {
		return *global, true
	}
	return Desc{}, false
}

func (r *Registry) resolve(sc Shortcut, scope string) (*Desc, bool) {
	var global *Desc
	for _, id := range r.order {
		d := r.byID[id]
		if !d.Matches(sc) || (d.Enabled != nil && !d.Enabled()) {
			continue
		}
		if scope != "" && d.Scope == scope {
			return d, true
		}
		if d.Scope == "" && global == nil {
			global = d
		}
	}
	return global, global != nil
}

func (r *Registry) Dispatch(in *input.State, scope string) (string, bool) {
	if in == nil {
		return "", false
	}
	mods := FromInput(in.Mods())
	hit := ""
	in.EachDown(func(k input.Scancode) {
		if hit != "" || isModifier(k) || !in.KeyRepeated(k) {
			return
		}
		if d, ok := r.resolve(Shortcut{Key: k, Mods: mods}, scope); ok {
			hit = d.ID
		}
	})
	if hit == "" {
		return "", false
	}
	r.Run(hit)
	return hit, true
}

func (r *Registry) Pressed(id string, in *input.State) bool {
	d, ok := r.Get(id)
	if !ok || in == nil {
		return false
	}
	mods := FromInput(in.Mods())
	for _, s := range d.Keys() {
		if in.KeyPressed(s.Key) && s.Mods == mods {
			return true
		}
	}
	return false
}

func (r *Registry) All() []Desc {
	out := make([]Desc, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, *r.byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) Search(q string) []Desc {
	q = strings.ToLower(strings.TrimSpace(q))
	all := r.All()
	if q == "" {
		return all
	}
	out := all[:0]
	for _, d := range all {
		if strings.Contains(strings.ToLower(d.Name), q) || strings.Contains(strings.ToLower(d.ID), q) {
			out = append(out, d)
		}
	}
	return out
}

func (d *Desc) Keys() []Shortcut {
	out := make([]Shortcut, 0, 1+len(d.Alt))
	if !d.Shortcut.IsZero() {
		out = append(out, d.Shortcut)
	}
	for _, s := range d.Alt {
		if !s.IsZero() {
			out = append(out, s)
		}
	}
	return out
}

func (d *Desc) Matches(sc Shortcut) bool {
	if sc.IsZero() {
		return false
	}
	return d.Shortcut == sc || slices.Contains(d.Alt, sc)
}

func (d *Desc) KeysText() string {
	var parts []string
	for _, k := range d.Keys() {
		parts = append(parts, k.String())
	}
	if d.Hint != "" {
		parts = append(parts, d.Hint)
	}
	return strings.Join(parts, " · ")
}

func (r *Registry) Describe(id, icon, tip string) {
	d, ok := r.byID[id]
	if !ok {
		return
	}
	if icon != "" {
		d.Icon = icon
	}
	if tip != "" {
		d.Tip = tip
	}
}
