package manifest

import (
	"testing"

	"aego/core/mathx"
	"aego/script/abi"
	"aego/script/bind"
)

type two struct{ A, B float32 }

type three struct {
	A, B float32
	C    mathx.Vec2
}

func hashOf[T any](t *testing.T) [32]byte {
	r := bind.New()
	bind.Struct[T](r.Module("test/m"), "S")
	m, err := FromRegistry(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m.ABIHash
}

func TestStructLayoutChangesHash(t *testing.T) {
	if hashOf[two](t) == hashOf[three](t) {
		t.Fatal("adding a field kept the ABI hash")
	}
}

func TestManifestKeepsStructsAndEvents(t *testing.T) {
	r := bind.New()
	bind.Struct[three](r.Module("test/m"), "S", bind.Doc("doc of S"))
	if err := r.Event(abi.EventDef{Name: "OnHit", Params: []abi.TypeRef{abi.StructOf("test/m.S")}}); err != nil {
		t.Fatal(err)
	}
	m, err := FromRegistry(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(Encode(m))
	if err != nil {
		t.Fatal(err)
	}
	if back.ABIHash != m.ABIHash || len(back.Structs) != 1 || len(back.Events) != 1 {
		t.Fatalf("decoded %+v", back)
	}
	if s, _ := back.StructDoc("test/m", "S"); s.Doc != "doc of S" || s.Slots != 3 {
		t.Fatalf("struct %+v", s)
	}
}
