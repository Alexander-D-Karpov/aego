package runtime

import (
	"strconv"
	"testing"

	"aego/engine"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/ast"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
	"aego/script/stdlib"
	"aego/script/vm"
)

func compileGame(tb testing.TB, src string) (*scene.Types, *bind.Registry, *bytecode.Module) {
	tb.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scriptcomp.Register(types); err != nil {
		tb.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		tb.Fatal(err)
	}
	reg := bind.New()
	stdlib.Install(reg, stdlib.Options{Seed: 1})
	BindComponents(reg, types.Schema())
	if err := reg.Freeze(); err != nil {
		tb.Fatal(err)
	}
	sm := source.NewManager()
	f, ds := parser.ParseSource(sm, "game/main.aes", []byte(src))
	a := checker.Check(sm, map[string][]*ast.File{"game": {f}}, reg.Surface(), checker.Options{Module: "game"})
	ds = append(ds, a.Diagnostics...)
	mod, cds := compiler.Compile(a, "game", compiler.Options{Sources: sm})
	ds = append(ds, cds...)
	if mod == nil || diag.HasErrors(ds) {
		for _, d := range ds {
			tb.Log(diag.Format(sm, d))
		}
		tb.Fatal("program does not compile")
	}
	return types, reg, mod
}

type rig struct {
	rt  *Runtime
	s   *scene.Scene
	ctx engine.Context
}

func newRig(tb testing.TB, src string, nodes int) *rig {
	tb.Helper()
	types, reg, mod := compileGame(tb, src)
	rt := New(Options{Bindings: reg, Schema: types.Schema(), VM: vm.Options{Budget: 100_000}})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		tb.Fatal(err)
	}
	w := engine.NewWorld("test")
	w.Input = &input.State{}
	return &rig{rt: rt, s: scene.New(types, nil, scene.Options{InitialNodes: nodes + 1}), ctx: engine.Context{World: w, Dt: 1.0 / 60}}
}

func (g *rig) run(tb testing.TB) {
	tb.Helper()
	g.s.Flush()
	if err := g.rt.Instantiate(g.s); err != nil {
		tb.Fatal(err)
	}
}

func (g *rig) frame() {
	g.ctx.Frame++
	g.rt.Update(&g.ctx)
	g.rt.Coroutines(&g.ctx)
	g.s.Flush()
}

func (g *rig) field(tb testing.TB, h scene.NodeHandle, typ, name string) vm.Value {
	tb.Helper()
	id, ok := g.rt.InstanceOf(h, typ)
	if !ok {
		tb.Fatalf("%s has no %s", g.s.Name(h), typ)
	}
	v, ok := g.rt.Field(id, name)
	if !ok {
		tb.Fatalf("field %s is missing", name)
	}
	return v
}

func refTo(s *scene.Scene, h scene.NodeHandle, name string) schema.PropertyBag {
	var b schema.PropertyBag
	b.Set(name, schema.Node(s.ID(h)))
	return b
}

const signalProgram = `package game

script Source {
	signal Ping(n int)

	var frames int
	var Count int

	func Update(dt float) {
		frames++
		if frames == 2 || frames == 3 {
			Ping.Emit(frames)
		}
	}
}

script Listener {
	@export var Src NodeRef

	var mine int
	var late int
	var hooked bool

	func Ready() {
		s := Src.GetScript<Source>()
		if s != nil {
			s.Ping.Connect(OnPing)
		}
	}

	func OnPing(n int) {
		s := Src.GetScript<Source>()
		s.Count++
		mine = s.Count
		if !hooked {
			hooked = true
			s.Ping.Connect(OnLate)
		}
	}

	func OnLate(n int) {
		late++
	}
}

script Loop {
	signal Echo()

	var started bool

	func Ready() {
		Echo.Connect(OnEcho)
	}

	func Update(dt float) {
		if !started {
			started = true
			Echo.Emit()
		}
	}

	func OnEcho() {
		Echo.Emit()
	}
}
`

