//go:build !aego_runtime

package docs

import (
	"strings"
	"testing"

	"aego/script/abi"
	"aego/script/bind/manifest"
)

func TestDocumentationParameterNamesDoNotMutateBindings(t *testing.T) {
	r := &Registry{}
	e := Entry{ID: "render2d.PostProcess.SetFloat", Kind: Method,
		Signature: "func (PostProcess) SetFloat(p0 string, p1 float)", Params: []Param{{"p0", "string"}, {"p1", "float"}}}
	r.documentParams(&e, []string{"key", "value"})
	if e.Signature != "func (PostProcess) SetFloat(key string, value float)" || e.Params[0].Name != "key" || len(r.problems) != 0 {
		t.Fatalf("invalid documentation override: %+v %+v", e, r.problems)
	}
	for _, names := range [][]string{{"key"}, {"key", "key"}, {"bad-name", "value"}} {
		r.problems = nil
		before := e.Signature
		r.documentParams(&e, names)
		if len(r.problems) != 1 || !r.problems[0].Error || e.Signature != before {
			t.Fatalf("invalid override accepted: %v", names)
		}
	}
}

func TestPublicValueTypesExposeTheirMembers(t *testing.T) {
	m := &manifest.Manifest{Modules: []manifest.Module{{Path: "aego/material"}}, Structs: []manifest.Struct{{
		Module: "aego/material", Name: "Vec4", Doc: "Four floating point lanes.",
		Fields: []abi.StructField{{Name: "X", Type: abi.T(abi.Float)}, {Name: "W", Type: abi.T(abi.Float)}},
	}}}
	r := Assemble(Sources{Bindings: m})
	if _, ok := r.Get("aego/material.Vec4"); !ok {
		t.Fatal("value structure omitted")
	}
	if field, ok := r.Get("aego/material.Vec4.X"); !ok || !strings.Contains(field.Signature, "float") {
		t.Fatal("value member omitted", field)
	}
	if got := publicType("[]struct:aego/physics.RayHit"); got != "[]aego/physics.RayHit" {
		t.Fatal(got)
	}
}
