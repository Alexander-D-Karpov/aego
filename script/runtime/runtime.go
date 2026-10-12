package runtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aego/core/ids"
	"aego/core/log"
	"aego/engine"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/coroutine"
	"aego/script/signal"
	"aego/script/vm"
)

const (
	ServiceID             = "script.runtime"
	Channel   log.Channel = "script"
)

type State uint8

const (
	Running State = iota
	Faulted
	Disabled
	dead
)

type Options struct {
	Bindings *bind.Registry
	Schema   *schema.Registry
	Log      *log.Logger
	VM       vm.Options
	Profile  bool
}

type FaultReport struct {
	Type  string
	Node  ids.NodeID
	Path  string
	Fault *vm.Fault
}

type TypeStats struct {
	Name      string
	Instances int
	Calls     uint64
	Time      time.Duration
	Faults    int
}

type Stats struct {
	Instances    int
	Update       int
	Fixed        int
	Instructions uint64
	HostCalls    uint64
	HeapObjects  int
	HeapBytes    int
	GCRuns       uint64
	GCTime       time.Duration
	Faults       int
	BudgetAborts uint64
	Coroutines   int
	Sleeping     int
	Resumes      uint64
	CoCanceled   uint64
	Connections  int
	Emits        uint64
	Handlers     uint64
	Types        []TypeStats
}

type fieldInit struct {
	name string
	slot int
	kind abi.Kind
	comp string
}

type scriptType struct {
	name     string
	former   []string
	prefix   string
	def      *bytecode.ScriptDef
	slots    int
	heap     []uint16
	init     []vm.Value
	fields   []fieldInit
	methods  []*vm.Func
	ready    *vm.Func
	fixed    *vm.Func
	update   *vm.Func
	reloaded *vm.Func
	vals     []vm.Value
	free     []int
	live     int
	calls    uint64
	time     time.Duration
	faults   int
}

type instance struct {
	typ   *scriptType
	base  int
	node  scene.NodeHandle
	state State
}

type Runtime struct {
	opt          Options
	log          *log.ChannelLogger
	m            *vm.Machine
	co           *coroutine.Scheduler
	sig          *signal.Registry
	deliverFn    signal.Deliver
	ctx          bind.Ctx
	types        []*scriptType
	byName       map[string]*scriptType
	comps        []compInfo
	inst         []instance
	instFree     []vm.InstanceID
	byNode       map[scene.NodeHandle][]vm.InstanceID
	update       []vm.InstanceID
	fixed        []vm.InstanceID
	ready        []vm.InstanceID
	readyRun     []vm.InstanceID
	spawn        []scene.NodeHandle
	dirty        bool
	scene        *scene.Scene
	unsub        func()
	scriptType   schema.TypeID
	faults       []FaultReport
	warned       map[string]bool
	arg          [1]vm.Value
	stats        Stats
	frame        uint64
	instrMark    uint64
	hostMark     uint64
	budgetAborts uint64
	emits        uint64
	handlers     uint64
	resumeMark   uint64
	elapsed      float64
	gen          uint64
	dmu          sync.Mutex
	deferred     []func()
}

func New(opt Options) *Runtime {
	lg := opt.Log
	if lg == nil {
		lg = log.Nop()
	}
	return &Runtime{
		opt:    opt,
		log:    lg.With(Channel),
		byName: map[string]*scriptType{},
		byNode: map[scene.NodeHandle][]vm.InstanceID{},
		warned: map[string]bool{},
	}
}

func funcAt(lm *vm.Module, idx int32) *vm.Func {
	if idx < 0 || int(idx) >= len(lm.Funcs) {
		return nil
	}
	return lm.Funcs[idx]
}

func newType(m *vm.Machine, lm *vm.Module, si int, def *bytecode.ScriptDef) *scriptType {
	t := &scriptType{
		name:     def.FullName,
		former:   def.Former,
		prefix:   shortName(def.FullName) + ".",
		def:      def,
		slots:    int(def.Slots),
		heap:     def.HeapSlots,
		init:     make([]vm.Value, def.Slots),
		ready:    funcAt(lm, def.Ready),
		fixed:    funcAt(lm, def.FixedUpdate),
		update:   funcAt(lm, def.Update),
		reloaded: funcAt(lm, def.Reloaded),
	}
	for _, f := range lm.Funcs {
		if f.Script == si {
			t.methods = append(t.methods, f)
		}
	}
	for _, f := range def.Fields {
		if f.HasDefault {
			if f.Default.Kind == bytecode.KString {
				t.init[f.Slot] = m.Intern(f.Default.Str)
			} else {
				t.init[f.Slot] = vm.Value{A: f.Default.A, B: f.Default.B}
			}
		}
		if f.Export {
			tr, _ := abi.Parse(f.Type)
			t.fields = append(t.fields, fieldInit{name: f.Name, slot: int(f.Slot), kind: tr.Kind, comp: tr.Name})
		}
	}
	return t
}

