package inspector

import (
	"testing"

	"aego/core/ids"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type refHolder struct {
	A scene.NodeRef `aego:"a"`
	B scene.NodeRef `aego:"b"`
}

type refModule struct{}

func (refModule) Info() module.Info {
	return module.Info{ID: "reftest"}
}

func (refModule) Register(r *module.Registry) error {
	return module.Component[refHolder](r, schema.Name("reftest.Holder")).Err()
}

func refTypes(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, refModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

func TestRefState(t *testing.T) {
	types := refTypes(t)
	s := scene.New(types, nil, scene.Options{})
	holder := s.Create(s.Root(), "Holder")
	target := s.Create(s.Root(), "Target")
	gone := s.ID(target)
	ref := scene.Add[refHolder](s, holder)
	ref.B = scene.NewNodeRef(gone)
	s.Destroy(target)
	s.Flush()

	cases := []struct {
		id    ids.NodeID
		need  string
		kind  refKind
		label string
	}{
		{ids.NodeID{}, "", refNil, "None"},
		{s.ID(holder), "", refAlive, "Holder"},
		{gone, "", refMissing, "Missing"},
		{s.ID(holder), "engine.Unknown", refNoComponent, "Holder · no Unknown"},
		{s.ID(holder), "reftest.Holder", refAlive, "Holder"},
	}
	for _, c := range cases {
		got := nodeState(s, c.id, c.need)
		if got.kind != c.kind || got.label != c.label {
			t.Errorf("nodeState(%s, %q) = %v %q, want %v %q", c.id.Short(), c.need, got.kind, got.label, c.kind, c.label)
		}
	}
	if st := assetState(nil, ids.AssetID{}); st.kind != refNil {
		t.Errorf("nil asset = %v", st.kind)
	}
	if st := assetState(nil, ids.NewAssetID()); st.kind != refMissing || st.label != "Missing asset" {
		t.Errorf("unknown asset = %v %q", st.kind, st.label)
	}

	data := sceneio.Save(s)
	back, rep, err := sceneio.Load(data, "refs.aescene", types, nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 0 {
		t.Fatalf("warnings: %+v", rep.Warnings)
	}
	if len(rep.Unresolved) != 1 || rep.Unresolved[0].Target != gone {
		t.Fatalf("unresolved = %+v", rep.Unresolved)
	}
	got, ok := scene.Get[refHolder](back, back.Find(back.Root(), "Holder"))
	if !ok {
		t.Fatal("holder lost")
	}
	if !got.A.ID.IsZero() || nodeState(back, got.A.ID, "").kind != refNil {
		t.Fatalf("nil ref became %s", got.A.ID.Short())
	}
	if got.B.ID != gone || nodeState(back, got.B.ID, "").kind != refMissing {
		t.Fatalf("missing ref became %s", got.B.ID.Short())
	}
	if string(sceneio.Save(back)) != string(data) {
		t.Fatal("round trip is not stable")
	}
}
