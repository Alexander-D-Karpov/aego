package signal

import (
	"testing"

	"aego/script/vm"
)

func goDeliver(c Conn, args []vm.Value) (bool, error) {
	if c.Go == nil {
		return true, nil
	}
	c.Go(args)
	return true, nil
}

func TestComponentSourceAndDropNode(t *testing.T) {
	r := New()
	comp := Source{Kind: KindComponent, A: 7, B: 42}
	script := ScriptSource(7)
	var got []int64
	h := r.Connect(Conn{Src: comp, Sig: 1, Go: func(a []vm.Value) { got = append(got, a[0].Int()) }})
	r.Connect(Conn{Src: comp, Sig: 0, Go: func([]vm.Value) { t.Fatal("another signal was delivered") }})
	s := r.Connect(Conn{Src: script, Sig: 1, Go: func([]vm.Value) {}})
	if !r.Has(comp) || r.Has(Source{Kind: KindComponent, A: 7, B: 43}) {
		t.Fatal("Has")
	}
	if err := r.Emit(comp, 1, []vm.Value{vm.Int(5)}, goDeliver); err != nil || len(got) != 1 || got[0] != 5 {
		t.Fatalf("emit %v %v", got, err)
	}
	r.DropNode(7)
	if r.Connected(h) || r.Has(comp) || !r.Connected(s) || r.Len() != 1 {
		t.Fatal("DropNode must drop component sources of that node only")
	}
}

func TestHandleSourceEndAndScriptTargets(t *testing.T) {
	r := New()
	src := Source{Kind: KindHandle, A: 99, B: 3}
	a := r.Connect(Conn{Src: src, Dst: 5, Method: 0})
	b := r.Connect(Conn{Src: src, Go: func([]vm.Value) {}})
	r.DropInstance(5)
	if r.Connected(a) || !r.Connected(b) {
		t.Fatal("DropInstance must drop connections whose target stopped")
	}
	r.DropSource(src)
	if r.Connected(b) || r.Len() != 0 {
		t.Fatal("DropSource")
	}
}

func TestFailedDeliveryDisconnects(t *testing.T) {
	r := New()
	src := Source{Kind: KindComponent, A: 1, B: 1}
	h := r.Connect(Conn{Src: src, Go: func([]vm.Value) {}})
	r.Emit(src, 0, nil, func(Conn, []vm.Value) (bool, error) { return false, nil })
	if r.Connected(h) {
		t.Fatal("a delivery that reports failure must disconnect")
	}
}

func TestDropDuringEmitAppliesAfter(t *testing.T) {
	r := New()
	src := Source{Kind: KindComponent, A: 2, B: 2}
	calls := 0
	r.Connect(Conn{Src: src, Go: func([]vm.Value) { calls++; r.DropNode(2) }})
	r.Connect(Conn{Src: src, Go: func([]vm.Value) { calls++ }})
	r.Emit(src, 0, nil, goDeliver)
	if calls != 2 || r.Len() != 0 || r.Has(src) {
		t.Fatalf("calls %d len %d", calls, r.Len())
	}
}
