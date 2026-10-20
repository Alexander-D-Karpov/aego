package input

import (
	"errors"
	"slices"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/format/aetext"
	raw "aego/platform/input"
	"aego/schema"
)

const (
	UserFile       = "user://input.aetext"
	KeySensitivity = ID + ".sensitivity"
	KeyInvertX     = ID + ".invertX"
	KeyInvertY     = ID + ".invertY"
	KeyDeadzone    = ID + ".deadzone"
	BindPrefix     = ID + ".bind."
)

type UserKeys struct {
	Store       *settings.Store
	Sensitivity settings.Key
	InvertX     settings.Key
	InvertY     settings.Key
	Deadzone    settings.Key
	Bind        map[string]settings.Key
}

type look struct {
	sens       float32
	invX, invY bool
}

type rebind struct {
	on       bool
	action   int
	slot     int
	conflict string
}

func bindingList(bs []Binding) schema.Value {
	out := schema.Value{Kind: schema.KindList, List: make([]schema.Value, len(bs))}
	for i := range bs {
		out.List[i] = schema.String(bs[i].String())
	}
	return out
}

func (m *Module) registerSettings(ctx *module.InitContext) error {
	var all []error
	add := func(d settings.Desc) settings.Key {
		k, err := ctx.Setting(d)
		if err != nil {
			all = append(all, err)
		}
		return k
	}
	both := settings.User | settings.Project
	cat := settings.CategoryInput
	m.keys.Sensitivity = add(settings.Desc{ID: KeySensitivity, Kind: schema.KindFloat64, Default: schema.Float64(1), Title: "Look Sensitivity",
		Category: cat, Order: 10, Flags: both, Min: 0.1, Max: 10, HasMin: true, HasMax: true, Step: 0.05})
	m.keys.InvertX = add(settings.Desc{ID: KeyInvertX, Kind: schema.KindBool, Title: "Invert Look X", Category: cat, Order: 20, Flags: both})
	m.keys.InvertY = add(settings.Desc{ID: KeyInvertY, Kind: schema.KindBool, Title: "Invert Look Y", Category: cat, Order: 21, Flags: both})
	m.keys.Deadzone = add(settings.Desc{ID: KeyDeadzone, Kind: schema.KindFloat64, Default: schema.Float64(float64(m.cfg.Deadzone)), Title: "Stick Dead Zone",
		Category: cat, Order: 30, Flags: both, Min: 0, Max: 0.95, HasMin: true, HasMax: true, Step: 0.01})
	m.keys.Bind = map[string]settings.Key{}
	for i := range m.cfg.Actions {
		a := &m.cfg.Actions[i]
		if a.Kind == Pointer || a.Kind == PointerDelta || !settings.ValidPath(a.Name) {
			continue
		}
		if k := add(settings.Desc{ID: BindPrefix + a.Name, Kind: schema.KindList, Default: bindingList(a.Bindings), Title: a.Name,
			Category: cat, Order: 100 + i, Flags: settings.User | settings.Controls}); k != 0 {
			m.keys.Bind[a.Name] = k
		}
	}
	return errors.Join(all...)
}

func (m *Module) migrateUserFile() {
	st := m.keys.Store
	if st == nil || m.files == nil {
		return
	}
	for _, k := range m.keys.Bind {
		if _, ok := st.UserValue(k); ok {
			return
		}
	}
	data, err := m.files.ReadFile(UserFile)
	if err != nil {
		return
	}
	doc, err := aetext.Parse(data, "input.aetext")
	if err != nil {
		return
	}
	lists := map[string][]schema.Value{}
	doc.EachBlock("bind", func(n *aetext.Node) {
		action, _ := n.Field("action")
		slot, _ := n.Field("slot")
		to, _ := n.Field("to")
		ai := slices.IndexFunc(m.cfg.Actions, func(a Action) bool { return a.Name == action.Str })
		if ai < 0 || to.Str == "" {
			return
		}
		l, ok := lists[action.Str]
		if !ok {
			l = bindingList(m.cfg.Actions[ai].Bindings).List
		}
		switch i := int(slot.AsInt()); {
		case i >= 0 && i < len(l):
			l[i] = schema.String(to.Str)
		case i == len(l):
			l = append(l, schema.String(to.Str))
		}
		lists[action.Str] = l
	})
	if len(lists) == 0 {
		return
	}
	tx := st.Begin()
	for name, l := range lists {
		if k, ok := m.keys.Bind[name]; ok {
			_ = tx.Set(k, schema.Value{Kind: schema.KindList, List: l})
		}
	}
	if tx.Apply() == nil {
		_ = st.Save()
	}
}

