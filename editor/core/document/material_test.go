package document

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/code"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene/mutate"
)

func TestMaterialMutationSharesSourceHistoryAndPreservesUnknown(t *testing.T) {
	const source = `material version=1 shader=asset("00000000000000000000000000000001") {
  params { OldField=0.625 }
  future version=4 { note="preserve" }
}`
	d := &ScriptDocument{Path: "assets/flash.aemat", Code: code.New([]byte(source))}
	sh := &material.Shader{Schema: material.NewSchema([]shader.Param{{Name: "Flash", Kind: shader.KindFloat, Instance: true, Default: [4]float32{.2}, Lane: 0}, {Name: "Index", Kind: shader.KindInt, Offset: 0}}, 16, 1), Variants: []material.Variant{{}}}
	resolve := func(ids.AssetID) (*material.Shader, bool) { return sh, true }
	before := append([]byte(nil), d.Code.Bytes()...)
	for i, v := range []string{"0.3", "0.6", "0.9"} {
		if _, err := ApplyMaterialMutation(d, resolve, mutate.Tx{Ops: []mutate.Op{mutate.MaterialSet("Flash", v)}}, i > 0); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(string(d.Code.Bytes()), "preserve") || !strings.Contains(string(d.Code.Bytes()), "OldField") {
		t.Fatal("unknown semantic data disappeared")
	}
	parsed, err := asset.ParseMaterial(d.Code.Bytes(), d.Path)
	if err != nil {
		t.Fatal(err)
	}
	v, explicit := parsed.Value("Flash")
	if !explicit || math.Abs(v.AsFloat()-.9) > 1e-6 {
		t.Fatalf("explicit parameter lost: %v", v)
	}
	if !d.Code.Undo() || !bytes.Equal(before, d.Code.Bytes()) || d.Code.CanUndo() {
		t.Fatal("material slider did not undo exactly to original source")
	}
	if !d.Code.Redo() {
		t.Fatal("redo")
	}
	last := append([]byte(nil), d.Code.Bytes()...)
	if _, err := ApplyMaterialMutation(d, resolve, mutate.Tx{Ops: []mutate.Op{mutate.MaterialSet("Flash", "true")}}, false); err == nil {
		t.Fatal("incompatible type accepted")
	}
	if !bytes.Equal(last, d.Code.Bytes()) {
		t.Fatal("failed semantic mutation changed dirty document")
	}
	if _, err := ApplyMaterialMutation(d, resolve, mutate.Tx{Ops: []mutate.Op{mutate.MaterialReset("Flash")}}, false); err != nil {
		t.Fatal(err)
	}
	parsed, err = asset.ParseMaterial(d.Code.Bytes(), d.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, explicit = parsed.Value("Flash"); explicit {
		t.Fatal("reset still explicit")
	}
}
