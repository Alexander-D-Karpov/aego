package fakephys_test

import (
	"strings"
	"testing"

	"aego/engine/module"
	"aego/scene"
	"aego/schema"
)

type badComp struct {
	Size  float32 `aego:"size"`
	Cache []int   `aego:"-"`
}

type badModule struct{}

func (badModule) Info() module.Info {
	return module.Info{ID: "badclone", Version: "1"}
}

func (badModule) Register(r *module.Registry) error {
	return module.Component[badComp](r, schema.Name("badclone.Comp")).Err()
}

func TestSkippedReferenceFieldNeedsClone(t *testing.T) {
	r := module.New(scene.NewTypes(schema.NewRegistry()), nil)
	if err := r.Add(badModule{}); err != nil {
		t.Fatal(err)
	}
	_, err := r.Build()
	if err == nil || !strings.Contains(err.Error(), "badclone") || !strings.Contains(err.Error(), "Cache") {
		t.Fatalf("got %v, want an error naming the module and the field", err)
	}
}