func load(m *vm.Machine, b *bind.Registry, mods []*bytecode.Module) ([]*vm.Module, error) {
	loaded := make([]*vm.Module, 0, len(mods))
	for _, bm := range mods {
		lm, err := m.Load(bm, b.Resolve)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, lm)
	}
	if err := vm.Link(loaded); err != nil {
		return nil, err
	}
	return loaded, nil
}

func buildTypes(m *vm.Machine, loaded []*vm.Module, mods []*bytecode.Module) ([]*scriptType, map[string]*scriptType) {
	var types []*scriptType
	byName := map[string]*scriptType{}
	for i, lm := range loaded {
		src := mods[i]
		for si := range src.Scripts {
			t := newType(m, lm, si, &src.Scripts[si])
			types = append(types, t)
			byName[t.name] = t
		}
	}
	for _, t := range types {
		for _, f := range t.former {
			if _, taken := byName[f]; !taken {
				byName[f] = t
			}
		}
	}
	return types, byName
}

func (r *Runtime) Link(mods []*bytecode.Module) error {
	if r.opt.Bindings == nil {
		return errors.New("script runtime needs a binding registry")
	}
	r.Reset()
	m := vm.New(r.opt.VM)
	m.Env = &r.ctx
	m.Comps = r
	m.Scripts = bridge{r}
	m.Signals = bridge{r}
	loaded, err := load(m, r.opt.Bindings, mods)
	if err != nil {
		return err
	}
	if r.opt.Schema != nil {
		if err := Publish(r.opt.Schema, mods, r.opt.Bindings.Surface()); err != nil {
			return err
		}
	}
	r.m = m
	r.co = coroutine.New(m)
	r.co.Self = r.selfOf
	r.co.OnFault = r.fault
	r.sig = signal.New()
	r.deliverFn = r.deliver
	r.types, r.byName = buildTypes(m, loaded, mods)
	r.comps = linkComponents(r.opt.Bindings, r.opt.Schema)
	r.scriptType = scriptcomp.ScriptTypeID
	r.stats.Types = make([]TypeStats, len(r.types))
	r.gen = 0
	return nil
}

func (r *Runtime) Linked() []string {
	out := make([]string, len(r.types))
	for i, t := range r.types {
		out[i] = t.name
	}
	return out
}

func (r *Runtime) Reset() {
	if r.unsub != nil {
		r.unsub()
		r.unsub = nil
	}
	if r.co != nil {
		r.co.CancelAll()
	}
	if r.sig != nil {
		r.sig.Reset()
	}
	r.scene, r.ctx.Scene = nil, nil
	r.inst = r.inst[:0]
	r.instFree = r.instFree[:0]
	r.update, r.fixed, r.ready, r.spawn = r.update[:0], r.fixed[:0], r.ready[:0], r.spawn[:0]
	clear(r.byNode)
	for _, t := range r.types {
		t.vals, t.free, t.live = t.vals[:0], t.free[:0], 0
	}
	r.faults = r.faults[:0]
	clear(r.warned)
	r.dirty = false
	r.elapsed = 0
}

func (r *Runtime) Instantiate(s *scene.Scene) error {
	if r.m == nil {
		return errors.New("script runtime is not linked")
	}
	if r.scene != nil {
		r.Reset()
	}
	r.scene, r.ctx.Scene = s, s
	r.unsub = s.Events().Subscribe(r.onEvent)
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h != root {
			r.spawnNode(h)
		}
	}
	return nil
}

func (r *Runtime) onEvent(ev scene.Event) {
	switch ev.Kind {
	case scene.EvNodeDestroyed:
		r.killNode(ev.Node)
	case scene.EvComponentRemoved:
		if ev.Type == r.scriptType {
			r.killNode(ev.Node)
		}
	case scene.EvComponentAdded:
		if ev.Type == r.scriptType {
			r.spawn = append(r.spawn, ev.Node)
		}
	}
}

func (r *Runtime) OnNodeDestroyed(h scene.NodeHandle) {
	r.killNode(h)
}

