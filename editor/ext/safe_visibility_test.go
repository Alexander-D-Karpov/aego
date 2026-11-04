package ext

import (
	"testing"

	"aego/core/ids"
	"aego/editor/core/context"
	"aego/schema"
)

type brokenFieldFilter struct{}

func (brokenFieldFilter) Draw(_ *context.Editor, _ []ids.NodeID, defaults func()) { defaults() }
func (brokenFieldFilter) FieldVisible(*context.Editor, []ids.NodeID, *schema.FieldDescriptor) bool {
	panic("bad optional visibility filter")
}

func TestOptionalFieldFilterFailureRestoresGenericInspector(t *testing.T) {
	r, _ := setup(t)
	c := &safeCustom{inner: brokenFieldFilter{}, ext: "filter", reg: r}
	if !c.FieldVisible(nil, nil, &schema.FieldDescriptor{Name: "savedValue"}) {
		t.Fatal("a failed extension hid a persisted field")
	}
	r.Flush()
	if _, disabled := r.Disabled()["filter"]; !disabled {
		t.Fatal("panicking filter was not disabled")
	}
}
