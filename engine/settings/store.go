package settings

import (
	"slices"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/schema"
)

const diagOwner = "engine"

type Key uint32

type entry struct {
	d                               Desc
	project, user, session, eff     schema.Value
	hasProject, hasUser, hasSession bool
	src                             Layer
}

func (e *entry) value() (schema.Value, Layer) {
	switch {
	case e.hasSession:
		return e.session, LayerSession
	case e.hasUser:
		return e.user, LayerUser
	case e.hasProject:
		return e.project, LayerProject
	}
	return e.d.Default, LayerDefault
}

func (e *entry) recompute() bool {
	v, src := e.value()
	changed := !Equal(v, e.eff)
	e.eff, e.src = v, src
	return changed
}

type owner struct {
	version   int
	migrate   func(*aetext.Node, int, int) error
	userExtra []aetext.Field
	projExtra []aetext.Field
	newer     *aetext.Node
}

type sub struct {
	id  uint64
	key Key
	fn  func(Key)
}

type saved struct {
	k                      Key
	project, user, session schema.Value
	hp, hu, hs             bool
}

type pending struct {
	deadline float64
	snap     []saved
}

type Store struct {
	list           []entry
	byID           map[string]Key
	owners         map[string]*owner
	order          []string
	frozen         bool
	started        bool
	subs           []sub
	nextSub        uint64
	preset         Key
	names          func(section, list string) []string
	persist        func([]byte) error
	afterLoad      []func()
	now            float64
	confirm        *pending
	restart        bool
	dirty          bool
	readOnly       bool
	diags          []problems.Diagnostic
	rev            uint64
	unknownUser    []*aetext.Node
	unknownProject []*aetext.Node
}

func New() *Store {
	return &Store{byID: map[string]Key{}, owners: map[string]*owner{}}
}

func bad(id, msg string) error {
	return errs.New(errs.EngineModule, "setting "+id+" "+msg, log.F("setting", id))
}

func (s *Store) ownerOf(name string) *owner {
	o := s.owners[name]
	if o == nil {
		o = &owner{version: 1}
		s.owners[name] = o
		s.order = append(s.order, name)
	}
	return o
}

func (s *Store) OwnerVersion(v OwnerVersion) error {
	switch {
	case s.frozen:
		return bad(v.Owner, "version is set after the settings were frozen")
	case v.Version < 1:
		return bad(v.Owner, "version must be positive")
	}
	o := s.ownerOf(v.Owner)
	o.version, o.migrate = v.Version, v.Migrate
	return nil
}

func (s *Store) Register(d Desc) (Key, error) {
	var zero schema.Kind
	switch {
	case s.frozen:
		return 0, bad(d.ID, "is registered after the settings were frozen")
	case d.Owner == "" || !strings.HasPrefix(d.ID, d.Owner+".") || !ValidPath(d.ID):
		return 0, bad(d.ID, "must be named "+d.Owner+".<name> with letters, digits and dots")
	case !supported(d.Kind):
		return 0, bad(d.ID, "has the unsupported kind "+d.Kind.String())
	}
	if _, dup := s.byID[d.ID]; dup {
		return 0, bad(d.ID, "is registered twice")
	}
	for i := range d.Choices {
		c := &d.Choices[i]
		c.Value, _ = Normalize(d.Kind, c.Value)
		if c.Label == "" {
			c.Label = c.Name
		}
	}
	if d.Default.Kind == zero {
		d.Default = zeroOf(&d)
	}
	def, ok := Normalize(d.Kind, d.Default)
	if !ok {
		return 0, bad(d.ID, "has a default of kind "+d.Default.Kind.String())
	}
	d.Default = def
	if d.Preset {
		if s.preset != 0 {
			return 0, bad(d.ID, "is a second preset")
		}
		if d.Kind != schema.KindEnum || !slices.ContainsFunc(d.Choices, func(c Choice) bool { return c.Name == PresetCustom }) {
			return 0, bad(d.ID, "is a preset and needs enum choices with "+PresetCustom)
		}
	}
	s.ownerOf(d.Owner)
	k := Key(len(s.list) + 1)
	s.list = append(s.list, entry{d: d, eff: def})
	s.byID[d.ID] = k
	if d.Preset {
		s.preset = k
	}
	return k, nil
}