func (r *Runtime) OnAttachmentAdded(h scene.NodeHandle) {
	r.spawn = append(r.spawn, h)
}

func (r *Runtime) warnOnce(key, msg string, fields ...log.Field) {
	if r.warned[key] {
		return
	}
	r.warned[key] = true
	r.log.Warn(msg, fields...)
}

func (r *Runtime) spawnNode(h scene.NodeHandle) {
	if !r.scene.IsAlive(h) {
		return
	}
	sc, ok := scriptcomp.ScriptsOf(r.scene, h)
	if !ok {
		return
	}
	have := map[*scriptType]bool{}
	for _, id := range r.byNode[h] {
		if in := &r.inst[id-1]; in.state != dead {
			have[in.typ] = true
		}
	}
	list := r.byNode[h]
	for i := range sc.Attachments {
		a := &sc.Attachments[i]
		if !a.Enabled {
			continue
		}
		t := r.byName[a.TypeName]
		if t == nil {
			r.warnOnce("type:"+a.TypeName, "script type is not compiled, the attachment does nothing",
				log.F("script", a.TypeName), log.F("node", r.scene.Path(h)))
			continue
		}
		if have[t] {
			continue
		}
		have[t] = true
		list = append(list, r.create(t, h, &a.Props))
	}
	r.byNode[h] = list
}

func (r *Runtime) retire(id vm.InstanceID) {
	in := &r.inst[id-1]
	if in.state == dead {
		return
	}
	in.state = dead
	in.typ.live--
	r.dirty = true
	if r.co != nil {
		r.co.CancelOwner(id)
	}
	if r.sig != nil {
		r.sig.DropInstance(id)
	}
}

func (r *Runtime) OnAttachmentRemoved(h scene.NodeHandle, typeName string) {
	t := r.byName[typeName]
	if t == nil {
		return
	}
	for _, id := range r.byNode[h] {
		if r.inst[id-1].typ == t {
			r.retire(id)
		}
	}
}

func (r *Runtime) create(t *scriptType, h scene.NodeHandle, props *schema.PropertyBag) vm.InstanceID {
	var base int
	if n := len(t.free); n > 0 {
		base = t.free[n-1]
		t.free = t.free[:n-1]
		copy(t.vals[base:base+t.slots], t.init)
	} else {
		base = len(t.vals)
		t.vals = append(t.vals, t.init...)
	}
	self := t.vals[base : base+t.slots]
	self[0] = bind.NodeValue(h)
	for _, f := range t.fields {
		if v, ok := props.Get(f.name); ok {
			self[f.slot] = r.convert(t, f, v, h)
		}
	}
	var id vm.InstanceID
	if n := len(r.instFree); n > 0 {
		id = r.instFree[n-1]
		r.instFree = r.instFree[:n-1]
		r.inst[id-1] = instance{typ: t, base: base, node: h, state: Running}
	} else {
		id = vm.InstanceID(len(r.inst) + 1)
		r.inst = append(r.inst, instance{typ: t, base: base, node: h, state: Running})
	}
	t.live++
	if t.update != nil {
		r.update = append(r.update, id)
	}
	if t.fixed != nil {
		r.fixed = append(r.fixed, id)
	}
	if t.ready != nil {
		r.ready = append(r.ready, id)
	}
	return id
}

func (r *Runtime) convert(t *scriptType, f fieldInit, v schema.Value, owner scene.NodeHandle) vm.Value {
	switch f.kind {
	case abi.Bool:
		return vm.Bool(v.I != 0)
	case abi.Int, abi.Enum:
		return vm.Int(v.AsInt())
	case abi.Float:
		return vm.Float(float32(v.AsFloat()))
	case abi.String:
		return r.m.NewString(v.S)
	case abi.Vec2:
		return bind.Vec2Value(v.V)
	case abi.Vec3:
		return bind.Vec3Value(v.V3)
	case abi.Color:
		return bind.ColorValue(v.C)
	case abi.Quat:
		return bind.QuatValue(v.Q)
	case abi.Rect:
		return bind.RectValue(v.R)
	case abi.AssetRef:
		return bind.AssetValue(v.Asset)
	case abi.NodeRef, abi.ComponentRef:
		if v.Node.IsZero() {
			return vm.Value{}
		}
		h := r.scene.Handle(v.Node)
		if h.IsZero() || !r.scene.IsAlive(h) {
			r.warnOnce("ref:"+v.Node.String(), "node reference does not resolve, the field starts as nil",
				log.F("script", t.name), log.F("field", f.name), log.F("node", r.scene.Path(owner)), log.F("target", v.Node.String()))
			return vm.Value{}
		}
		if f.kind == abi.NodeRef {
			return bind.NodeValue(h)
		}
		idx, ok := r.componentRef(h, f.comp)
		if !ok {
			r.warnOnce("comp:"+v.Node.String()+":"+f.comp, "referenced node has no "+f.comp+", the field starts as nil",
				log.F("script", t.name), log.F("field", f.name), log.F("node", r.scene.Path(owner)), log.F("target", r.scene.Path(h)))
			return vm.Value{}
		}
		return vm.Value{A: bind.NodeValue(h).A, B: uint64(idx)}
	}
	return vm.Value{}
}

