package checker

import (
	"testing"

	"aego/script/diag"
)

func TestHostStructAndEvents(t *testing.T) {
	src := `package ev

import "aego/probe"

script Probe {
	var last probe.RayHit

	func BodyEntered(other NodeRef) {
		last = probe.Raycast(Vec2{}, Vec2{X: 1})
		hit := probe.RayHit{Distance: 2}
		last.Distance += hit.Distance
	}
}
`
	_, sm, ds := check(t, map[string]string{"game/ev": src})
	if len(ds) > 0 {
		t.Fatalf("\n%s", dump(sm, ds))
	}
}

func TestEventErrors(t *testing.T) {
	src := `package ev

script Probe {
	func BodyEntered(n int) {
	}
}

script Caller {
	func BodyEntered(other NodeRef) {
	}

	func Update(dt float) {
		BodyEntered(nil)
	}
}
`
	_, sm, ds := check(t, map[string]string{"game/ev": src})
	c := codes(ds)
	for _, code := range []string{diag.EventSignature, diag.EventCall} {
		if c[code] != 1 {
			t.Errorf("%s reported %d times\n%s", code, c[code], dump(sm, ds))
		}
	}
}
