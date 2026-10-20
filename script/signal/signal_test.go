package signal

import (
	"errors"
	"slices"
	"testing"

	"aego/script/vm"
)

func TestOrderSnapshotAndDeath(t *testing.T) {
	r := New()
	var got []int
	var h3 Handle
	r.Connect(Conn{Src: ScriptSource(1), Sig: 0, Dst: 10, Method: 1})
	h2 := r.Connect(Conn{Src: ScriptSource(1), Sig: 0, Dst: 11, Method: 2})
	r.Connect(Conn{Src: ScriptSource(1), Sig: 1, Dst: 12, Method: 9})
	deliver := func(c Conn, _ []vm.Value) (bool, error) {
		dst, m := c.Dst, c.Method
		got = append(got, m)
		if m == 1 && h3.Gen == 0 {
			h3 = r.Connect(Conn{Src: ScriptSource(1), Sig: 0, Dst: 13, Method: 3})
			r.Disconnect(h2)
			if r.Connected(h2) {
				t.Fatal("Connected must be false right after Disconnect")
			}
		}
		return dst != 13, nil
	}
	if err := r.Emit(ScriptSource(1), 0, nil, deliver); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("first emit %v", got)
	}
	got = got[:0]
	r.Emit(ScriptSource(1), 0, nil, deliver)
	if !slices.Equal(got, []int{1, 3}) || r.Connected(h3) || r.Len() != 2 {
		t.Fatalf("second emit %v len %d", got, r.Len())
	}
}

func TestDropRebindError(t *testing.T) {
	r := New()
	r.Connect(Conn{Src: ScriptSource(1), Dst: 2, Signal: "A"})
	r.Connect(Conn{Src: ScriptSource(2), Dst: 3, Signal: "B"})
	r.Connect(Conn{Src: ScriptSource(4), Dst: 5, Signal: "C"})
	r.DropInstance(2)
	if r.Len() != 1 {
		t.Fatalf("len %d", r.Len())
	}
	if n := r.Rebind(func(c *Conn) bool { c.Sig = 7; return c.Signal != "C" }); n != 1 || r.Len() != 0 {
		t.Fatalf("rebind removed %d, len %d", n, r.Len())
	}
	r.Connect(Conn{Src: ScriptSource(1), Dst: 2})
	r.Connect(Conn{Src: ScriptSource(1), Dst: 3})
	calls := 0
	boom := errors.New("boom")
	err := r.Emit(ScriptSource(1), 0, nil, func(Conn, []vm.Value) (bool, error) {
		calls++
		return true, boom
	})
	if err != boom || calls != 1 {
		t.Fatalf("err %v calls %d", err, calls)
	}
	r.Reset()
	if r.Len() != 0 || len(r.bySrc) != 0 {
		t.Fatal("reset left connections")
	}
}

func TestEmitZeroAllocs(t *testing.T) {
	r := New()
	for i := 0; i < 10; i++ {
		r.Connect(Conn{Src: ScriptSource(1), Dst: vm.InstanceID(2 + i)})
	}
	d := Deliver(func(Conn, []vm.Value) (bool, error) { return true, nil })
	r.Emit(ScriptSource(1), 0, nil, d)
	if n := testing.AllocsPerRun(100, func() { r.Emit(ScriptSource(1), 0, nil, d) }); n != 0 {
		t.Fatalf("emit allocates %.1f", n)
	}
}