func (r *Runtime) componentRef(h scene.NodeHandle, name string) (int, bool) {
	td, ok := r.scene.Registry().ByName(name)
	if !ok {
		return 0, false
	}
	if _, has := r.scene.GetByType(h, td.ID); !has {
		return 0, false
	}
	for i := range r.comps {
		if r.comps[i].typ == td.ID {
			return i, true
		}
	}
	return 0, false
}

func (r *Runtime) killNode(h scene.NodeHandle) {
	list, ok := r.byNode[h]
	if !ok {
		return
	}
	for _, id := range list {
		r.retire(id)
	}
	delete(r.byNode, h)
	r.dirty = true
}

func keep(list []vm.InstanceID, inst []instance) []vm.InstanceID {
	n := 0
	for _, id := range list {
		if inst[id-1].state != dead {
			list[n] = id
			n++
		}
	}
	return list[:n]
}

func (r *Runtime) compact() {
	r.update = keep(r.update, r.inst)
	r.fixed = keep(r.fixed, r.inst)
	r.ready = keep(r.ready, r.inst)
	for i := range r.inst {
		in := &r.inst[i]
		if in.state != dead || in.base < 0 {
			continue
		}
		t := in.typ
		clear(t.vals[in.base : in.base+t.slots])
		t.free = append(t.free, in.base)
		in.base = -1
		r.instFree = append(r.instFree, vm.InstanceID(i+1))
	}
	r.dirty = false
}

func (r *Runtime) flush() {
	for round := 0; round < 8; round++ {
		for i := 0; i < len(r.spawn); i++ {
			r.spawnNode(r.spawn[i])
		}
		r.spawn = r.spawn[:0]
		if r.dirty {
			r.compact()
		}
		if len(r.ready) == 0 {
			return
		}
		r.readyRun, r.ready = r.ready, r.readyRun[:0]
		for _, id := range r.readyRun {
			in := &r.inst[id-1]
			if in.state == Running {
				r.call(id, in.typ.ready, nil)
			}
		}
		r.readyRun = r.readyRun[:0]
	}
}

func (r *Runtime) call(id vm.InstanceID, f *vm.Func, args []vm.Value) {
	in := &r.inst[id-1]
	t := in.typ
	r.ctx.Node, r.ctx.Self = in.node, id
	var start time.Time
	if r.opt.Profile {
		start = time.Now()
	}
	_, fl := r.m.CallAs(f, t.vals[in.base:in.base+t.slots], args, id)
	if r.opt.Profile {
		t.time += time.Since(start)
	}
	t.calls++
	if fl != nil {
		r.fault(id, fl)
	}
}

func (r *Runtime) fault(id vm.InstanceID, fl *vm.Fault) {
	if id == vm.NoInstance || int(id) > len(r.inst) {
		r.log.Error("script stopped: "+fl.Message, log.F("code", fl.Code))
		return
	}
	in := &r.inst[id-1]
	if in.state == Running {
		in.state = Faulted
	}
	t := in.typ
	t.faults++
	if fl.Code == vm.CodeBudget {
		r.budgetAborts++
	}
	rep := FaultReport{Type: t.name, Fault: fl}
	if r.scene != nil && r.scene.IsAlive(in.node) {
		rep.Node, rep.Path = r.scene.ID(in.node), r.scene.Path(in.node)
	}
	r.faults = append(r.faults, rep)
	var b strings.Builder
	for _, f := range fl.Stack {
		fmt.Fprintf(&b, "\n  at %s (%s:%d)", f.Function, f.File, f.Line)
	}
	fields := []log.Field{log.F("script", t.name), log.F("node", rep.Path), log.F("code", fl.Code)}
	if len(fl.Stack) > 0 {
		fields = append(fields, log.F("path", fl.Stack[0].File), log.F("line", fl.Stack[0].Line))
	}
	fields = append(fields, log.F("stack", b.String()))
	if fl.Detail != "" {
		fields = append(fields, log.F("go", fl.Detail))
	}
	r.log.Error("script stopped: "+fl.Message, fields...)
	if r.co != nil {
		r.co.CancelOwner(id)
	}
	if r.sig != nil {
		r.sig.DropInstance(id)
	}
}