func (s *Store) Freeze() {
	s.frozen = true
}

func (s *Store) Frozen() bool {
	return s.frozen
}

func (s *Store) SetNames(fn func(section, list string) []string) {
	s.names = fn
}

func (s *Store) SetPersist(fn func([]byte) error) {
	s.persist = fn
}

func (s *Store) AfterLoad(fn func()) {
	if s.started {
		fn()
		return
	}
	s.afterLoad = append(s.afterLoad, fn)
}

func (s *Store) Start() {
	if s.started {
		return
	}
	s.started = true
	for i := range s.list {
		e := &s.list[i]
		if e.d.Apply == nil {
			continue
		}
		if err := e.d.Apply(e.eff); err != nil {
			s.report("CFG004", problems.Warning, e.d.ID+" could not be applied: "+err.Error()+", the project value is used", e.d.ID)
			e.hasUser = false
			e.recompute()
			_ = e.d.Apply(e.eff)
		}
	}
	for _, fn := range s.afterLoad {
		fn()
	}
	s.afterLoad = nil
}

func (s *Store) at(k Key) *entry {
	if k == 0 || int(k) > len(s.list) {
		return nil
	}
	return &s.list[k-1]
}

func (s *Store) Key(id string) Key {
	return s.byID[id]
}

func (s *Store) Len() int {
	return len(s.list)
}

func (s *Store) Desc(k Key) *Desc {
	if e := s.at(k); e != nil {
		return &e.d
	}
	return nil
}

func (s *Store) Each(fn func(Key, *Desc)) {
	for i := range s.list {
		fn(Key(i+1), &s.list[i].d)
	}
}

func (s *Store) Value(k Key) schema.Value {
	if e := s.at(k); e != nil {
		return e.eff
	}
	return schema.Value{}
}

func (s *Store) Bool(k Key) bool {
	e := s.at(k)
	return e != nil && e.eff.I != 0
}

func (s *Store) Int(k Key) int64 {
	if e := s.at(k); e != nil {
		return e.eff.I
	}
	return 0
}

func (s *Store) Float(k Key) float64 {
	if e := s.at(k); e != nil {
		return e.eff.F
	}
	return 0
}

func (s *Store) Text(k Key) string {
	if e := s.at(k); e != nil {
		return e.eff.S
	}
	return ""
}

func (s *Store) Source(k Key) Layer {
	if e := s.at(k); e != nil {
		return e.src
	}
	return LayerDefault
}

func (s *Store) Base(k Key) schema.Value {
	e := s.at(k)
	switch {
	case e == nil:
		return schema.Value{}
	case e.hasProject:
		return e.project
	}
	return e.d.Default
}

func (s *Store) UserValue(k Key) (schema.Value, bool) {
	if e := s.at(k); e != nil && e.hasUser {
		return e.user, true
	}
	return schema.Value{}, false
}

func (s *Store) ProjectValue(k Key) (schema.Value, bool) {
	if e := s.at(k); e != nil && e.hasProject {
		return e.project, true
	}
	return schema.Value{}, false
}

func (s *Store) Enabled(k Key) bool {
	e := s.at(k)
	return e != nil && (e.d.EnabledWhen == nil || e.d.EnabledWhen(s))
}

func (s *Store) Choices(k Key) []Choice {
	if e := s.at(k); e != nil {
		return s.choices(e)
	}
	return nil
}

func (s *Store) choices(e *entry) []Choice {
	d := &e.d
	switch {
	case len(d.Choices) > 0:
		return d.Choices
	case d.ChoicesWhen != nil:
		return d.ChoicesWhen(s)
	case d.ChoicesFrom.Section != "" && s.names != nil:
		names := s.names(d.ChoicesFrom.Section, d.ChoicesFrom.List)
		out := make([]Choice, len(names))
		for i, n := range names {
			v := schema.String(n)
			if d.Kind == schema.KindInt64 {
				v = schema.Int64(int64(i))
			}
			out[i] = Choice{Name: n, Label: n, Value: v}
		}
		return out
	}
	return nil
}

func (s *Store) choiceOf(e *entry, v schema.Value) (Choice, bool) {
	for _, c := range s.choices(e) {
		if Equal(c.Value, v) {
			return c, true
		}
	}
	return Choice{}, false
}

