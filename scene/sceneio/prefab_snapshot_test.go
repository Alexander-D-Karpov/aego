package sceneio

import (
	"testing"

	"aego/core/curve"
	"aego/scene"
	"aego/scene/prefab"
	"aego/schema"
)

func TestPrefabSnapshotRetainsIndependentNestedOrphanProperties(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := RegisterPrefab(types); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "Prefab")
	pi := scene.Add[PrefabInstance](s, h)
	pi.Orphans.Props = []prefab.PropOverride{{Value: schema.List(schema.Curve(curve.Constant(3)))}}
	pi.Orphans.AddComps = []prefab.CompAdd{{Comp: prefab.Comp{Fields: []prefab.Field{{Name: "nested", Value: schema.List(schema.Curve(curve.Constant(7)))}}}}}
	nested := schema.Value{Kind: schema.KindStruct, Obj: &schema.Object{Fields: []schema.Value{schema.Curve(curve.Constant(11))}}}
	added := &prefab.Node{Comps: []prefab.Comp{{Fields: []prefab.Field{{Name: "nested", Value: nested}}}}}
	pi.Orphans.AddNodes = []prefab.NodeAdd{{Node: added}}
	s.Flush()
	key := s.Capture(nil)
	for i := 0; i < 3; i++ {
		pi, _ = scene.Get[PrefabInstance](s, h)
		pi.Orphans.Props[0].Value.List[0].Cv.Keys[0].V = 99
		pi.Orphans.AddComps[0].Comp.Fields[0].Value.List[0].Cv.Keys[0].V = 99
		pi.Orphans.AddNodes[0].Node.Comps[0].Fields[0].Value.Obj.Fields[0].Cv.Keys[0].V = 99
		s.Restore(key)
		pi, _ = scene.Get[PrefabInstance](s, h)
		if got := pi.Orphans.Props[0].Value.List[0].Cv.Keys[0].V; got != 3 {
			t.Fatalf("nested orphan property aliases snapshot after restore %d: %v", i, got)
		}
		if got := pi.Orphans.AddComps[0].Comp.Fields[0].Value.List[0].Cv.Keys[0].V; got != 7 {
			t.Fatalf("nested added component property aliases snapshot after restore %d: %v", i, got)
		}
		if got := pi.Orphans.AddNodes[0].Node.Comps[0].Fields[0].Value.Obj.Fields[0].Cv.Keys[0].V; got != 11 {
			t.Fatalf("nested added node property aliases snapshot after restore %d: %v", i, got)
		}
	}
}
