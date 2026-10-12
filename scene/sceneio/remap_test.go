package sceneio_test

import (
	"testing"

	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type linker struct {
	Target scene.NodeRef `aego:"target"`
}

type linkModule struct{}

func (linkModule) Info() module.Info {
	return module.Info{ID: "link"}
}

func (linkModule) Register(r *module.Registry) error {
	return module.Component[linker](r, schema.Name("link.Linker")).Err()
}

func TestLoadIntoRemapsInternalRefs(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, linkModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	src := scene.New(types, nil, scene.Options{})
	root := src.Create(src.Root(), "Prefab")
	b := src.Create(root, "B")
	a := src.Create(root, "A")
	scene.Add[linker](src, a).Target = scene.NewNodeRef(src.ID(b))
	src.Flush()
	data := sceneio.SaveSubtree(src, root, true)

	dst := scene.New(types, nil, scene.Options{})
	var roots []scene.NodeHandle
	for i := 0; i < 2; i++ {
		created, rep, err := sceneio.LoadInto(dst, dst.Root(), data, "prefab", sceneio.LoadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Unresolved) != 0 || len(rep.Warnings) != 0 {
			t.Fatalf("report = %+v", rep)
		}
		roots = append(roots, created[0])
	}
	for _, r := range roots {
		got, ok := scene.Get[linker](dst, dst.Find(r, "A"))
		if !ok {
			t.Fatal("linker lost")
		}
		if want := dst.ID(dst.Find(r, "B")); got.Target.ID != want || got.Target.ID == src.ID(b) {
			t.Fatalf("instance points at %s, want %s", got.Target.ID.Short(), want.Short())
		}
	}
}
