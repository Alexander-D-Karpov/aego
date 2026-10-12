package module

import (
	"strings"
	"testing"
	"testing/fstest"

	"aego/scene"
	"aego/schema"
)

type iconComp struct {
	V float32 `aego:"v"`
}

type iconModule struct{}

func (iconModule) Info() Info {
	return Info{ID: "icontest"}
}

func (iconModule) Register(r *Registry) error {
	r.Icons(fstest.MapFS{
		"extra@16x1.png": {Data: []byte("png")},
		"bad.png":        {Data: []byte("png")},
	}, "icontest.")
	return Component[iconComp](r, schema.Name("icontest.Comp")).Icon("icontest.need").Err()
}

func TestAuditListsEveryDifference(t *testing.T) {
	rep, err := Audit(scene.NewTypes(schema.NewRegistry()), nil, iconModule{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[IconProblemKind]string{
		IconMissing: "icontest.need",
		IconUnused:  "icontest.extra",
		IconBadName: "bad.png",
	}
	for _, p := range rep.Problems {
		if want[p.Kind] == p.Icon {
			delete(want, p.Kind)
		}
	}
	if len(want) > 0 {
		t.Fatalf("not reported: %v in %+v", want, rep.Problems)
	}
	var out strings.Builder
	if rep.Write(&out) != 1 || !strings.Contains(out.String(), "icontest.need") || !strings.Contains(out.String(), "icontest.extra") {
		t.Fatalf("output:\n%s", out.String())
	}
	if _, err := Load(scene.NewTypes(schema.NewRegistry()), nil, iconModule{}); err == nil {
		t.Fatal("Build must still reject an unknown icon")
	}
}
