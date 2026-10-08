package runtime

import (
	"strconv"
	"testing"

	"aego/engine"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/components"
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
)

const benchProgram = `package game

import "aego/input"

script Walker {
	@export var Speed float = 3

	func Update(dt float) {
		p := node.Position()
		p.X += input.Axis("horizontal")*Speed*dt + 0.01
		node.SetPosition(p)
	}
}

script Idle {
	@export var Note int = 1
}
`

type bench struct {
	rt  *Runtime
	ctx engine.Context
	s   *scene.Scene
}

func prepare(tb testing.TB, walkers, idle int) *bench {
	tb.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(types); err != nil {
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
	f, ds := parser.ParseSource(sm, "game/main.aes", []byte(benchProgram))
	a := checker.Check(sm, map[string][]*ast.File{"game": {f}}, reg.Surface(), checker.Options{Module: "game"})
	ds = append(ds, a.Diagnostics...)
	mod, cds := compiler.Compile(a, "game", compiler.Options{Sources: sm})
	if mod == nil || diag.HasErrors(append(ds, cds...)) {
		tb.Fatal("bench program does not compile")
	}
	rt := New(Options{Bindings: reg, Schema: types.Schema()})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		tb.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{InitialNodes: walkers + idle + 1})
	add := func(n int, typ string) {
		for i := 0; i < n; i++ {
			h := s.Create(s.Root(), typ+strconv.Itoa(i))
			sc := scene.Add[components.ScriptComponent](s, h)
			sc.Attachments = append(sc.Attachments, components.Attachment{TypeName: typ, Enabled: true})
		}
	}
	add(walkers, "game.Walker")
	add(idle, "game.Idle")
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		tb.Fatal(err)
	}
	w := engine.NewWorld("bench")
	w.Input = &input.State{}
	return &bench{rt: rt, s: s, ctx: engine.Context{World: w, Dt: 1.0 / 60}}
}

func (b *bench) frame() {
	b.ctx.Frame++
	b.rt.Update(&b.ctx)
}

func TestUpdateZeroAllocs2k(t *testing.T) {
	b := prepare(t, 2000, 0)
	for i := 0; i < 30; i++ {
		b.frame()
	}
	if allocs := testing.AllocsPerRun(50, b.frame); allocs > 0 {
		t.Fatalf("%.2f Go allocations per frame with 2k scripted nodes, want 0", allocs)
	}
	if n := len(b.rt.Faults()); n != 0 {
		t.Fatalf("%d faults: %v", n, b.rt.Faults()[0].Fault)
	}
}

func BenchmarkUpdate2k(bm *testing.B) {
	b := prepare(bm, 2000, 0)
	for i := 0; i < 30; i++ {
		b.frame()
	}
	bm.ReportAllocs()
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		b.frame()
	}
	bm.ReportMetric(float64(b.rt.Stats().Instructions), "instr/frame")
}

func BenchmarkSchedule10k(bm *testing.B) {
	b := prepare(bm, 2000, 8000)
	b.frame()
	b.frame()
	if st := b.rt.Stats(); st.Update != 2000 || st.Instances != 10000 {
		bm.Fatalf("update schedule %d of %d instances, want 2000 of 10000", st.Update, st.Instances)
	}
	bm.ReportAllocs()
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		b.frame()
	}
}
