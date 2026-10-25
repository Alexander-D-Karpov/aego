package anim

import (
	"slices"
	"testing"

	"aego/core/curve"
)

const step = float32(1) / 16

func testGraph() *Graph {
	return &Graph{
		Params: []Param{{Name: "Speed", Kind: ParamFloat}, {Name: "Grounded", Kind: ParamBool, Default: 1}, {Name: "Attack", Kind: ParamTrigger}},
		States: []State{
			{Name: "Idle", Speed: 1, Length: 1},
			{Name: "Run", Speed: 1, Length: 0.75},
			{Name: "Hit", Speed: 1, Length: 0.5, Loop: LoopOnce, HasLoop: true},
		},
		Transitions: []Transition{
			{From: 0, To: 1, Duration: 0.125, Conds: []Cond{{Param: 0, Op: OpGt, Value: 0.1}}},
			{From: 1, To: 0, Duration: 0.125, Conds: []Cond{{Param: 0, Op: OpLe, Value: 0.1}}},
			{From: AnyState, To: 2, Interrupt: true, Conds: []Cond{{Param: 2, Op: OpSet}}},
			{From: 2, To: 0, Duration: 0.125, ExitTime: 1, HasExit: true},
		},
		Ease: curve.None,
	}
}

type rig struct {
	g   *Graph
	m   Machine
	p   []float64
	out []Change
}

func newRig(g *Graph) *rig {
	return &rig{g: g, p: g.Defaults(nil), out: make([]Change, 0, 32)}
}

func (r *rig) step() []Change {
	r.out = r.m.Step(r.g, step, r.p, r.out[:0])
	r.g.Consume(r.p)
	return r.out
}

func (r *rig) steps(n int) {
	for range n {
		r.step()
	}
}

func enter(s int32) Change { return Change{Kind: ChangeEnter, State: s} }
func exit(s int32) Change  { return Change{Kind: ChangeExit, State: s} }

func expectChanges(t *testing.T, name string, got []Change, want ...Change) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: got %v, want %v", name, got, want)
	}
}

func TestEntryConditionsAndBlend(t *testing.T) {
	r := newRig(testGraph())
	expectChanges(t, "entry", r.step(), enter(0))
	r.p[0] = 1
	expectChanges(t, "to run", r.step(), enter(1))
	if !r.m.Blend || r.m.Prev != 0 || r.m.State != 1 {
		t.Fatalf("blend %+v", r.m)
	}
	expectChanges(t, "half", r.step())
	if w := r.m.Weight(r.g); w != 0.5 {
		t.Fatalf("weight at half %v", w)
	}
	expectChanges(t, "end", r.step(), exit(0))
	if r.m.Blend {
		t.Fatal("the blend must end")
	}
	r.p[0] = 0
	expectChanges(t, "back", r.step(), enter(0))
}

func TestBlendWaitsForInterrupt(t *testing.T) {
	r := newRig(testGraph())
	r.step()
	r.p[0] = 1
	r.step()
	r.p[0] = 0
	expectChanges(t, "no interrupt", r.step())
	if r.m.State != 1 {
		t.Fatal("a transition without interrupt fired during a blend")
	}
	expectChanges(t, "after the blend", r.step(), exit(0), enter(0))
}

func TestTriggerFromAnyIsConsumed(t *testing.T) {
	r := newRig(testGraph())
	r.p[0] = 1
	r.steps(4)
	r.p[2] = 1
	expectChanges(t, "hit", r.step(), exit(1), enter(2))
	if r.p[2] != 0 {
		t.Fatal("the trigger must be consumed by the transition")
	}
	r.p[2] = 1
	expectChanges(t, "no hit into hit", r.step())
	if r.p[2] != 0 {
		t.Fatal("an unused trigger must be reset at the end of the tick")
	}
}

func TestExitTime(t *testing.T) {
	r := newRig(testGraph())
	r.step()
	r.p[2] = 1
	r.step()
	for i := range 7 {
		if got := r.step(); len(got) != 0 {
			t.Fatalf("left Hit after %d steps: %v", i+1, got)
		}
	}
	expectChanges(t, "exit time", r.step(), enter(0))
}

