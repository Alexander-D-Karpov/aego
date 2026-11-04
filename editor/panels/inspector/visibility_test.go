package inspector

import (
	"testing"

	"aego/schema"
)

func TestSelectionFilterPreservesRemainingTypedRowsAndSchema(t *testing.T) {
	types, _ := testTypes(t)
	td, _ := types.Schema().ByName("test.Rows")
	before := plan(td, false)
	got := planFiltered(td, false, func(fd *schema.FieldDescriptor) bool {
		return fd.Name != "layer" && fd.Name != "flipY"
	})
	want := []fieldRow{{1, -1}, {2, -1}, {3, -1}, {5, -1}, {6, -1}, {7, -1}}
	if len(got) != len(want) {
		t.Fatalf("filtered rows: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d: %v, expected %v", i, got[i], want[i])
		}
	}
	after := plan(td, false)
	for i := range before {
		if before[i] != after[i] {
			t.Fatal("selection-specific visibility mutated shared schema")
		}
	}
}
