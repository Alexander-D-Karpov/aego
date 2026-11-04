package loader

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/shaderx"
	"aego/vfs"
)

func writeMaterialFixture(t *testing.T, files vfs.FS, path string, data []byte) {
	t.Helper()
	if err := files.WriteFile(path, data); err != nil {
		t.Fatal(err)
	}
}

func meshMaterialShader(t *testing.T) []byte {
	t.Helper()
	u, ds := shaderx.Parse([]byte("#pragma aego abi 1\n#pragma domain mesh2d\n#pragma material\nuniform float Value;\n#pragma end\nvoid fragment(){aego_out=vec4(Value);}\n"), "mesh.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	return asset.EncodeShader(asset.BuildShader(u))
}

func TestNewInvalidMaterialRevisionCancelsQueuedCandidate(t *testing.T) {
	for _, switched := range []bool{false, true} {
		label := "same-shader"
		if switched {
			label = "changed-shader"
		}
		for _, invalid := range []string{"type", "missing-shader", "domain", "corrupt-artifact"} {
			t.Run(label+"/"+invalid, func(t *testing.T) {
				l, c, files, se, me, m := materialFixture(t, "uniform float Value; // @default(1)", "Value=4")
				old := m.Shader()
				other := asset.Entry{ID: ids.AssetID{Lo: 103}, Name: "other", Kind: asset.ArtShader, Artifacts: []asset.Artifact{{Kind: asset.ArtShader, Path: "cache://other.aesh"}}}
				mesh := asset.Entry{ID: ids.AssetID{Lo: 104}, Name: "mesh", Kind: asset.ArtShader, Artifacts: []asset.Artifact{{Kind: asset.ArtShader, Path: "cache://mesh.aesh"}}}
				writeMaterialFixture(t, files, "cache://other.aesh", loaderShader(t, "uniform float Value; // @default(2)"))
				writeMaterialFixture(t, files, "cache://mesh.aesh", meshMaterialShader(t))
				if err := l.Swap([]asset.Entry{other, mesh}, nil); err != nil {
					t.Fatal(err)
				}
				target := se.ID
				if switched {
					target = other.ID
				}
				writeMaterialFixture(t, files, "cache://reload.aemt", loaderMaterial(t, target, "Value=8"))
				if err := l.Swap([]asset.Entry{me}, nil); err != nil {
					t.Fatal(err)
				}
				if !c.Pending(m) {
					t.Fatal("valid revision was not queued")
				}
				var bad []byte
				switch invalid {
				case "type":
					bad = loaderMaterial(t, target, "Value=[1,2]")
				case "missing-shader":
					bad = loaderMaterial(t, ids.AssetID{Lo: 999}, "Value=9")
				case "domain":
					bad = loaderMaterial(t, mesh.ID, "Value=9")
				case "corrupt-artifact":
					bad = []byte("truncated AEMT")
				}
				writeMaterialFixture(t, files, "cache://reload.aemt", bad)
				err := l.Swap([]asset.Entry{me}, nil)
				if (err != nil) != (invalid == "corrupt-artifact") {
					t.Fatalf("unexpected artifact preparation result: %v", err)
				}
				c.Begin()
				c.End()
				v, _ := m.Get("Value")
				if m.Shader() != old || v[0] != 4 || l.mat.uses[me.ID] != se.ID || c.Pending(m) {
					t.Fatalf("older success survived newer invalid source: shader=%s use=%s value=%v pending=%v", m.Shader().ID, l.mat.uses[me.ID], v[0], c.Pending(m))
				}
			})
		}
	}
}

func TestTextureRefreshPreservesQueuedMaterialSource(t *testing.T) {
	decl := "uniform float Value; // @default(1)\nuniform sampler2D Noise;"
	l, c, files, _, me, m := materialFixture(t, decl, "Value=4")
	other := asset.Entry{ID: ids.AssetID{Lo: 103}, Name: "other", Kind: asset.ArtShader, Artifacts: []asset.Artifact{{Kind: asset.ArtShader, Path: "cache://other.aesh"}}}
	tex := asset.Entry{ID: ids.AssetID{Lo: 104}, Name: "noise", Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://noise.aetex"}}}
	writeMaterialFixture(t, files, "cache://other.aesh", loaderShader(t, decl))
	writeMaterialFixture(t, files, "cache://noise.aetex", textureBytes(2, 2, mathx.Color{R: 1, A: 1}))
	if err := l.Swap([]asset.Entry{other, tex}, nil); err != nil {
		t.Fatal(err)
	}
	writeMaterialFixture(t, files, "cache://reload.aemt", loaderMaterial(t, other.ID, "Value=8 Noise=asset(\""+tex.ID.String()+"\")"))
	if err := l.Swap([]asset.Entry{me}, nil); err != nil {
		t.Fatal(err)
	}
	writeMaterialFixture(t, files, "cache://noise.aetex", textureBytes(2, 2, mathx.Color{G: 1, A: 1}))
	if err := l.Swap([]asset.Entry{tex}, nil); err != nil {
		t.Fatal(err)
	}
	c.Begin()
	c.End()
	v, _ := m.Get("Value")
	if m.Shader().ID != other.ID || l.mat.uses[me.ID] != m.Shader().ID || v[0] != 8 {
		t.Fatalf("texture refresh discarded pending source: shader=%s use=%s value=%v", m.Shader().ID, l.mat.uses[me.ID], v[0])
	}
	current, ok := l.Texture(tex.ID)
	if !ok || !m.UsesTexture(current.Texture) {
		t.Fatal("committed material retained the retired texture revision")
	}
}

func TestMaterialSwitchUsesShaderRevisionFromSameSwap(t *testing.T) {
	l, c, files, _, me, m := materialFixture(t, "uniform float Value; // @default(1)", "Value=4")
	other := asset.Entry{ID: ids.AssetID{Lo: 103}, Name: "other", Kind: asset.ArtShader, Artifacts: []asset.Artifact{{Kind: asset.ArtShader, Path: "cache://other.aesh"}}}
	writeMaterialFixture(t, files, "cache://other.aesh", loaderShader(t, "uniform float Value; // @default(2)"))
	if err := l.Swap([]asset.Entry{other}, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := l.Shader(other.ID); !ok {
		t.Fatal("other shader did not load")
	}
	writeMaterialFixture(t, files, "cache://other.aesh", loaderShader(t, "uniform float Value; // @default(3)\nuniform float Added; // @default(9)"))
	writeMaterialFixture(t, files, "cache://reload.aemt", loaderMaterial(t, other.ID, "Value=8"))
	if err := l.Swap([]asset.Entry{other, me}, nil); err != nil {
		t.Fatal(err)
	}
	c.Begin()
	c.End()
	active, ok := l.Shader(other.ID)
	added, present := m.Get("Added")
	if !ok || m.Shader() != active || !present || added[0] != 9 {
		t.Fatalf("material attached to old shader revision: live=%p registered=%p added=%v present=%v", m.Shader(), active, added, present)
	}
}