func (s *Store) ChoiceName(k Key) string {
	e := s.at(k)
	if e == nil {
		return ""
	}
	if c, ok := s.choiceOf(e, e.eff); ok {
		return c.Name
	}
	return e.eff.S
}

func (s *Store) text(e *entry, v schema.Value) string {
	if c, ok := s.choiceOf(e, v); ok {
		if c.Label != "" {
			return c.Label
		}
		return c.Name
	}
	return plain(v)
}

func (s *Store) Format(k Key) string {
	if e := s.at(k); e != nil {
		return s.text(e, e.eff)
	}
	return ""
}

func (s *Store) FormatValue(k Key, v schema.Value) string {
	if e := s.at(k); e != nil {
		return s.text(e, v)
	}
	return ""
}

func (s *Store) check(e *entry, v schema.Value) (schema.Value, error) {
	d := &e.d
	n, ok := Normalize(d.Kind, v)
	if !ok {
		return v, bad(d.ID, "expects "+d.Kind.String()+", got "+v.Kind.String())
	}
	if d.Kind == schema.KindInt64 || d.Kind == schema.KindFloat64 {
		f := numOf(n)
		if (d.HasMin && f < d.Min) || (d.HasMax && f > d.Max) {
			return v, bad(d.ID, "is out of range: "+strconv.FormatFloat(f, 'g', -1, 64))
		}
	}
	if d.Kind != schema.KindList {
		if cs := s.choices(e); len(cs) > 0 && !slices.ContainsFunc(cs, func(c Choice) bool { return Equal(c.Value, n) }) {
			return v, bad(d.ID, "has no choice "+plain(n))
		}
	}
	if d.Validate != nil {
		if err := d.Validate(n); err != nil {
			return v, bad(d.ID, err.Error())
		}
	}
	if d.Supports != nil && !d.Supports(n) {
		return v, bad(d.ID, "value "+s.text(e, n)+" is not supported here")
	}
	return clone(n), nil
}

func (s *Store) Check(k Key, v schema.Value) (schema.Value, error) {
	e := s.at(k)
	if e == nil {
		return v, bad("#"+strconv.Itoa(int(k)), "does not exist")
	}
	return s.check(e, v)
}

func (s *Store) snapshot(keys []Key) []saved {
	out := make([]saved, len(keys))
	for i, k := range keys {
		e := s.at(k)
		out[i] = saved{k: k, project: e.project, user: e.user, session: e.session, hp: e.hasProject, hu: e.hasUser, hs: e.hasSession}
	}
	return out
}

func (s *Store) restore(snap []saved) {
	for _, x := range snap {
		e := s.at(x.k)
		e.project, e.user, e.session = x.project, x.user, x.session
		e.hasProject, e.hasUser, e.hasSession = x.hp, x.hu, x.hs
	}
}

func (s *Store) recompute(keys []Key) []Key {
	var changed []Key
	for _, k := range keys {
		if s.at(k).recompute() {
			changed = append(changed, k)
		}
	}
	return changed
}

func (s *Store) markDirty(snap []saved) {
	for _, x := range snap {
		e := s.at(x.k)
		if e.d.Flags.Has(Transient) {
			continue
		}
		if e.hasUser != x.hu || (e.hasUser && !Equal(e.user, x.user)) {
			s.dirty = true
		}
	}
}

func (s *Store) commit(keys []Key, mutate func()) error {
	snap := s.snapshot(keys)
	mutate()
	changed := s.recompute(keys)
	if !s.started {
		s.markDirty(snap)
		return nil
	}
	for i, k := range changed {
		e := s.at(k)
		if e.d.Apply == nil {
			continue
		}
		if err := e.d.Apply(e.eff); err != nil {
			s.restore(snap)
			s.recompute(keys)
			for _, b := range changed[:i+1] {
				if be := s.at(b); be.d.Apply != nil {
					_ = be.d.Apply(be.eff)
				}
			}
			s.report("CFG003", problems.Error, e.d.ID+" could not be applied: "+err.Error()+"; the previous settings are back", e.d.ID)
			return bad(e.d.ID, "could not be applied: "+err.Error())
		}
	}
	for _, k := range changed {
		if s.at(k).d.Flags.Has(Restart) {
			s.restart = true
		}
	}
	s.markDirty(snap)
	s.notify(changed)
	return nil
}