func (s *Service) settingChanged(k settings.Key) {
	if d := s.keys.Store.Desc(k); d != nil && d.Owner == ID {
		s.build()
	}
}

func (s *Service) Close() {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
}

func (s *Service) userBindings(a *Action) {
	st := s.keys.Store
	k, ok := s.keys.Bind[a.Name]
	if st == nil || !ok || st.Source(k) == settings.LayerDefault {
		return
	}
	var out []Binding
	for _, x := range st.Value(k).List {
		if b, err := ParseBinding(x.S); err == nil && b.fits(a.Kind) {
			out = append(out, b)
		}
	}
	a.Bindings = out
}

func (s *Service) BeginRebind(name string, slot int) bool {
	ai, ok := s.acts[name]
	if !ok || slot < 0 || slot > len(s.cfg.Actions[ai].Bindings) {
		return false
	}
	s.rebind = rebind{on: true, action: ai, slot: slot}
	return true
}

func (s *Service) Rebinding() bool {
	return s.rebind.on
}

func (s *Service) RebindConflict() string {
	return s.rebind.conflict
}

func (s *Service) CancelRebind() {
	s.rebind.on = false
}

func FirstPressed(in *raw.State, pads uint8) (Control, bool) {
	for k := raw.Scancode(1); k < raw.ScancodeCount; k++ {
		if k != raw.KeyEscape && in.KeyPressed(k) {
			return Control{DevKey, uint16(k)}, true
		}
	}
	for b := 0; b < int(raw.ButtonCount); b++ {
		if in.ButtonPressed(raw.Button(b)) {
			return Control{DevMouse, uint16(b)}, true
		}
	}
	for i := 0; i < raw.MaxPads; i++ {
		if pads&(1<<i) == 0 {
			continue
		}
		for b := raw.PadButton(0); b < raw.PadButtonCount; b++ {
			if in.PadPressed(i, b) {
				return Control{DevPadButton, uint16(b)}, true
			}
		}
		for a := raw.PadAxis(0); a < raw.PadAxisCount; a++ {
			if mathx.Abs(in.PadAxisValue(i, a)) > 0.6 {
				return Control{DevPadAxis, uint16(a)}, true
			}
		}
	}
	return Control{}, false
}

func (s *Service) conflictWith(ai int, c Control) string {
	for _, acts := range s.ctxActs {
		if !slices.Contains(acts, ai) {
			continue
		}
		for _, o := range acts {
			if o == ai {
				continue
			}
			for i := range s.cfg.Actions[o].Bindings {
				if s.cfg.Actions[o].Bindings[i].Uses(c) {
					return s.cfg.Actions[o].Name
				}
			}
		}
	}
	return ""
}

func (s *Service) pollRebind(in *raw.State) {
	rb := &s.rebind
	if in.KeyPressed(raw.KeyEscape) {
		rb.on = false
		return
	}
	c, ok := FirstPressed(in, s.devs[0]>>1)
	if !ok {
		return
	}
	rb.on = false
	a := &s.cfg.Actions[rb.action]
	b := Binding{Parts: [4]Control{c}}
	b.finish()
	if !b.fits(a.Kind) {
		rb.conflict = b.text + " cannot drive " + a.Name
		return
	}
	rb.conflict = s.conflictWith(rb.action, c)
	s.setBinding(rb.action, rb.slot, b)
}

func (s *Service) setBinding(ai, slot int, b Binding) {
	a := &s.cfg.Actions[ai]
	k, ok := s.keys.Bind[a.Name]
	if s.keys.Store == nil || !ok {
		if slot < len(a.Bindings) {
			a.Bindings[slot] = b
		} else {
			a.Bindings = append(a.Bindings, b)
		}
		return
	}
	list := bindingList(a.Bindings)
	if slot < len(list.List) {
		list.List[slot] = schema.String(b.String())
	} else {
		list.List = append(list.List, schema.String(b.String()))
	}
	tx := s.keys.Store.Begin()
	if tx.Set(k, list) == nil {
		_ = tx.Apply()
	}
}

func (s *Service) ResetBinding(name string) {
	k, ok := s.keys.Bind[name]
	if s.keys.Store == nil || !ok {
		s.build()
		return
	}
	tx := s.keys.Store.Begin()
	tx.Reset(k)
	_ = tx.Apply()
}

func (s *Service) ResetBindings() {
	if s.keys.Store == nil {
		s.build()
		return
	}
	tx := s.keys.Store.Begin()
	for _, k := range s.keys.Bind {
		tx.Reset(k)
	}
	_ = tx.Apply()
}

func (s *Service) SaveBindings() error {
	if s.keys.Store == nil {
		return nil
	}
	return s.keys.Store.Save()
}
