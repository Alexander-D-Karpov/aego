package schema

import (
	"testing"
	"unsafe"

	"aego/core/mathx"
)

type flatCommon struct {
	Offset    mathx.Vec2
	IsTrigger bool
}

type FlatCommon = flatCommon

type flatBox struct {
	FlatCommon
	Size mathx.Vec2 `aego:"size,gizmo=vector"`
}

type badGizmo struct {
	R float32 `aego:"r,gizmo=vector"`
}

func TestFlattenEmbedded(t *testing.T) {
	r := NewRegistry()
	td, err := Register[flatBox](r, Name("t.Box"))
	if err != nil {
		t.Fatal(err)
	}
	fd, ok := td.Field("isTrigger")
	if !ok {
		t.Fatal("embedded field is not visible")
	}
	var b flatBox
	if err := fd.Set(unsafe.Pointer(&b), Bool(true)); err != nil || !b.IsTrigger {
		t.Fatal("embedded accessor writes the wrong offset")
	}
	if sz, _ := td.Field("size"); sz.Hints.Gizmo != GizmoVector {
		t.Fatal("gizmo tag lost")
	}
}

func TestGizmoKindChecked(t *testing.T) {
	if _, err := Register[badGizmo](NewRegistry()); err == nil {
		t.Fatal("vector gizmo on a float was accepted")
	}
}
