package shader_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/asset"
	matimp "aego/asset/importers/material"
	"aego/asset/importers/shader"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/null"
	"aego/project"
	"aego/render/material"
	rshader "aego/render/shader"
	"aego/render/shaderx"
	"aego/render/texture"
)

const shaderSrc = `#pragma aego abi 1
#pragma variant GLOW
#include "engine/sprite.glsl"
#include "waves.glsl"

#pragma material
uniform float Speed; // @default(1)
uniform vec4 Tint; // @color
#pragma end

void fragment() {
    vec4 c = aego_sample() * Tint;
#if GLOW
    c.rgb += 0.1;
#endif
    aego_out = c * wave(aego_time * Speed);
}
`

func write(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openProject(t *testing.T) (*project.Project, *pipeline.Pipeline, *shader.Importer) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Shaders", false)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.Root, "assets/shaders/include/waves.glsl", "float wave(float x) { return 0.75 + 0.25 * sin(x); }\n")
	write(t, p.Root, "assets/shaders/flash.aeshader", shaderSrc)
	reg := pipeline.NewImporterRegistry()
	si := shader.New()
	for _, imp := range []pipeline.Importer{si, shader.NewInclude(), matimp.New()} {
		if err := reg.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	b, err := pl.ImportAll()
	if err != nil || len(b.Failed) > 0 {
		t.Fatalf("import: %v %v", err, b.Failed)
	}
	shID, _ := pl.Lookup("assets/shaders/flash.aeshader")
	write(t, p.Root, "assets/flash.aemat", `material version=1 shader=asset("`+shID.String()+`") {
  variants=[GLOW]
  params {
    Speed=3
    Tint=#ff8000ff
    Unknown=1
  }
}
`)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if b, err := pl.ImportAll(); err != nil || len(b.Failed) > 0 {
		t.Fatalf("material import: %v %v", err, b.Failed)
	}
	return p, pl, si
}

func TestShaderAndMaterialThroughTheLoader(t *testing.T) {
	p, pl, _ := openProject(t)
	shID, _ := pl.Lookup("assets/shaders/flash.aeshader")
	incID, ok := pl.Lookup("assets/shaders/include/waves.glsl")
	if !ok {
		t.Fatal("the project include is not an asset")
	}
	if !slices.Contains(pl.Dependents(incID), shID) {
		t.Fatal("the shader does not depend on its include")
	}
	matID, _ := pl.Lookup("assets/flash.aemat")

	dev := null.New()
	l := loader.New(dev, texture.NewUploader(dev), log.Nop())
	if err := l.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	c := material.NewCache(dev, false, func(rshader.Domain, int, rshader.Blend) gpu.PipelineDesc { return gpu.PipelineDesc{} })
	l.SetMaterials(c)
	m, ok := l.Material(matID)
	if !ok {
		t.Fatal("the material does not load")
	}
	if m.Mask() != 1 || m.Shader().ID != shID {
		t.Fatalf("mask %b, shader %v", m.Mask(), m.Shader().ID)
	}
	if v, _ := m.Get("Speed"); v[0] != 3 {
		t.Fatalf("Speed = %v", v)
	}
	if v, _ := m.Get("Tint"); v[0] != 1 || v[3] != 1 || v[1] < 0.5 || v[1] > 0.51 {
		t.Fatalf("Tint = %v", v)
	}
	if _, ok := l.Schemas().Get(shID); !ok {
		t.Fatal("the shader's schema is not published")
	}

	old := m.Shader()
	m.Set("Speed", [4]float32{5})
	write(t, p.Root, "assets/shaders/include/waves.glsl", "float wave(float x) { return 0.5 + 0.5 * sin(x); }\n")
	b, err := pl.Reimport(append([]ids.AssetID{incID}, pl.Dependents(incID)...)...)
	if err != nil || len(b.Failed) > 0 {
		t.Fatalf("reimport: %v %v", err, b.Failed)
	}
	if err := l.Swap(b.Imported, nil); err != nil {
		t.Fatal(err)
	}
	f := dev.BeginFrame(8, 8)
	if !c.Bind(f.BeginPass(gpu.PassDesc{}), m) {
		t.Fatal("the material stopped drawing")
	}
	dev.EndFrame()
	if m.Shader() == old || m.Failed() {
		t.Fatal("the reimported shader did not reach the material")
	}
	if !strings.Contains(m.Shader().Variants[0].Sources[1], "0.5 + 0.5") {
		t.Fatal("the new include is not in the new shader")
	}
	if v, _ := m.Get("Speed"); v[0] != 5 {
		t.Fatalf("Speed lost on reload: %v", v)
	}
}

// A broken shader keeps its previous artifact (D393) and reports SHD diagnostics with the line.
func TestBrokenShaderKeepsTheLastGood(t *testing.T) {
	p, pl, si := openProject(t)
	shID, _ := pl.Lookup("assets/shaders/flash.aeshader")
	e, _ := pl.Index().Lookup(shID)
	before, _ := e.Artifact(asset.ArtShader)
	write(t, p.Root, "assets/shaders/flash.aeshader", strings.Replace(shaderSrc, "uniform float Speed;", "uniform mat3 Speed;", 1))
	b, err := pl.Reimport(shID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Failed[shID] == nil {
		t.Fatal("a broken shader imported")
	}
	e, _ = pl.Index().Lookup(shID)
	if after, ok := e.Artifact(asset.ArtShader); !ok || after != before {
		t.Fatal("the failed import replaced the last good artifact")
	}
	diags, _ := si.Diagnostics()
	if len(diags) != 1 || diags[0].Code != shaderx.SHD003 || diags[0].Line != 7 || diags[0].File != "assets/shaders/flash.aeshader" {
		t.Fatalf("diagnostics %+v", diags)
	}
	write(t, p.Root, "assets/shaders/flash.aeshader", shaderSrc)
	if b, _ := pl.Reimport(shID); len(b.Failed) > 0 {
		t.Fatal(b.Failed)
	}
	if diags, _ := si.Diagnostics(); len(diags) != 0 {
		t.Fatalf("diagnostics stay after the fix: %+v", diags)
	}
}

func TestArtifactRoundTrip(t *testing.T) {
	_, pl, _ := openProject(t)
	shID, _ := pl.Lookup("assets/shaders/flash.aeshader")
	e, _ := pl.Index().Lookup(shID)
	art, _ := e.Artifact(asset.ArtShader)
	data, err := pl.Artifacts().ReadFile(art.Path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := asset.DecodeShader(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(asset.EncodeShader(a), data) {
		t.Fatal("decode and encode do not round-trip")
	}
	if len(a.Variants) != 2 || len(a.Params) != 2 || a.Size != 32 || !slices.Equal(a.Keywords, []string{"GLOW"}) {
		t.Fatalf("variants %d params %d size %d keywords %v", len(a.Variants), len(a.Params), a.Size, a.Keywords)
	}
	if _, err := asset.DecodeShader(data[:len(data)/2]); err == nil {
		t.Fatal("a truncated artifact decoded")
	}
}