func inputOf(ec *engine.Context) *input.State {
	if ec.World != nil && ec.World.Input != nil {
		return ec.World.Input
	}
	if ec.Engine != nil {
		return ec.Engine.Input()
	}
	return nil
}

func (r *Runtime) begin(ec *engine.Context) bool {
	if r.m == nil || r.scene == nil {
		return false
	}
	if ec.Frame != r.frame {
		r.rollFrame()
		r.frame = ec.Frame
	}
	r.ctx.World = ec.World
	r.ctx.Input = inputOf(ec)
	r.ctx.Random = ec.Random
	r.ctx.Frame = ec.Frame
	r.ctx.Log = r.log
	r.ctx.Elapsed = r.elapsed
	r.flush()
	return true
}

func (r *Runtime) end() {
	r.m.MaybeCollect(r)
}

func (r *Runtime) FixedUpdate(ec *engine.Context) {
	if !r.begin(ec) {
		return
	}
	dt := float32(ec.Dt)
	r.ctx.Dt, r.ctx.Fixed = dt, dt
	r.arg[0] = vm.Float(dt)
	for i := 0; i < len(r.fixed); i++ {
		id := r.fixed[i]
		if in := &r.inst[id-1]; in.state == Running {
			r.call(id, in.typ.fixed, r.arg[:1])
		}
	}
	r.co.Fixed()
	r.end()
}

func (r *Runtime) Update(ec *engine.Context) {
	if !r.begin(ec) {
		return
	}
	dt := float32(ec.Dt)
	r.elapsed += ec.Dt
	r.ctx.Dt, r.ctx.Elapsed = dt, r.elapsed
	r.arg[0] = vm.Float(dt)
	for i := 0; i < len(r.update); i++ {
		id := r.update[i]
		if in := &r.inst[id-1]; in.state == Running {
			r.call(id, in.typ.update, r.arg[:1])
		}
	}
	r.end()
}

func (r *Runtime) MarkRoots(mk *vm.Marker) {
	for i := range r.inst {
		in := &r.inst[i]
		if in.state == dead || in.base < 0 {
			continue
		}
		t := in.typ
		mk.MarkSlots(t.vals[in.base:in.base+t.slots], t.heap)
	}
}

func (r *Runtime) rollFrame() {
	vs := r.m.Stats()
	cs := r.co.Stats()
	s := &r.stats
	s.Instructions, r.instrMark = vs.Instructions-r.instrMark, vs.Instructions
	s.HostCalls, r.hostMark = vs.HostCalls-r.hostMark, vs.HostCalls
	s.HeapObjects, s.HeapBytes = vs.HeapObjects, vs.HeapBytes
	s.GCRuns, s.GCTime = vs.GCRuns, vs.GCTime
	s.Faults, s.BudgetAborts = len(r.faults), r.budgetAborts
	s.Update, s.Fixed = len(r.update), len(r.fixed)
	s.Coroutines = cs.Live
	s.Sleeping = cs.Sleeping + cs.Fixed + cs.Awaiting
	s.Resumes, r.resumeMark = cs.Resumed-r.resumeMark, cs.Resumed
	s.CoCanceled = cs.Canceled
	s.Connections = r.sig.Len()
	s.Emits, r.emits = r.emits, 0
	s.Handlers, r.handlers = r.handlers, 0
	s.Instances = 0
	for i, t := range r.types {
		s.Instances += t.live
		if i < len(s.Types) {
			s.Types[i] = TypeStats{Name: t.name, Instances: t.live, Calls: t.calls, Time: t.time, Faults: t.faults}
		}
		t.calls, t.time = 0, 0
	}
}

func (r *Runtime) Stats() Stats {
	return r.stats
}

func (r *Runtime) Faults() []FaultReport {
	return r.faults
}

func (r *Runtime) Close() {
	r.Reset()
	r.dmu.Lock()
	r.deferred = nil
	r.dmu.Unlock()
	r.m, r.co, r.sig = nil, nil, nil
}
