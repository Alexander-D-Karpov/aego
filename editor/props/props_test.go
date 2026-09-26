package props

import (
	"testing"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/history"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type Health struct {
	Max    float32 `aego:"max,min=0,max=100"`
	Locked int32   `aego:"locked,readonly"`
}

type target struct{ s *scene.Scene }

func (t target) Scene() *scene.Scene { return t.s }

func setup(t *testing.T) (*Access, []ids.NodeID) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Health](types, schema.Name("test.Health")); err != nil {
		t.Fatal(err)
	}
	sceneio.RegisterUnknown(types)
	types.Schema().Freeze()
	s := scene.New(types, nil, scene.Options{})
	var nodes []ids.NodeID
	for _, name := range []string{"A", "B", "C"} {
		h := s.Create(s.Root(), name)
		scene.Add[Health](s, h).Max = 10
		nodes = append(nodes, s.ID(h))
	}
	s.Flush()
	return New(s, history.New(target{s})), nodes
}

func field(t *testing.T, a *Access, name string) (schema.TypeID, schema.FieldID) {
	t.Helper()
	typeID := scene.Store[Health](a.Scene()).Type()
	fd, ok := a.Scene().Registry().Field(typeID, name)
	if !ok {
		t.Fatal("field missing")
	}
	return typeID, fd.ID
}

func TestMultiEditIsOneUndo(t *testing.T) {
	a, nodes := setup(t)
	typeID, max := field(t, a, "max")
	if err := a.Set(nil, nodes, typeID, max, schema.Float32(50)); err != nil {
		t.Fatal(err)
	}
	if a.History().Cursor() != 1 {
		t.Fatalf("cursor = %d", a.History().Cursor())
	}
	v, mixed, _ := a.Common(nodes, typeID, max)
	if mixed || v.AsFloat() != 50 {
		t.Fatalf("common = %v mixed=%v", v, mixed)
	}
	a.History().Undo()
	for _, n := range nodes {
		v, _ := a.Get(n, typeID, max)
		if v.AsFloat() != 10 {
			t.Fatal("undo missed a node")
		}
	}
}

func TestMixedValues(t *testing.T) {
	a, nodes := setup(t)
	typeID, max := field(t, a, "max")
	a.Set(nil, nodes[:1], typeID, max, schema.Float32(20))
	if _, mixed, _ := a.Common(nodes, typeID, max); !mixed {
		t.Fatal("different values must report mixed")
	}
}

func TestValidation(t *testing.T) {
	a, nodes := setup(t)
	typeID, max := field(t, a, "max")
	a.Set(nil, nodes, typeID, max, schema.Float32(500))
	v, _ := a.Get(nodes[0], typeID, max)
	if v.AsFloat() != 100 {
		t.Fatalf("max not clamped: %v", v.AsFloat())
	}
	if err := a.Validate(typeID, max, schema.String("x")); !errs.Has(err, errs.SchemaBadValue) {
		t.Fatalf("err = %v", err)
	}
	_, locked := field(t, a, "locked")
	if err := a.Set(nil, nodes, typeID, locked, schema.Int32(1)); !errs.Has(err, errs.SceneReadOnly) {
		t.Fatalf("err = %v", err)
	}
}

func TestEditTransformSkipsNoop(t *testing.T) {
	a, nodes := setup(t)
	a.EditTransform(nil, nodes, func(tr *scene.Transform2D) { tr.Position.X = 3 })
	a.EditTransform(nil, nodes, func(tr *scene.Transform2D) { tr.Position.X = 3 })
	if a.History().Cursor() != 1 {
		t.Fatalf("no-op transform entered history: %d", a.History().Cursor())
	}
}