func TestInterruptFreezesThePose(t *testing.T) {
	g := testGraph()
	g.Transitions[2].Duration = 0.125
	r := newRig(g)
	r.step()
	r.p[0] = 1
	r.step()
	r.p[2] = 1
	expectChanges(t, "interrupt", r.step(), exit(0), Change{Kind: ChangeFreeze, State: 1}, exit(1), enter(2))
	if !r.m.Blend || r.m.Prev != Frozen || r.m.BlendT != 0 {
		t.Fatalf("machine %+v", r.m)
	}
	r.steps(2)
	if r.m.Blend {
		t.Fatal("the blend from the frozen pose must end")
	}
}

func TestInstantLoopIsBoundedAndReported(t *testing.T) {
	g := &Graph{
		States:      []State{{Name: "A", Speed: 1}, {Name: "B", Speed: 1}},
		Transitions: []Transition{{From: 0, To: 1}, {From: 1, To: 0}},
	}
	r := newRig(g)
	if got := r.step(); len(got) > 1+2*maxHops {
		t.Fatalf("too many changes %d", len(got))
	}
	ps := g.Check(nil)
	if !slices.ContainsFunc(ps, func(p GraphProblem) bool { return p.Kind == GraphLoop }) {
		t.Fatalf("problems %+v", ps)
	}
}

func TestGraphCheck(t *testing.T) {
	g := testGraph()
	g.Transitions = append(g.Transitions,
		Transition{From: 0, To: 0},
		Transition{From: 0, To: 1, Conds: []Cond{{Param: 1, Op: OpGt, Value: 0}}},
		Transition{From: 0, To: 9},
	)
	var kinds []GraphProblemKind
	for _, p := range g.Check(nil) {
		kinds = append(kinds, p.Kind)
	}
	for _, want := range []GraphProblemKind{GraphSelf, GraphParam, GraphState} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("missing %v in %v", want, kinds)
		}
	}
	if ps := testGraph().Check(nil); len(ps) != 0 {
		t.Fatalf("a good graph reports %+v", ps)
	}
	if ps := (&Graph{}).Check(nil); len(ps) != 1 || ps[0].Kind != GraphEmpty {
		t.Fatalf("empty graph %+v", ps)
	}
}

func TestPlayAndCrossFade(t *testing.T) {
	r := newRig(testGraph())
	r.out = r.m.Play(r.g, 2, r.out[:0])
	expectChanges(t, "play before start", r.out, enter(2))
	r.out = r.m.CrossFade(r.g, 1, 0.25, r.out[:0])
	expectChanges(t, "cross fade", r.out, enter(1))
	if !r.m.Blend || r.m.Prev != 2 || r.m.BlendDur != 0.25 {
		t.Fatalf("machine %+v", r.m)
	}
	r.out = r.m.Play(r.g, 0, r.out[:0])
	expectChanges(t, "play during a blend", r.out, exit(2), exit(1), enter(0))
}

func TestEasedWeight(t *testing.T) {
	g := testGraph()
	g.Ease = curve.InOutQuad
	r := newRig(g)
	r.step()
	r.out = r.m.CrossFade(g, 1, 0.5, r.out[:0])
	r.m.BlendT = 0.125
	if w := r.m.Weight(g); w != 0.125 {
		t.Fatalf("InOutQuad at a quarter %v", w)
	}
	if w := r.m.WeightAt(g, 0.25); w != 0.5 {
		t.Fatalf("InOutQuad at half %v", w)
	}
}

func TestStepAllocatesNothing(t *testing.T) {
	r := newRig(testGraph())
	r.steps(4)
	flip := 0.0
	if n := testing.AllocsPerRun(200, func() {
		flip = 1 - flip
		r.p[0] = flip
		r.step()
	}); n != 0 {
		t.Fatalf("Step allocates %v", n)
	}
}