func (s *Store) Subscribe(k Key, fn func(Key)) func() {
	s.nextSub++
	id := s.nextSub
	s.subs = append(s.subs, sub{id: id, key: k, fn: fn})
	return func() {
		s.subs = slices.DeleteFunc(s.subs, func(x sub) bool { return x.id == id })
	}
}

func (s *Store) notify(keys []Key) {
	for _, k := range keys {
		for i := 0; i < len(s.subs); i++ {
			if sb := s.subs[i]; sb.key == 0 || sb.key == k {
				sb.fn(k)
			}
		}
	}
}

func (s *Store) Override(k Key, v schema.Value) error {
	e := s.at(k)
	if e == nil {
		return bad("#"+strconv.Itoa(int(k)), "does not exist")
	}
	n, err := s.check(e, v)
	if err != nil {
		return err
	}
	return s.commit([]Key{k}, func() { e.session, e.hasSession = n, true })
}

func (s *Store) ClearOverride(k Key) error {
	e := s.at(k)
	if e == nil || !e.hasSession {
		return nil
	}
	return s.commit([]Key{k}, func() { e.session, e.hasSession = schema.Value{}, false })
}

func (s *Store) ClearOverrides() error {
	var keys []Key
	for i := range s.list {
		if s.list[i].hasSession {
			keys = append(keys, Key(i+1))
		}
	}
	return s.commit(keys, func() {
		for _, k := range keys {
			e := s.at(k)
			e.session, e.hasSession = schema.Value{}, false
		}
	})
}

func (s *Store) SetProject(vals map[Key]schema.Value) error {
	clean := make(map[Key]schema.Value, len(vals))
	for k, v := range vals {
		e := s.at(k)
		if e == nil || !e.d.Flags.Has(Project) {
			return bad("#"+strconv.Itoa(int(k)), "has no project value")
		}
		n, err := s.check(e, v)
		if err != nil {
			return err
		}
		clean[k] = n
	}
	var keys []Key
	for i := range s.list {
		k := Key(i + 1)
		if _, ok := clean[k]; ok || s.list[i].hasProject {
			keys = append(keys, k)
		}
	}
	return s.commit(keys, func() {
		for _, k := range keys {
			e := s.at(k)
			v, ok := clean[k]
			e.project, e.hasProject = v, ok
		}
	})
}

func (s *Store) Tick(now float64) {
	s.now = now
	if s.confirm != nil && now >= s.confirm.deadline {
		_ = s.RevertPending()
	}
}

func (s *Store) Pending() (float64, bool) {
	if s.confirm == nil {
		return 0, false
	}
	return max(0, s.confirm.deadline-s.now), true
}

func (s *Store) Confirm() {
	s.confirm = nil
}

func (s *Store) RevertPending() error {
	p := s.confirm
	if p == nil {
		return nil
	}
	s.confirm = nil
	keys := make([]Key, len(p.snap))
	for i, x := range p.snap {
		keys[i] = x.k
	}
	return s.commit(keys, func() { s.restore(p.snap) })
}

func (s *Store) NeedsRestart() bool {
	return s.restart
}

func (s *Store) Dirty() bool {
	return s.dirty
}

func (s *Store) ReadOnly() bool {
	return s.readOnly
}

func (s *Store) Save() error {
	if s.persist == nil || s.readOnly {
		return nil
	}
	if err := s.persist(s.EncodeProfile()); err != nil {
		s.report("CFG005", problems.Error, "the settings profile could not be written: "+err.Error(), "")
		return err
	}
	s.dirty = false
	return nil
}

func (s *Store) report(code string, sev problems.Severity, msg, setting string) {
	s.diags = append(s.diags, problems.Diagnostic{
		Owner: diagOwner, Code: code, Severity: sev, Message: msg,
		Subject: problems.Subject{Kind: problems.SubjectSetting, Setting: setting},
	})
	s.rev++
}

func (s *Store) Diagnostics() []problems.Diagnostic {
	return s.diags
}

func (s *Store) Problems() problems.Source {
	return problems.Source{
		Revision: func() uint64 { return s.rev },
		Append:   func(dst []problems.Diagnostic) []problems.Diagnostic { return append(dst, s.diags...) },
	}
}