func listeners(g *rig, src scene.NodeHandle, n int) []scene.NodeHandle {
	var out []scene.NodeHandle
	for i := 0; i < n; i++ {
		out = append(out, attach(g.s, "L"+strconv.Itoa(i), "game.Listener", refTo(g.s, src, "Src")))
	}
	return out
}

func TestSignalOrderTargetDeathLateConnect(t *testing.T) {
	g := newRig(t, signalProgram, 4)
	src := attach(g.s, "Source", "game.Source", schema.PropertyBag{})
	ls := listeners(g, src, 3)
	g.run(t)
	g.frame()
	g.s.Destroy(ls[1])
	g.frame()
	if a, b := g.field(t, ls[0], "game.Listener", "mine").Int(), g.field(t, ls[2], "game.Listener", "mine").Int(); a != 1 || b != 2 {
		t.Fatalf("delivery order %d %d", a, b)
	}
	for _, h := range []scene.NodeHandle{ls[0], ls[2]} {
		if l := g.field(t, h, "game.Listener", "late").Int(); l != 0 {
			t.Fatal("a connection made during Emit was delivered by the same Emit")
		}
	}
	g.frame()
	for _, h := range []scene.NodeHandle{ls[0], ls[2]} {
		if l := g.field(t, h, "game.Listener", "late").Int(); l != 1 {
			t.Fatalf("late handler ran %d times", l)
		}
	}
	if len(g.rt.Faults()) != 0 {
		t.Fatalf("faults %v", g.rt.Faults()[0].Fault)
	}
}

func TestSignalSourceDeathDropsConnections(t *testing.T) {
	g := newRig(t, signalProgram, 3)
	src := attach(g.s, "Source", "game.Source", schema.PropertyBag{})
	listeners(g, src, 2)
	g.run(t)
	g.frame()
	if n := g.rt.sig.Len(); n != 2 {
		t.Fatalf("connections %d", n)
	}
	g.s.Destroy(src)
	g.frame()
	if n := g.rt.sig.Len(); n != 0 {
		t.Fatalf("source death left %d connections", n)
	}
}

func TestNestedEmitIsCallDepth(t *testing.T) {
	g := newRig(t, signalProgram, 1)
	attach(g.s, "Loop", "game.Loop", schema.PropertyBag{})
	g.run(t)
	g.frame()
	g.frame()
	fs := g.rt.Faults()
	if len(fs) == 0 || fs[0].Fault.Code != vm.CodeCallDepth {
		t.Fatalf("faults %+v", fs)
	}
}

const asyncProgram = `package game

script Spin {
	var n int

	func Ready() {
		start Loop()
	}

	func Loop() coroutine {
		for {
			n++
			yield
		}
	}
}

script Waiter {
	var childDone bool
	var done bool

	func Ready() {
		start Main()
	}

	func Main() coroutine {
		c := start Child()
		yield c
		done = true
	}

	func Child() coroutine {
		yield
		yield
		childDone = true
	}
}

script Timer {
	var hit bool

	func Ready() {
		start Run()
	}

	func Run() coroutine {
		yield wait(1.0)
		hit = true
	}
}

script Prey {
	var Hp int = 3
}

script Hunter {
	@export var Target NodeRef

	var ref ScriptRef<Prey>

	func Ready() {
		ref = Target.GetScript<Prey>()
	}

	func Update(dt float) {
		ref.Hp -= 1
	}
}

script Hub {
	signal Tick(n int)

	var got int

	func Ready() {
		for i := 0; i < 10; i++ {
			Tick.Connect(OnTick)
		}
	}

	func Update(dt float) {
		Tick.Emit(1)
	}

	func OnTick(n int) {
		got += n
	}
}
`

func TestCoroutineCancelledWithNode(t *testing.T) {
	g := newRig(t, asyncProgram, 1)
	h := attach(g.s, "Spin", "game.Spin", schema.PropertyBag{})
	g.run(t)
	g.frame()
	g.frame()
	if g.rt.CoStats().Live != 1 || g.field(t, h, "game.Spin", "n").Int() < 2 {
		t.Fatalf("stats %+v", g.rt.CoStats())
	}
	g.s.Destroy(h)
	g.frame()
	if st := g.rt.CoStats(); st.Live != 0 || st.Canceled != 1 {
		t.Fatalf("destroyed node left coroutines %+v", st)
	}
}

