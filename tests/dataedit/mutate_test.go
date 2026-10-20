package dataedit_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/data"
	"aego/editor/core/dataedit"
	"aego/format/aetext"
	"aego/schema"
)

func TestDocMergesAndUndoes(t *testing.T) {
	env, reg := envOf(t)
	lib := data.NewLibrary(reg, baseStore())
	src := data.WriteDoc(&data.Doc{Type: "test.Stats", Base: baseID, Fields: []aetext.Field{{Name: "mystery", Value: aetext.Int(7)}}})
	d, err := dataedit.NewDoc(env, lib, childID, "assets/fast.aedata", src)
	if err != nil {
		t.Fatal(err)
	}
	orig := d.Bytes()
	speed := field(t, reg, "speed")
	if err := d.Set(speed, schema.Float32(3), true); err != nil {
		t.Fatal(err)
	}
	if err := d.Set(speed, schema.Float32(4), false); err != nil {
		t.Fatal(err)
	}
	if err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Bytes(), orig) || d.CanUndo() || d.Dirty() {
		t.Fatalf("one undo must revert a merged drag:\n%s", d.Bytes())
	}
	if err := d.Redo(); err != nil || d.Value(speed).AsFloat() != 4 {
		t.Fatalf("redo: %v, speed %v", err, d.Value(speed).AsFloat())
	}
	if !bytes.Contains(d.Bytes(), []byte("mystery=7")) {
		t.Fatalf("an unknown field was dropped:\n%s", d.Bytes())
	}
	if err := d.Set(speed, schema.Float32(2), true); err != nil || d.Overridden("speed") {
		t.Fatalf("a value equal to the base must reset the field: %v", err)
	}
	if err := d.Set(speed, schema.Float32(6), true); err != nil {
		t.Fatal(err)
	}
	if err := d.Revert("speed"); err != nil || d.Overridden("speed") || d.Value(speed).AsFloat() != 2 {
		t.Fatalf("revert: %v", err)
	}
	if err := d.SetBase(ids.AssetID{}); err != nil || !d.Base().IsZero() {
		t.Fatalf("clear base: %v", err)
	}
	if err := d.Undo(); err != nil || d.Base() != baseID {
		t.Fatalf("undo base: %v", err)
	}
	if err := d.Redo(); err != nil || !d.Base().IsZero() {
		t.Fatalf("redo base: %v", err)
	}
	for d.CanUndo() {
		if err := d.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if d.Dirty() || !bytes.Equal(d.Bytes(), orig) {
		t.Fatalf("undo to the saved state must be clean:\n%s", d.Bytes())
	}
}

func table(t *testing.T) (*dataedit.Table, *schema.FieldDescriptor) {
	t.Helper()
	env, reg := envOf(t)
	row := func(id string, speed float32) data.TableRow {
		return data.TableRow{ID: id, Fields: []aetext.Field{{Name: "speed", Value: aetext.Float32(speed)}}}
	}
	doc := &data.TableDoc{Type: "test.Stats", Rows: []data.TableRow{row("x", 1), row("x_copy", 2), row("y", 3)}}
	tab, err := dataedit.NewTable(env, ids.AssetID{Hi: 3, Lo: 1}, "assets/t.aetable", data.WriteTable(doc))
	if err != nil {
		t.Fatal(err)
	}
	return tab, field(t, reg, "speed")
}

func TestTableMergeDuplicateDelete(t *testing.T) {
	tab, speed := table(t)
	orig := tab.Bytes()
	if err := tab.Set([]int{0, 2}, speed, schema.Float32(5), true); err != nil {
		t.Fatal(err)
	}
	if err := tab.Set([]int{0, 2}, speed, schema.Float32(6), false); err != nil {
		t.Fatal(err)
	}
	if err := tab.Set([]int{0}, speed, schema.Float32(7), false); err != nil {
		t.Fatal(err)
	}
	if err := tab.Undo(); err != nil || tab.Value(0, speed).AsFloat() != 6 {
		t.Fatalf("a different row set must not merge: %v %v", err, tab.Value(0, speed).AsFloat())
	}
	if err := tab.Undo(); err != nil || !bytes.Equal(tab.Bytes(), orig) {
		t.Fatalf("the merged drag must undo in one step:\n%s", tab.Bytes())
	}
	made, err := tab.Duplicate([]int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range tab.Rows {
		if seen[r.ID] {
			t.Fatalf("duplicate id %s", r.ID)
		}
		seen[r.ID] = true
	}
	if len(made) != 2 || !strings.HasPrefix(tab.Rows[made[0]].ID, "x_copy") || tab.Rows[made[0]].ID == "x_copy" {
		t.Fatalf("duplicates %v", made)
	}
	if err := tab.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := tab.Delete([]int{0, 2}); err != nil || len(tab.Rows) != 1 {
		t.Fatalf("delete: %v, %d rows", err, len(tab.Rows))
	}
	if err := tab.Undo(); err != nil || !bytes.Equal(tab.Bytes(), orig) {
		t.Fatalf("undo of delete must restore the order:\n%s", tab.Bytes())
	}
	if err := tab.Rename(0, "y"); err == nil {
		t.Fatal("rename onto a taken id was accepted")
	}
	csv, err := tab.ExportCSV()
	if err != nil {
		t.Fatal(err)
	}
	if err := tab.Set([]int{1}, speed, schema.Float32(9), true); err != nil {
		t.Fatal(err)
	}
	mid := tab.Bytes()
	if err := tab.ImportCSV(csv); err != nil {
		t.Fatal(err)
	}
	if err := tab.Undo(); err != nil || !bytes.Equal(tab.Bytes(), mid) {
		t.Fatalf("undo of a CSV import:\n%s", tab.Bytes())
	}
}
