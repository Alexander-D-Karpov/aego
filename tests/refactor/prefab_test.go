package refactor_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/core/ids"
	"aego/project/refactor"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/script/bind"
)

const (
	enemyLeft = "c0000000000000000000000000000004"
	ground    = "c0000000000000000000000000000002"
	enemyRoot = "e1000000000000000000000000000001"
)

func sceneTypes(t *testing.T) *scene.Types {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

type prefabFixture struct {
	*fixture
	types *scene.Types
	lib   *sceneio.Library
	doc   *mutate.Document
}

func (f *prefabFixture) env() mutate.Env {
	return mutate.Env{Types: f.types, Library: f.lib, Lookup: f.pl.Lookup}
}

func (f *prefabFixture) library() {
	f.t.Helper()
	lib, bad := refactor.PrefabLibrary(f.types.Schema(), f.pl, nil)
	if len(bad) > 0 {
		f.t.Fatalf("prefab library: %v", bad)
	}
	f.lib = lib
}

func (f *prefabFixture) load(src []byte) {
	f.t.Helper()
	d, err := mutate.Load(src, mainScene, f.env())
	if err != nil {
		f.t.Fatal(err)
	}
	f.doc = d
}

func openPrefabs(t *testing.T) *prefabFixture {
	t.Helper()
	f := &prefabFixture{fixture: open(t), types: sceneTypes(t)}
	f.library()
	f.load(f.read(mainScene))
	return f
}

func nodeID(t *testing.T, s string) ids.NodeID {
	t.Helper()
	id, err := ids.ParseNodeID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *prefabFixture) tint(node string) string {
	f.t.Helper()
	res, err := f.doc.DryRun(mutate.Tx{Ops: []mutate.Op{mutate.Set("#"+node, "engine.SpriteRenderer.tint", "#000000ff")}})
	if err != nil {
		f.t.Fatal(err)
	}
	return res.Changes[0].Before
}

func TestPrefabLibraryLoadsEveryPrefab(t *testing.T) {
	f := openPrefabs(t)
	n := 0
	for _, a := range f.pl.Assets() {
		if a.Kind != asset.ArtPrefab {
			continue
		}
		p, ok := f.lib.Prefab(a.ID)
		if !ok {
			t.Fatalf("%s is not in the library", a.Path)
		}
		if _, _, err := prefab.Expand(f.lib, p, nil); err != nil {
			t.Fatalf("%s: %v", a.Path, err)
		}
		n++
	}
	if n < 2 {
		t.Fatalf("only %d prefabs", n)
	}
}

func TestApplyPrefabProperty(t *testing.T) {
	f := openPrefabs(t)
	const color = "#123456ff"
	if _, err := f.doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.Set("#"+enemyLeft, "engine.SpriteRenderer.tint", color)}}); err != nil {
		t.Fatal(err)
	}
	before := f.doc.Bytes()
	if !bytes.Contains(before, []byte(`prop="engine.SpriteRenderer.tint" value=`+color)) {
		t.Fatalf("the set did not reach the instance overrides:\n%s", before)
	}
	res, err := refactor.ApplyPrefab(f.doc, f.lib, f.pl, refactor.PrefabChange{
		Node: nodeID(t, enemyLeft), Level: prefab.LevelProperty, Prop: prefab.PropKey{Owner: "engine.SpriteRenderer", Field: "tint"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Edits) != 1 || res.Edits[0].Path != enemyPrefab || !bytes.Contains(res.Edits[0].After, []byte("tint="+color)) {
		t.Fatalf("edits %+v", res.Edits)
	}
	if bytes.Contains(f.doc.Bytes(), []byte(color)) {
		t.Fatal("the instance still overrides the applied tint")
	}
	if got := f.tint(enemyLeft); got != color {
		t.Fatalf("instance tint %s after apply", got)
	}
	applied := map[ids.AssetID]*prefab.Node{}
	disk, bad := refactor.PrefabLibrary(f.types.Schema(), f.pl, nil)
	if len(bad) > 0 {
		t.Fatal(bad)
	}
	for _, e := range res.Edits {
		applied[e.ID], _ = f.lib.Prefab(e.ID)
		old, _ := disk.Prefab(e.ID)
		f.lib.Set(e.ID, old)
	}
	if _, err := f.doc.Apply(res.Result.Inverse); err != nil || !bytes.Equal(f.doc.Bytes(), before) {
		t.Fatalf("inverse against the old prefab: %v", err)
	}
	for id, m := range applied {
		f.lib.Set(id, m)
	}
	if _, err := f.doc.Apply(res.Result.Redo); err != nil {
		t.Fatal(err)
	}
	if err := refactor.WriteAll(f.pl.Abs, res.Edits); err != nil {
		t.Fatal(err)
	}
	f.reimport(enemyPrefab)
	saved := f.doc.Bytes()
	f.library()
	f.load(saved)
	if got := f.tint(enemyLeft); got != color {
		t.Fatalf("tint %s after reload", got)
	}
	if got := f.tint("c0000000000000000000000000000007"); got != color {
		t.Fatalf("the other instance has tint %s", got)
	}
}

func TestApplyPrefabNode(t *testing.T) {
	f := openPrefabs(t)
	res, err := f.doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.AddNode("#"+enemyLeft, "Extra")}})
	if err != nil {
		t.Fatal(err)
	}
	added := nodeID(t, strings.TrimPrefix(res.Created[0], "#"))
	out, err := refactor.ApplyPrefab(f.doc, f.lib, f.pl, refactor.PrefabChange{Node: added, Level: prefab.LevelNode})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Edits) != 1 || !bytes.Contains(out.Edits[0].After, []byte(`name="Extra"`)) {
		t.Fatalf("edits %+v", out.Edits)
	}
	if h := f.doc.Scene.Handle(added); h.IsZero() || !f.doc.Scene.IsAlive(h) {
		t.Fatal("the applied node lost its scene ID")
	}
}

