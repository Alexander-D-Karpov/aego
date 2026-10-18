package data

import (
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/rng"
	"aego/format/aetext"
	"aego/schema"
)

type stats struct {
	Speed float32                  `aego:"speed,min=0,max=10"`
	HP    int32                    `aego:"hp"`
	Name  string                   `aego:"name"`
	Icon  asset.Ref[asset.Texture] `aego:"icon"`
}

type source struct {
	data, tables map[ids.AssetID][]byte
}

func (s source) Data(id ids.AssetID) ([]byte, bool)  { b, ok := s.data[id]; return b, ok }
func (s source) Table(id ids.AssetID) ([]byte, bool) { b, ok := s.tables[id]; return b, ok }

func registry(t *testing.T) *schema.Registry {
	t.Helper()
	reg := schema.NewRegistry()
	if _, err := schema.RegisterData[stats](reg, schema.Name("test.Stats")); err != nil {
		t.Fatal(err)
	}
	if err := RegisterWeighted[stats](reg, "test.WeightedStats"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Freeze(); err != nil {
		t.Fatal(err)
	}
	return reg
}

func id(n uint64) ids.AssetID {
	return ids.AssetID{Lo: n}
}

func doc(base ids.AssetID, fields ...aetext.Field) []byte {
	return WriteDoc(&Doc{Type: "test.Stats", Base: base, Fields: fields})
}

func f(name string, v aetext.Value) aetext.Field {
	return aetext.Field{Name: name, Value: v}
}

func TestBaseAndOverrides(t *testing.T) {
	reg := registry(t)
	src := source{data: map[ids.AssetID][]byte{
		id(1): doc(ids.AssetID{}, f("speed", aetext.Float32(2)), f("hp", aetext.Int(3)), f("name", aetext.String("goblin"))),
		id(2): doc(id(1), f("speed", aetext.Float32(5))),
	}}
	l := NewLibrary(reg, src)
	fast, err := Get[stats](l, id(2))
	if err != nil {
		t.Fatal(err)
	}
	if fast.Speed != 5 || fast.HP != 3 || fast.Name != "goblin" {
		t.Fatalf("variant %+v", fast)
	}
	base, _ := Get[stats](l, id(1))
	if base.Speed != 2 {
		t.Fatal("a variant must not change its base")
	}
}

func TestBaseCycleAndMissing(t *testing.T) {
	reg := registry(t)
	src := source{data: map[ids.AssetID][]byte{
		id(1): doc(id(2)),
		id(2): doc(id(1)),
		id(3): doc(id(9)),
	}}
	l := NewLibrary(reg, src)
	if _, err := l.Record(id(1)); err == nil || !strings.Contains(err.Error(), "refers back") {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := l.Record(id(3)); err == nil {
		t.Fatal("a missing base must fail")
	}
}

func TestUnknownFieldsAreIssues(t *testing.T) {
	reg := registry(t)
	src := source{data: map[ids.AssetID][]byte{id(1): doc(ids.AssetID{}, f("nope", aetext.Int(1)), f("speed", aetext.Float32(-1)))}}
	rec, err := NewLibrary(reg, src).Record(id(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Issues) != 1 {
		t.Fatalf("issues %v", rec.Issues)
	}
	is := Validate(rec.Type, rec.Ptr, func(ids.AssetID) bool { return false })
	if len(is) != 1 || is[0].Code != CodeRange {
		t.Fatalf("validate %v", is)
	}
}

func TestTablesAndCSV(t *testing.T) {
	reg := registry(t)
	tbl := &TableDoc{Type: "test.Stats", Rows: []TableRow{
		{ID: "goblin", Fields: []aetext.Field{f("speed", aetext.Float32(2)), f("hp", aetext.Int(3)), f("name", aetext.String("Gob, the small"))}},
		{ID: "orc", Fields: []aetext.Field{f("hp", aetext.Int(8))}},
	}}
	src := source{tables: map[ids.AssetID][]byte{id(5): WriteTable(tbl)}}
	l := NewLibrary(reg, src)
	orc, ok, err := Row[stats](l, id(5), "orc")
	if err != nil || !ok || orc.HP != 8 || orc.Speed != 0 {
		t.Fatalf("row %+v %v %v", orc, ok, err)
	}
	out, err := ExportCSV(reg, tbl)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ImportCSV(reg, &TableDoc{Type: "test.Stats"}, out)
	if err != nil {
		t.Fatal(err)
	}
	if string(WriteTable(back)) != string(WriteTable(tbl)) {
		t.Fatalf("CSV round trip:\n%s\n%s", WriteTable(tbl), WriteTable(back))
	}
	if _, err := ImportCSV(reg, &TableDoc{Type: "test.Stats"}, []byte("id,bogus\na,1\n")); err == nil {
		t.Fatal("unknown column accepted")
	}
}

func TestWeightedPickIsDeterministic(t *testing.T) {
	w := Weighted[stats]{Items: []stats{{Name: "a"}, {Name: "b"}}, Weights: []float32{1, 3}}
	count := func(seed uint64) int {
		r := rng.New(seed)
		n := 0
		for range 10000 {
			if p, _ := w.Pick(r); p.Name == "b" {
				n++
			}
		}
		return n
	}
	a, b := count(42), count(42)
	if a != b || a < 7200 || a > 7800 {
		t.Fatalf("picks %d %d", a, b)
	}
	empty := Weighted[stats]{Items: []stats{{}}, Weights: []float32{0}}
	if _, ok := empty.Pick(rng.New(1)); ok {
		t.Fatal("zero weights cannot pick")
	}
}

func TestRefKinds(t *testing.T) {
	var r Ref[stats]
	var tr TableRef[stats]
	if r.AssetKind() != KindData || tr.AssetKind() != KindTable {
		t.Fatal("kinds")
	}
}