func TestAwaitWakesInSameFrame(t *testing.T) {
	g := newRig(t, asyncProgram, 1)
	h := attach(g.s, "Waiter", "game.Waiter", schema.PropertyBag{})
	g.run(t)
	for i := 0; i < 6; i++ {
		g.frame()
		child := g.field(t, h, "game.Waiter", "childDone").Bool()
		done := g.field(t, h, "game.Waiter", "done").Bool()
		if child != done {
			t.Fatalf("frame %d: child %v waiter %v", i+1, child, done)
		}
	}
	if !g.field(t, h, "game.Waiter", "done").Bool() {
		t.Fatal("waiter never woke")
	}
}

func TestWaitUsesElapsed(t *testing.T) {
	g := newRig(t, asyncProgram, 1)
	g.ctx.Dt = 0.25
	h := attach(g.s, "Timer", "game.Timer", schema.PropertyBag{})
	g.run(t)
	for i := 0; i < 3; i++ {
		g.frame()
	}
	if g.field(t, h, "game.Timer", "hit").Bool() {
		t.Fatal("wait(1.0) resumed after 0.75 s")
	}
	for i := 0; i < 3; i++ {
		g.frame()
	}
	if !g.field(t, h, "game.Timer", "hit").Bool() {
		t.Fatal("wait(1.0) did not resume after 1.5 s")
	}
}

func TestScriptRefToDeadInstanceFaults(t *testing.T) {
	g := newRig(t, asyncProgram, 2)
	prey := attach(g.s, "Prey", "game.Prey", schema.PropertyBag{})
	attach(g.s, "Hunter", "game.Hunter", refTo(g.s, prey, "Target"))
	g.run(t)
	g.frame()
	if hp := g.field(t, prey, "game.Prey", "Hp").Int(); hp != 2 {
		t.Fatalf("hp %d", hp)
	}
	g.s.Destroy(prey)
	g.frame()
	fs := g.rt.Faults()
	if len(fs) != 1 || fs[0].Fault.Code != vm.CodeNil || fs[0].Type != "game.Hunter" {
		t.Fatalf("faults %+v", fs)
	}
}

func TestSignalsZeroAllocs(t *testing.T) {
	g := newRig(t, asyncProgram, 1000)
	for i := 0; i < 1000; i++ {
		attach(g.s, "Hub"+strconv.Itoa(i), "game.Hub", schema.PropertyBag{})
	}
	g.run(t)
	for i := 0; i < 30; i++ {
		g.frame()
	}
	st := g.rt.Stats()
	if st.Connections != 10_000 || st.Emits != 1000 || st.Handlers != 10_000 {
		t.Fatalf("stats connections %d emits %d handlers %d", st.Connections, st.Emits, st.Handlers)
	}
	if allocs := testing.AllocsPerRun(50, g.frame); allocs > 0 {
		t.Fatalf("%.2f Go allocations per frame with 10k connections and 1k emits", allocs)
	}
}

func TestCoroutinesZeroAllocs(t *testing.T) {
	g := newRig(t, asyncProgram, 2000)
	for i := 0; i < 2000; i++ {
		attach(g.s, "Spin"+strconv.Itoa(i), "game.Spin", schema.PropertyBag{})
	}
	g.run(t)
	for i := 0; i < 30; i++ {
		g.frame()
	}
	if allocs := testing.AllocsPerRun(50, g.frame); allocs > 0 {
		t.Fatalf("%.2f Go allocations per frame with 2k coroutines", allocs)
	}
}

func BenchmarkCoroutines2k(bm *testing.B) {
	g := newRig(bm, asyncProgram, 2000)
	for i := 0; i < 2000; i++ {
		attach(g.s, "Spin"+strconv.Itoa(i), "game.Spin", schema.PropertyBag{})
	}
	g.run(bm)
	for i := 0; i < 30; i++ {
		g.frame()
	}
	bm.ReportAllocs()
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		g.frame()
	}
	bm.ReportMetric(float64(g.rt.Stats().Resumes), "resumes/frame")
}
