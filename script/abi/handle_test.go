package abi

import "testing"

func TestHandleTypes(t *testing.T) {
	r := HandleOf("aego/tween.Tween")
	back, ok := Parse(r.String())
	if !ok || !back.Equal(r) || r.Slots() != 1 || !Plain(r) {
		t.Fatalf("round trip %v %v", back, ok)
	}
	if c, ok := Parse("Curve"); !ok || c.Kind != Curve {
		t.Fatal("Curve does not parse")
	}
	if HandleID("aego/tween.Tween") == HandleID("aego/timer.Timer") || HandleID("") == 0 {
		t.Fatal("handle ids must differ and never be zero")
	}
	tab := NewTable()
	if err := tab.AddHandle(HandleDef{Name: "aego/tween.Tween", Await: true}); err != nil {
		t.Fatal(err)
	}
	if err := tab.AddHandle(HandleDef{Name: "tween"}); err == nil {
		t.Fatal("a handle name needs a module")
	}
	if err := tab.AddHandleMethod("aego/tween.Tween", "Done", Sig(T(Bool))); err != nil {
		t.Fatal(err)
	}
	tab.Freeze()
	if _, ok := tab.HandleMethod("aego/tween.Tween", "Done"); !ok || len(tab.HandleMethodList()) != 1 {
		t.Fatal("handle method lost")
	}
}
