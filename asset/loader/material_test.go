package loader

import (
	"fmt"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/shaderx"
	"aego/vfs"
)

func loaderShader(t *testing.T, declarations string) []byte {
	t.Helper()
	u, ds := shaderx.Parse([]byte("#pragma aego abi 1\n#pragma material\n"+declarations+"\n#pragma end\nvoid fragment(){aego_out=vec4(1.);}\n"), "reload.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	return asset.EncodeShader(asset.BuildShader(u))
}

func loaderMaterial(t *testing.T, sid ids.AssetID, values string) []byte {
	t.Helper()
	doc, err := asset.ParseMaterial([]byte(fmt.Sprintf("material version=1 shader=asset(%q) { params { %s } }", sid.String(), values)), "reload.aemat")
	if err != nil {
		t.Fatal(err)
	}
	return asset.EncodeMaterial(doc)
}

func materialFixture(t *testing.T, declarations, values string) (*Loader, *material.Cache, vfs.FS, asset.Entry, asset.Entry, *material.Material) {
	t.Helper()
	l, dev := newLoader(t)
	c := material.NewCache(dev, false, func(shader.Domain, int, shader.Blend) gpu.PipelineDesc {
		return gpu.PipelineDesc{Primitive: gpu.PrimTriangleStrip}
	})
	l.SetMaterials(c)
	t.Cleanup(func() { l.Close(); c.Close(); dev.Close() })
	se := asset.Entry{ID: ids.AssetID{Lo: 101}, Name: "reload", Kind: asset.ArtShader, Artifacts: []asset.Artifact{{Kind: asset.ArtShader, Path: "cache://reload.aesh"}}}
	me := asset.Entry{ID: ids.AssetID{Lo: 102}, Name: "material", Kind: asset.ArtMaterial, Artifacts: []asset.Artifact{{Kind: asset.ArtMaterial, Path: "cache://reload.aemt"}}}
	fs := newFS(t, map[string][]byte{"reload.aesh": loaderShader(t, declarations), "reload.aemt": loaderMaterial(t, se.ID, values)})
	idx := asset.NewIndex()
	idx.Put(se)
	idx.Put(me)
	if err := l.Mount(idx, fs); err != nil {
		t.Fatal(err)
	}
	m, ok := l.Material(me.ID)
	if !ok {
		t.Fatal("initial material did not load")
	}
	return l, c, fs, se, me, m
}

func TestBuiltinMaterialsLoadThroughPreparedRuntimeArtifacts(t *testing.T) {
	l, _ := newLoader(t)
	for _, name := range asset.BuiltinMaterialNames() {
		id, _ := asset.BuiltinMaterialNamed(name)
		if err := l.Preload(id); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m, ok := l.Material(id)
		if !ok || m.ID != id || m.Shader().Schema == nil {
			t.Fatalf("%s: incomplete runtime definition", name)
		}
		if again, _ := l.Material(id); again != m {
			t.Fatal("material identity changed on a second lookup")
		}
	}
	for _, name := range []string{"Lit", "LitMesh2D"} {
		id, exists := asset.BuiltinMaterialNamed(name)
		m, ok := l.Material(id)
		if !exists || !ok || m.Shader().Features&shader.FeatureLighting == 0 {
			t.Fatalf("%s must expose a real lighting shader, not an Unlit alias", name)
		}
		p, ok := m.Shader().Schema.Param("NormalMap")
		if !ok || !p.Normal || p.Slot != shader.TexNormal {
			t.Fatalf("%s has no declared normal-map binding", name)
		}
	}
}

func TestLoaderPublishesProgramSchemaAndValuesOnlyAtCommit(t *testing.T) {
	l, c, fs, se, me, m := materialFixture(t, "uniform float Value; // @default(1)\nuniform float Inherited; // @default(1)", "Value=1 Retained=7")
	old := m.Shader()
	before := l.Generation()
	notifications := 0
	l.Subscribe(func(asset.Reloaded) { notifications++ })
	if err := fs.WriteFile("cache://reload.aesh", loaderShader(t, "uniform float Value; // @default(2)\nuniform float Inherited; // @default(3)\nuniform float Retained; // @default(0)")); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap([]asset.Entry{se}, nil); err != nil {
		t.Fatal(err)
	}
	if m.Shader() != old || l.Generation() != before || notifications != 0 {
		t.Fatal("prepared source was published before the frame boundary")
	}
	if s, _ := l.Schemas().Get(se.ID); s != old.Schema {
		t.Fatal("schema advanced before program")
	}
	c.Begin()
	c.End()
	if m.Shader() == old || l.Generation() == before || notifications != 1 {
		t.Fatal("successful candidate was not published exactly once")
	}
	if again, _ := l.Material(me.ID); again != m {
		t.Fatal("reload replaced the shared material identity")
	}
	for name, want := range map[string]float32{"Value": 1, "Inherited": 3, "Retained": 7} {
		v, ok := m.Get(name)
		if !ok || v[0] != want {
			t.Fatalf("%s: %v, want %v", name, v, want)
		}
	}
	if !m.Explicit("Value") || !m.Explicit("Retained") || m.Explicit("Inherited") {
		t.Fatal("explicit/inherited provenance changed")
	}
}

func TestLoaderRejectsNewerInvalidSourceWithoutCommittingQueuedSuccess(t *testing.T) {
	l, c, fs, se, _, m := materialFixture(t, "uniform float Value; // @default(1)", "")
	old := m.Shader()
	fs.WriteFile("cache://reload.aesh", loaderShader(t, "uniform float Value; // @default(2)"))
	if err := l.Swap([]asset.Entry{se}, nil); err != nil {
		t.Fatal(err)
	}
	l.RejectSource(se.ID)
	c.Begin()
	c.End()
	if m.Shader() != old {
		t.Fatal("superseded candidate replaced last-good")
	}
	fs.WriteFile("cache://reload.aesh", loaderShader(t, "uniform float Value; // @default(3)"))
	if err := l.Swap([]asset.Entry{se}, nil); err != nil {
		t.Fatal(err)
	}
	c.Begin()
	c.End()
	if v, _ := m.Get("Value"); v[0] != 3 {
		t.Fatal("fixed current revision did not commit")
	}
}

func TestIncompatibleMaterialEditKeepsDefinitionAndDeletedShaderCanRecover(t *testing.T) {
	l, c, fs, se, me, m := materialFixture(t, "uniform float Value; // @default(1)", "Value=4")
	old := m.Shader()
	fs.WriteFile("cache://reload.aemt", loaderMaterial(t, se.ID, "Value=[1,2]"))
	if err := l.Swap([]asset.Entry{me}, nil); err != nil {
		t.Fatal(err)
	}
	c.Begin()
	c.End()
	if v, _ := m.Get("Value"); v[0] != 4 || m.Shader() != old {
		t.Fatal("invalid material edit changed last-good")
	}
	fs.WriteFile("cache://reload.aemt", loaderMaterial(t, se.ID, "Value=4"))
	l.Swap([]asset.Entry{me}, nil)
	c.Begin()
	c.End()
	l.Drop(se.ID)
	fs.WriteFile("cache://reload.aesh", loaderShader(t, "uniform float Value; // @default(2)\nuniform float Added; // @default(9)"))
	if err := l.Swap([]asset.Entry{se}, nil); err != nil {
		t.Fatal(err)
	}
	c.Begin()
	c.End()
	if v, ok := m.Get("Added"); !ok || v[0] != 9 {
		t.Fatal("a returned source did not recover its original consumers")
	}
	if v, _ := m.Get("Value"); v[0] != 4 {
		t.Fatal("source deletion lost explicit material value")
	}
}

func TestCorruptMaterialSwapKeepsArtifactMount(t *testing.T) {
	l, _, fs, _, me, m := materialFixture(t, "uniform float Value; // @default(1)", "Value=4")
	bad := newFS(t, map[string][]byte{"reload.aemt": []byte("truncated")})
	before := l.Generation()
	if err := l.Swap([]asset.Entry{me}, bad); err == nil {
		t.Fatal("corrupt AEMT accepted")
	}
	if l.Artifacts() != fs || l.Generation() != before {
		t.Fatal("failed swap changed the artifact mount or revision")
	}
	if live, _ := l.Material(me.ID); live != m {
		t.Fatal("failed swap replaced the runtime material")
	}
}
