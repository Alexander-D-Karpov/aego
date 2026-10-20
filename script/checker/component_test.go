package checker

import (
	"testing"

	"aego/script/types"
)

func TestComponentMethodCalls(t *testing.T) {
	src := `package t

script S {
	func Update(dt float) {
		sr := node.Get<SpriteRenderer>()
		sr.Flip(true)
		sr.Order = int(sr.Pulse(1, Vec2{0, 1}) + dt)
	}
}
`
	a, sm, ds := check(t, map[string]string{"game/t": src})
	if len(ds) > 0 {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
	n := 0
	for _, c := range a.Calls {
		if c.Kind == CallComponentMethod {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("component method calls = %d", n)
	}
}

func TestComponentMethodErrors(t *testing.T) {
	src := `package t

script S {
	func Ready() {
		sr := node.Get<SpriteRenderer>()
		sr.Missing()
		sr.FlipX()
		sr.Flip(1)
	}
}
`
	_, _, ds := check(t, map[string]string{"game/t": src})
	got := codes(ds)
	if got["AES2006"] != 1 || len(ds) != 3 {
		t.Fatalf("diagnostics %v", got)
	}
}

func TestVectorFieldDefaults(t *testing.T) {
	src := `package t

const Up float = -1

script S {
	@export
	var Jump Vec2 = Vec2{0, -5}
	@export
	var Tint Color = Color{R: 1, A: 0.5}
	@export
	var Aim Vec2 = Vec2{X: 2, Y: Up * 3}
	var Area Rect = Rect{1, 2, 3, 4}
}
`
	a, sm, ds := check(t, map[string]string{"game/t": src})
	if len(ds) > 0 {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
	fs := a.Scripts[0].Fields
	want := [][4]float64{{0, -5}, {1, 0, 0, 0.5}, {2, -3}, {1, 2, 3, 4}}
	for i, f := range fs {
		if f.Default == nil || f.Default.Kind != types.CLanes || f.Default.Lanes != want[i] {
			t.Fatalf("%s default %+v", f.Name, f.Default)
		}
	}
}

func TestVectorFieldDefaultNeedsConstants(t *testing.T) {
	src := `package t

func f() float {
	return 1
}

script S {
	var Jump Vec2 = Vec2{0, f()}
}
`
	_, _, ds := check(t, map[string]string{"game/t": src})
	if codes(ds)["AES3006"] != 1 {
		t.Fatalf("diagnostics %v", codes(ds))
	}
}

func TestPersistField(t *testing.T) {
	src := `package t

script S {
	@persist
	var Coins int
	@persist @export
	var Spawn Vec2
	var Temp float
}
`
	a, sm, ds := check(t, map[string]string{"game/t": src})
	if len(ds) > 0 {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
	fs := a.Scripts[0].Fields
	if !fs[0].Persist || !fs[1].Persist || !fs[1].Export || fs[2].Persist {
		t.Fatalf("persist flags %v %v %v", fs[0].Persist, fs[1].Persist, fs[2].Persist)
	}
}

func TestPersistRejectsSlices(t *testing.T) {
	src := `package t

script S {
	@persist
	var Items []int
	@persist("x")
	var N int
}
`
	_, _, ds := check(t, map[string]string{"game/t": src})
	c := codes(ds)
	if c["AES3007"] != 1 || c["AES3003"] != 1 {
		t.Fatalf("diagnostics %v", c)
	}
}
