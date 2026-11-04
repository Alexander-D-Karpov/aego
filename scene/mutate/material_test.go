package mutate_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"aego/core/ids"
	"aego/format/aetext"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene/mutate"
)

func materialDocument(t *testing.T, params string) (*mutate.MaterialDocument, *material.Shader) {
	t.Helper()
	id := ids.AssetID{Lo: 1}
	sh := &material.Shader{ID: id, Name: "fixture", Keywords: []string{"A", "B"}, Variants: []material.Variant{{Mask: 0}, {Mask: 1}, {Mask: 2}}, Schema: material.NewSchema([]shader.Param{
		{Name: "Speed", Kind: shader.KindFloat, Default: [4]float32{1}, Offset: 0},
		{Name: "Count", Kind: shader.KindInt, Offset: 4},
		{Name: "Enabled", Kind: shader.KindBool, Offset: 8},
		{Name: "NewName", Kind: shader.KindFloat, Offset: 12, Former: []string{"OldName"}},
	}, 16, 0)}
	src := `material version=1 shader=asset("` + id.String() + `") future="keep" { params { Removed={ nested=[1, 2, 3] } ` + params + ` } extension { keep=true } }`
	doc, err := mutate.LoadMaterial([]byte(src), "test.aemat", func(got ids.AssetID) (*material.Shader, bool) { return sh, got == id })
	if err != nil {
		t.Fatal(err)
	}
	return doc, sh
}

func TestMaterialMutationAtomicHistoryAndExactInteger(t *testing.T) {
	doc, _ := materialDocument(t, "")
	before := doc.Bytes()
	result, err := doc.Apply(mutate.Tx{Label: "Change material", Ops: []mutate.Op{
		mutate.MaterialSet("Speed", "1"),
		mutate.MaterialSet("Count", "2147483647"),
		mutate.MaterialSetValue("Enabled", aetext.Bool(true)),
		mutate.MaterialVariants([]string{"A"}),
		mutate.MaterialBlend("additive"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := doc.Doc().Value("Speed"); !ok || v.AsFloat() != 1 {
		t.Fatal("explicit equal-to-default value became inherited")
	}
	if v, ok := doc.Doc().Value("Count"); !ok || v.Int != math.MaxInt32 {
		t.Fatal("mutation rounded an int32 through float32")
	}
	after := doc.Bytes()
	for _, fragment := range []string{"Removed", "nested", "future", "extension"} {
		if !strings.Contains(string(after), fragment) {
			t.Fatalf("lost unknown %s", fragment)
		}
	}
	if _, err := doc.Apply(result.Inverse); err != nil || !bytes.Equal(before, doc.Bytes()) {
		t.Fatalf("undo failed: %v", err)
	}
	if _, err := doc.Apply(result.Redo); err != nil || !bytes.Equal(after, doc.Bytes()) {
		t.Fatalf("redo failed: %v", err)
	}
	reset, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.MaterialReset("Speed")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Doc().Value("Speed"); ok {
		t.Fatal("reset did not restore inheritance")
	}
	if _, err := doc.Apply(reset.Inverse); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Doc().Value("Speed"); !ok {
		t.Fatal("undo reset did not restore explicit flag")
	}

	for _, bad := range []mutate.Op{mutate.MaterialSet("Count", "1.5"), mutate.MaterialSet("Enabled", "1"), mutate.MaterialVariants([]string{"A", "B"}), mutate.MaterialVariants([]string{"NOPE"}), mutate.MaterialShader(ids.AssetID{Lo: 999}.String())} {
		stable := doc.Bytes()
		if _, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.MaterialSet("Speed", "9"), bad}}); err == nil {
			t.Fatalf("invalid atomic transaction accepted: %+v", bad)
		}
		if !bytes.Equal(stable, doc.Bytes()) {
			t.Fatal("failed batch kept an earlier operation")
		}
	}
}

func TestMaterialDryRunAndTransactionEncoding(t *testing.T) {
	doc, _ := materialDocument(t, "Count=2")
	before := doc.Bytes()
	tx := mutate.Tx{Label: "preview", Ops: []mutate.Op{mutate.MaterialSetValue("Count", aetext.Int(math.MinInt32)), mutate.MaterialSet("Speed", "2.5")}}
	encoded, err := tx.Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mutate.ParseTx(encoded, "tx.aetext")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := doc.DryRun(parsed)
	if err != nil || !preview.DryRun || len(preview.Changes) != 2 || !preview.Inverse.Empty() || !preview.Redo.Empty() {
		t.Fatalf("dry run %+v %v", preview, err)
	}
	if !bytes.Equal(before, doc.Bytes()) {
		t.Fatal("dry run changed source or explicit flags")
	}
	if _, err := doc.Apply(parsed); err != nil {
		t.Fatal(err)
	}
	if v, _ := doc.Doc().Value("Count"); v.Int != math.MinInt32 {
		t.Fatal("transaction encoding lost exact integer")
	}
}

func TestMaterialMutationsRespectFormerNames(t *testing.T) {
	doc, _ := materialDocument(t, "OldName=0.5")
	if _, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.MaterialReset("NewName")}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Doc().Value("OldName"); ok {
		t.Fatal("reset left the effective alias explicit")
	}
	doc, _ = materialDocument(t, "OldName=0.5")
	if _, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.MaterialSet("NewName", "0.75")}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Doc().Value("OldName"); ok {
		t.Fatal("canonical set left a conflicting alias")
	}
	if v, _ := doc.Doc().Value("NewName"); v.AsFloat() != 0.75 {
		t.Fatal("canonical value was not written")
	}
}

func TestMaterialUndoKeepsFormerlyValidUnknownState(t *testing.T) {
	doc, sh := materialDocument(t, "Speed=1")
	before := doc.Bytes()
	result, err := doc.Apply(mutate.Tx{Ops: []mutate.Op{mutate.MaterialSet("Speed", "2")}})
	if err != nil {
		t.Fatal(err)
	}
	sh.Schema = material.NewSchema([]shader.Param{{Name: "Speed", Kind: shader.KindVec2}}, 16, 0)
	undo, err := doc.Apply(result.Inverse)
	if err != nil || !bytes.Equal(before, doc.Bytes()) {
		t.Fatalf("undo dropped incompatible semantic data: %v", err)
	}
	errorReported := false
	for _, p := range undo.Problems {
		errorReported = errorReported || p.Error
	}
	if !errorReported {
		t.Fatal("incompatible restored value lost its diagnostic")
	}
}
