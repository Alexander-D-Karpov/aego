package dataasset

import (
	"testing"

	"aego/core/ids"
	"aego/data"
	"aego/engine/module"
	"aego/format/aetext"
	"aego/schema"
)

type stats struct {
	Speed float32 `aego:"speed,min=0"`
}

type source map[ids.AssetID][]byte

func (s source) Data(id ids.AssetID) ([]byte, bool)  { b, ok := s[id]; return b, ok }
func (s source) Table(id ids.AssetID) ([]byte, bool) { b, ok := s[id]; return b, ok }

func check(t *testing.T, reg *schema.Registry, src source, id ids.AssetID, partial bool) []string {
	t.Helper()
	c := &module.AssetCheck{ID: id, Path: "x.aedata", Kind: data.KindData, Source: src[id], Schema: reg, Data: src, Partial: partial}
	var codes []string
	for _, d := range checkRecord(c) {
		codes = append(codes, d.Code)
	}
	return codes
}

func TestRecordDiagnostics(t *testing.T) {
	reg := schema.NewRegistry()
	if _, err := schema.RegisterData[stats](reg, schema.Name("test.Stats")); err != nil {
		t.Fatal(err)
	}
	reg.Freeze()
	a, b, c := ids.AssetID{Lo: 1}, ids.AssetID{Lo: 2}, ids.AssetID{Lo: 3}
	src := source{
		a: data.WriteDoc(&data.Doc{Type: "test.Stats", Fields: []aetext.Field{{Name: "speed", Value: aetext.Float32(-1)}}}),
		b: data.WriteDoc(&data.Doc{Type: "test.Stats", Base: b}),
		c: data.WriteDoc(&data.Doc{Type: "game.Missing"}),
	}
	if got := check(t, reg, src, a, false); len(got) != 1 || got[0] != data.CodeRange {
		t.Fatalf("range: %v", got)
	}
	if got := check(t, reg, src, b, false); len(got) != 1 || got[0] != codeBroken {
		t.Fatalf("cycle: %v", got)
	}
	if got := check(t, reg, src, c, false); len(got) != 1 || got[0] != codeType {
		t.Fatalf("unknown type: %v", got)
	}
	if got := check(t, reg, src, c, true); len(got) != 0 {
		t.Fatalf("a partial check must skip unknown types: %v", got)
	}
}
