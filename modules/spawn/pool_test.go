package spawn

import (
	"slices"
	"testing"

	"aego/contracts/scriptevents"
	"aego/scene"
	"aego/schema"
)

func TestRespawnKeepsTemplateTags(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[scene.Tags](types, schema.Name(scene.TagsName)); err != nil {
		t.Fatal(err)
	}
	sc := scene.New(types, nil, scene.Options{})
	h := sc.Create(sc.Root(), "Enemy")
	scene.Add[scene.Tags](sc, h).Tags = []string{"Enemy", "Flying", "Boss"}
	sc.Flush()
	svc := NewService(sc, nil, Events{Spawned: scriptevents.NoEvent, Despawned: scriptevents.NoEvent})
	m := &member{}
	svc.capture(h, m)
	want := []string{"Enemy", "Flying", "Boss"}
	if !sc.RemoveTag(h, "Enemy") {
		t.Fatal("RemoveTag failed")
	}
	svc.restore(m)
	got, _ := scene.Get[scene.Tags](sc, h)
	if !slices.Equal(got.Tags, want) {
		t.Fatalf("after RemoveTag and respawn got %v, want %v", got.Tags, want)
	}
	got.Tags[0] = "Changed"
	svc.restore(m)
	if got, _ = scene.Get[scene.Tags](sc, h); !slices.Equal(got.Tags, want) {
		t.Fatalf("a write into the spawned copy reached the template: %v", got.Tags)
	}
}
