package inspect

import (
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/schema"
	"testing"
)

type filteredInspector struct{}

func (filteredInspector) Draw(_ *context.Editor, _ []ids.NodeID, def func()) { def() }
func (filteredInspector) FieldVisible(_ *context.Editor, _ []ids.NodeID, fd *schema.FieldDescriptor) bool {
	return fd.Name != "hiddenForThisSelection"
}
func TestOptionalFieldVisibilityAndDisabledFallback(t *testing.T) {
	r := NewRegistry()
	id := schema.TypeID(71)
	fd := &schema.FieldDescriptor{Name: "hiddenForThisSelection"}
	if !r.FieldVisible(id, nil, fd) {
		t.Fatal("default inspector hid field")
	}
	if err := r.RegisterCustomID(id, filteredInspector{}); err != nil {
		t.Fatal(err)
	}
	if r.FieldVisible(id, nil, fd) {
		t.Fatal("filter not applied")
	}
	r.Disable(id, "failed plugin")
	if !r.FieldVisible(id, nil, fd) {
		t.Fatal("disabled plugin still hides fields")
	}
}
