package dataedit_test

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/data"
	"aego/editor/core/dataedit"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/schema"
	"aego/script/bind"
)

type stats struct {
	Speed  float32 `aego:"speed,min=0"`
	Damage int64   `aego:"damage"`
	Title  string  `aego:"title"`
}

type mod struct{}

func (mod) Info() module.Info {
	return module.Info{ID: "dataeditcase"}
}

func (mod) Register(r *module.Registry) error {
	return module.DataType[stats](r, schema.Name("test.Stats")).Err()
}

type store struct {
	docs   map[ids.AssetID][]byte
	tables map[ids.AssetID][]byte
}

func (s *store) Data(id ids.AssetID) ([]byte, bool) {
	b, ok := s.docs[id]
	return b, ok
}

func (s *store) Table(id ids.AssetID) ([]byte, bool) {
	b, ok := s.tables[id]
	return b, ok
}

func registry(t *testing.T) *schema.Registry {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Headless().Load(types, bind.New(), assembly.Options{}, mod{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types.Schema()
}

func field(t *testing.T, reg *schema.Registry, name string) *schema.FieldDescriptor {
	t.Helper()
	td, ok := reg.ByName("test.Stats")
	if !ok {
		t.Fatal("test.Stats is not registered")
	}
	fd, ok := td.Field(name)
	if !ok {
		t.Fatalf("test.Stats has no field %s", name)
	}
	return fd
}

var (
	baseID  = ids.AssetID{Hi: 1, Lo: 1}
	childID = ids.AssetID{Hi: 1, Lo: 2}
)

func baseStore() *store {
	return &store{docs: map[ids.AssetID][]byte{
		baseID: data.WriteDoc(&data.Doc{Type: "test.Stats", Fields: []aetext.Field{
			{Name: "speed", Value: aetext.Float32(2)},
			{Name: "damage", Value: aetext.Int(1)},
		}}),
	}}
}

func TestDocOverridesBase(t *testing.T) {
	reg := registry(t)
	lib := data.NewLibrary(reg, baseStore())
	d, err := dataedit.NewDoc(reg, lib, childID, "assets/fast.aedata", &data.Doc{Type: "test.Stats", Base: baseID})
	if err != nil {
		t.Fatal(err)
	}
	speed := field(t, reg, "speed")
	if got := d.Value(speed).AsFloat(); got != 2 {
		t.Fatalf("inherited speed = %v, want 2", got)
	}
	if d.Overridden("speed") || d.Dirty() {
		t.Fatal("a fresh child must not override or be dirty")
	}
	if err := d.Set(speed, schema.Float32(5), true); err != nil {
		t.Fatal(err)
	}
	if !d.Overridden("speed") || !d.Dirty() {
		t.Fatal("setting a field must override it")
	}
	if err := d.Set(speed, schema.Float32(2), true); err != nil {
		t.Fatal(err)
	}
	if d.Overridden("speed") {
		t.Fatal("a value equal to the base is not an override")
	}
	if err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := d.Value(speed).AsFloat(); got != 5 {
		t.Fatalf("undo: speed = %v, want 5", got)
	}
	if err := d.Revert("speed"); err != nil {
		t.Fatal(err)
	}
	if got := d.Value(speed).AsFloat(); got != 2 || d.Overridden("speed") {
		t.Fatalf("revert: speed = %v, overridden = %v", got, d.Overridden("speed"))
	}
	if err := d.Undo(); err != nil {
		t.Fatal(err)
	}
	out, err := data.ParseDoc(d.Bytes(), "fast")
	if err != nil {
		t.Fatal(err)
	}
	if out.Base != baseID || len(out.Fields) != 1 || out.Fields[0].Name != "speed" || out.Fields[0].Value.AsFloat() != 5 {
		t.Fatalf("saved document = %+v", out)
	}
}

func TestDocBaseCycle(t *testing.T) {
	reg := registry(t)
	src := baseStore()
	src.docs[childID] = data.WriteDoc(&data.Doc{Type: "test.Stats", Base: baseID})
	lib := data.NewLibrary(reg, src)
	d, err := dataedit.NewDoc(reg, lib, baseID, "assets/base.aedata", &data.Doc{Type: "test.Stats"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetBase(baseID); err == nil {
		t.Fatal("an asset must not become its own base")
	}
	if err := d.SetBase(childID); err == nil {
		t.Fatal("a base chain that comes back must be rejected")
	}
	if !d.Base().IsZero() || d.Dirty() {
		t.Fatal("a rejected base must leave the document unchanged")
	}
}

func TestTableMassEditAndCSV(t *testing.T) {
	reg := registry(t)
	row := func(id string, speed float32) data.TableRow {
		return data.TableRow{ID: id, Fields: []aetext.Field{{Name: "speed", Value: aetext.Float32(speed)}}}
	}
	doc := &data.TableDoc{Type: "test.Stats", Rows: []data.TableRow{row("goblin", 2), row("bat", 4), row("troll", 1)}}
	tab, err := dataedit.NewTable(reg, ids.AssetID{Hi: 2, Lo: 1}, "assets/enemies.aetable", doc)
	if err != nil {
		t.Fatal(err)
	}
	speed := field(t, reg, "speed")
	if err := tab.Set([]int{0, 2}, speed, schema.Float32(7), true); err != nil {
		t.Fatal(err)
	}
	for i, want := range []float64{7, 4, 7} {
		if got := tab.Value(i, speed).AsFloat(); got != want {
			t.Fatalf("row %d speed = %v, want %v", i, got, want)
		}
	}
	if err := tab.Rename(1, "goblin"); err == nil {
		t.Fatal("a duplicate row id must be rejected")
	}
	csv, err := tab.ExportCSV()
	if err != nil {
		t.Fatal(err)
	}
	if err := tab.Set([]int{1}, speed, schema.Float32(9), true); err != nil {
		t.Fatal(err)
	}
	if err := tab.ImportCSV(csv); err != nil {
		t.Fatal(err)
	}
	if got := tab.Value(1, speed).AsFloat(); got != 4 {
		t.Fatalf("after CSV import bat speed = %v, want 4", got)
	}
	tab.Undo()
	if got := tab.Value(1, speed).AsFloat(); got != 9 {
		t.Fatalf("undo import: bat speed = %v, want 9", got)
	}
	tab.Undo()
	if got := tab.Value(1, speed).AsFloat(); got != 4 {
		t.Fatalf("undo set: bat speed = %v, want 4", got)
	}
	back, err := data.ParseTable(tab.Bytes(), "enemies")
	if err != nil || len(back.Rows) != 3 {
		t.Fatalf("round trip: %v, %d rows", err, len(back.Rows))
	}
}
