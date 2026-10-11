package coroutine_test

import (
	"testing"

	"aego/script/abi"
	bc "aego/script/bytecode"
	"aego/script/coroutine"
	"aego/script/vm"
)

func coFn(name string, consts []bc.Const, code ...bc.Instr) bc.Function {
	code = append(code, bc.ABx(bc.GETSF, 0, 1), bc.ABx(bc.CALLH, 0, 0), bc.ABC(bc.RET, 0, 0, 0))
	return bc.Function{Name: name, Registers: 1, Script: 0, Coroutine: true, Code: code, Consts: consts}
}

func module() *bc.Module {
	m := bc.NewModule("co")
	m.Imports = []bc.HostImport{{Module: "t", Name: "hit", Sig: abi.Sig(abi.T(abi.Void), abi.T(abi.Int))}}
	m.Scripts = []bc.ScriptDef{{
		FullName: "co.S", Slots: 2,
		Fields: []bc.FieldDef{{Name: "Id", Type: "int", Slot: 1, Slots: 1}},
		Ready:  -1, FixedUpdate: -1, Update: -1, Reloaded: -1,
	}}
	yield := func(kind uint8) bc.Instr { return bc.ABC(bc.YIELD, 0, 0, kind) }
	m.Functions = []bc.Function{
		coFn("S.Next", nil, yield(bc.WaitNext)),
		coFn("S.Wait", []bc.Const{bc.FloatK(0.5)}, bc.ABx(bc.LOADK, 0, 0), yield(bc.WaitTime)),
		coFn("S.Long", []bc.Const{bc.FloatK(1000)}, bc.ABx(bc.LOADK, 0, 0), yield(bc.WaitTime)),
		coFn("S.Frames", nil, bc.AsBx(bc.LOADI, 0, 3), yield(bc.WaitFrames)),
		coFn("S.Fixed", nil, bc.AsBx(bc.LOADI, 0, 2), yield(bc.WaitFixed)),
		coFn("S.Await", nil, bc.ABx(bc.START, 0, 2), bc.ABC(bc.YIELD, 0, 0, bc.WaitAwait)),
	}
	return m
}

type rig struct {
	m     *vm.Machine
	s     *coroutine.Scheduler
	lm    *vm.Module
	hits  []int64
	selfs map[vm.InstanceID][]vm.Value
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{selfs: map[vm.InstanceID][]vm.Value{}}
	r.m = vm.New(vm.Options{})
	lm, err := r.m.Load(module(), func(bc.HostImport) (vm.HostFunc, bool) {
		return func(_ *vm.Machine, w []vm.Value) error {
			r.hits = append(r.hits, w[0].Int())
			return nil
		}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	r.lm = lm
	r.s = coroutine.New(r.m)
	r.s.Self = func(id vm.InstanceID) ([]vm.Value, bool) {
		v, ok := r.selfs[id]
		return v, ok
	}
	return r
}

func (r *rig) spawn(t *testing.T, name string, id int) vm.CoHandle {
	t.Helper()
	self := []vm.Value{{}, vm.Int(int64(id))}
	r.selfs[vm.InstanceID(id)] = self
	h, fl := r.s.Start(r.lm.Func(name), self, nil, vm.InstanceID(id))
	if fl != nil {
		t.Fatal(fl)
	}
	return h
}

func (r *rig) framesUntilHit(max int, step func()) int {
	for i := 1; i <= max; i++ {
		step()
		if len(r.hits) > 0 {
			return i
		}
	}
	return -1
}

func TestStartOrder(t *testing.T) {
	r := newRig(t)
	for i := 1; i <= 3; i++ {
		r.spawn(t, "S.Next", i)
	}
	r.s.Frame(1.0 / 60)
	r.s.Frame(1.0 / 60)
	if len(r.hits) != 3 || r.hits[0] != 1 || r.hits[1] != 2 || r.hits[2] != 3 {
		t.Fatalf("order %v", r.hits)
	}
}

func TestWaitFramesFixed(t *testing.T) {
	r := newRig(t)
	r.spawn(t, "S.Wait", 1)
	if n := r.framesUntilHit(10, func() { r.s.Frame(0.2) }); n != 3 {
		t.Fatalf("wait(0.5) at dt 0.2 resumed after %d frames", n)
	}
	r = newRig(t)
	r.spawn(t, "S.Frames", 1)
	if n := r.framesUntilHit(10, func() { r.s.Frame(1.0 / 60) }); n != 4 {
		t.Fatalf("frames(3) resumed after %d frames", n)
	}
	r = newRig(t)
	r.spawn(t, "S.Fixed", 1)
	for i := 0; i < 5; i++ {
		r.s.Frame(1.0 / 60)
	}
	if len(r.hits) != 0 {
		t.Fatal("a fixed wait resumed on Frame")
	}
	if n := r.framesUntilHit(10, r.s.Fixed); n != 3 {
		t.Fatalf("fixed(2) resumed after %d fixed steps", n)
	}
}

func TestOnlyReadyAreVisited(t *testing.T) {
	r := newRig(t)
	for i := 1; i <= 10_000; i++ {
		r.spawn(t, "S.Long", i)
	}
	for i := 10_001; i <= 10_100; i++ {
		r.spawn(t, "S.Next", i)
	}
	r.s.Frame(1.0 / 60)
	before := r.s.Stats()
	r.s.Frame(1.0 / 60)
	after := r.s.Stats()
	if d := after.Visits - before.Visits; d != 100 {
		t.Fatalf("frame visited %d coroutines, want 100", d)
	}
	if len(r.hits) != 100 || after.Sleeping != 10_000 {
		t.Fatalf("hits %d sleeping %d", len(r.hits), after.Sleeping)
	}
}

func TestStop(t *testing.T) {
	r := newRig(t)
	h := r.spawn(t, "S.Next", 1)
	r.s.Stop(h)
	r.s.Frame(1.0 / 60)
	if len(r.hits) != 0 || r.s.Stats().Canceled != 1 || r.s.Stats().Live != 0 {
		t.Fatalf("hits %v stats %+v", r.hits, r.s.Stats())
	}
}

func TestCancelRemovesSleepers(t *testing.T) {
	r := newRig(t)
	h := r.spawn(t, "S.Long", 1)
	r.s.Frame(1.0 / 60)
	if st := r.s.Stats(); st.Sleeping != 1 || st.Live != 1 {
		t.Fatalf("before cancel %+v", st)
	}
	r.s.Stop(h)
	if st := r.s.Stats(); st.Sleeping != 0 || st.Live != 0 || st.Canceled != 1 {
		t.Fatalf("after cancel %+v", st)
	}
	r.s.Frame(1.0 / 60)
	if st := r.s.Stats(); st.Sleeping != 0 {
		t.Fatalf("after frame %+v", st)
	}
}

func TestCancelRemovesAwaiter(t *testing.T) {
	r := newRig(t)
	w := r.spawn(t, "S.Await", 1)
	if st := r.s.Stats(); st.Awaiting != 1 || st.Sleeping != 1 || st.Live != 2 {
		t.Fatalf("before cancel %+v", st)
	}
	r.s.Stop(w)
	if st := r.s.Stats(); st.Awaiting != 0 || st.Sleeping != 1 || st.Live != 1 {
		t.Fatalf("after cancelling the waiter %+v", st)
	}
	r.s.CancelAll()
	if st := r.s.Stats(); st.Sleeping != 0 || st.Live != 0 {
		t.Fatalf("after cancel all %+v", st)
	}
}
