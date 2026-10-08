package sceneio

import (
	"testing"

	"aego/scene"
	"aego/schema"
)

type twin struct {
	V float32 `aego:"v"`
}

func twinTypes(t *testing.T) *scene.Types {
	t.Helper()
	ty := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[twin](ty, schema.Name("test.Twin")); err != nil {
		t.Fatal(err)
	}
	if err := RegisterUnknown(ty); err != nil {
		t.Fatal(err)
	}
	if err := ty.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return ty
}

func queryCount(t *testing.T, src *scene.Types, dst *scene.Types) int {
	t.Helper()
	s := scene.New(src, nil, scene.Options{})
	h := s.Create(s.Root(), "N")
	scene.Add[twin](s, h).V = 1
	s.Flush()
	loaded, _, err := Load(Save(s), "t.aescene", dst, nil, LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	scene.Query1[twin](loaded).Each(func(scene.NodeHandle, *twin) { n++ })
	return n
}

func TestQuerySameRegistry(t *testing.T) {
	ty := twinTypes(t)
	if n := queryCount(t, ty, ty); n != 1 {
		t.Fatalf("query sees %d nodes", n)
	}
}

func TestQueryAcrossRegistries(t *testing.T) {
	if n := queryCount(t, twinTypes(t), twinTypes(t)); n != 1 {
		t.Fatalf("query sees %d nodes when the scene is loaded into a second registry", n)
	}
}
