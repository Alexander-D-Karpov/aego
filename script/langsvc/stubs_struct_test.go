package langsvc

import (
	"strings"
	"testing"

	"aego/core/mathx"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bind/manifest"
	"aego/script/workspace"
)

type stubHit struct {
	Point    mathx.Vec2
	Distance float32
}

func TestStubListsHostStructsAndEvents(t *testing.T) {
	r := bind.New()
	bind.Struct[stubHit](r.Module("test/phys"), "RayHit", bind.Doc("RayHit is the result of a raycast."))
	if err := r.Event(abi.EventDef{Name: "OnHit", Params: []abi.TypeRef{abi.T(abi.Int)}}, bind.Doc("Called on a hit.")); err != nil {
		t.Fatal(err)
	}
	man, err := manifest.FromRegistry(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	tab, err := man.Table()
	if err != nil {
		t.Fatal(err)
	}
	snap := &workspace.Snapshot{Bindings: tab, Docs: man}
	st, ok := StubFor(snap, StubPath("test/phys"))
	if !ok {
		t.Fatal("no stub for the module")
	}
	for _, want := range []string{"type RayHit struct", "Point", "Distance", "RayHit is the result of a raycast."} {
		if !strings.Contains(st.Text, want) {
			t.Fatalf("stub lacks %q:\n%s", want, st.Text)
		}
	}
	bi, _ := StubFor(snap, StubPath(StubBuiltin))
	if !strings.Contains(bi.Text, "func OnHit") || !strings.Contains(bi.Text, "Called on a hit.") {
		t.Fatalf("builtin stub lacks the event:\n%s", bi.Text)
	}
}