func TestApplyPrefabNothing(t *testing.T) {
	f := openPrefabs(t)
	_, err := refactor.ApplyPrefab(f.doc, f.lib, f.pl, refactor.PrefabChange{
		Node: nodeID(t, enemyLeft), Level: prefab.LevelProperty, Prop: prefab.PropKey{Owner: "engine.SpriteRenderer", Field: "tint"},
	})
	if !errors.Is(err, mutate.ErrNothing) {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyPrefabNearest(t *testing.T) {
	t.Skip("the platformer has no nested prefabs, only the fast_enemy variant")
}

func subtreeIDs(s *scene.Scene, h scene.NodeHandle) []ids.NodeID {
	var out []ids.NodeID
	var walk func(n scene.NodeHandle)
	walk = func(n scene.NodeHandle) {
		out = append(out, s.ID(n))
		s.Children(n, func(c scene.NodeHandle) bool {
			walk(c)
			return true
		})
	}
	walk(h)
	return out
}

func TestCreatePrefab(t *testing.T) {
	f := openPrefabs(t)
	const rel = "prefabs/ground.aeprefab"
	s := f.doc.Scene
	h := s.Handle(nodeID(t, ground))
	want := sceneio.SaveSubtree(s, h, true)
	keep := subtreeIDs(s, h)
	id, _, err := refactor.CreatePrefab(f.pl, f.doc, f.lib, "#"+ground, rel)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := f.pl.Lookup(rel); !ok || got != id || !exists(f.fixture, rel+".meta") {
		t.Fatalf("prefab %s is not in the pipeline", rel)
	}
	if !bytes.Equal(f.read(rel), want) {
		t.Fatalf("prefab file differs from the subtree:\n%s", f.read(rel))
	}
	h = s.Handle(nodeID(t, ground))
	if pi, ok := sceneio.InstanceOf(s, h); !ok || !pi.Owner || pi.Prefab != id {
		t.Fatal("the node did not become an instance of the new prefab")
	}
	for _, n := range keep {
		if s.Handle(n).IsZero() {
			t.Fatalf("node %s lost its ID", n)
		}
	}
	before := f.doc.Bytes()
	if _, _, err := refactor.CreatePrefab(f.pl, f.doc, f.lib, "#"+ground, rel); err == nil {
		t.Fatal("creating over an existing prefab was accepted")
	}
	if !bytes.Equal(f.doc.Bytes(), before) {
		t.Fatal("a failed create changed the scene")
	}
}

func TestCreatePrefabRefusesInstanceChild(t *testing.T) {
	f := openPrefabs(t)
	const rel = "prefabs/sprite.aeprefab"
	inside := ""
	s := f.doc.Scene
	s.Children(s.Handle(nodeID(t, enemyLeft)), func(c scene.NodeHandle) bool {
		inside = s.ID(c).String()
		return false
	})
	if inside == "" {
		t.Skip("the enemy prefab has no child node")
	}
	if _, _, err := refactor.CreatePrefab(f.pl, f.doc, f.lib, "#"+inside, rel); err == nil {
		t.Fatal("a node inside an instance became a prefab")
	}
	if _, err := os.Stat(f.abs(rel)); err == nil || exists(f.fixture, rel+".meta") {
		t.Fatal("a failed create left files behind")
	}
	if _, ok := f.pl.Lookup(rel); ok {
		t.Fatal("a failed create left the prefab in the pipeline")
	}
}
