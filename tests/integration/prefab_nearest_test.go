package integration

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/editor/core/prefabs"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/script/bind"
)

func TestApplyNearestToNodeAddedInsideNested(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	qid, pid := ids.AssetID{Hi: 0xa1, Lo: 1}, ids.AssetID{Hi: 0xa1, Lo: 2}
	q1 := &prefab.Node{Local: ids.NodeID{Hi: 1, Lo: 1}}
	q := &prefab.Node{Local: ids.NodeID{Hi: 1, Lo: 0}, Children: []*prefab.Node{q1}}
	x := &prefab.Node{Local: ids.NodeID{Hi: 3, Lo: 1}}
	n := &prefab.Node{Local: ids.NodeID{Hi: 2, Lo: 1}, Instance: &prefab.Instance{Prefab: qid, Overrides: prefab.Overrides{
		AddNodes: []prefab.NodeAdd{{Parent: prefab.LocalKey(q1.Local), Node: x}},
	}}}
	p := &prefab.Node{Local: ids.NodeID{Hi: 2, Lo: 0}, Children: []*prefab.Node{n}}
	lib := sceneio.NewLibrary(types.Schema(), nil)
	lib.Set(qid, q)
	lib.Set(pid, p)
	key := prefab.Key(string(prefab.LocalKey(n.Local)) + "/" + string(prefab.LocalKey(x.Local)))
	got, ok := prefabs.NearestForTest(lib, pid, key)
	if !ok || got != pid {
		t.Fatalf("nearest of a node added inside the nested instance = %v %v, want the outer prefab", got, ok)
	}
	if _, ok := prefabs.NearestForTest(lib, pid, prefab.Key(string(prefab.LocalKey(n.Local))+"/"+string(prefab.LocalKey(q1.Local)))); !ok {
		t.Fatal("a node of the nested prefab itself still routes to the nested prefab")
	}
}
