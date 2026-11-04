package asset_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"slices"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/shaderx"
)

func TestShaderSurfaceChannelsVersionAndLegacyDecoding(t *testing.T) {
	a := makeShader(t, `#pragma aego abi 1
#pragma domain sprite
#pragma lighting forward2d
#pragma material
uniform sampler2D Normal; // @normal
uniform sampler2D Glow; // @emission
#pragma end
void fragment(){aego_out=vec4(1.0);}
`)
	encoded := asset.EncodeShader(a)
	if binary.LittleEndian.Uint16(encoded[4:6]) != 4 {
		t.Fatal("surface layout was silently written under an old version")
	}
	decoded, err := asset.DecodeShader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Params[1].Emission || decoded.Params[1].Slot != shader.TexEmission || decoded.Features&shader.FeatureSurfaceChannels == 0 {
		t.Fatal("emission ABI role did not round trip")
	}
	rehash := func(b []byte) { h := sha256.Sum256(b[:len(b)-32]); copy(b[len(b)-32:], h[:]) }
	legacy := bytes.Clone(encoded)
	binary.LittleEndian.PutUint16(legacy[4:6], 3)
	rehash(legacy)
	if _, err = asset.DecodeShader(legacy); err == nil {
		t.Fatal("AESH4-only channel data disguised as AESH3 was accepted")
	}
	plain := makeShader(t, "#pragma aego abi 1\nvoid fragment(){aego_out=vec4(1.0);}\n")
	legacy = asset.EncodeShader(plain)
	binary.LittleEndian.PutUint16(legacy[4:6], 3)
	rehash(legacy)
	if _, err = asset.DecodeShader(legacy); err != nil {
		t.Fatalf("legacy AESH3 stopped decoding: %v", err)
	}
	plain.Features = 0
	plain.LayoutHash = shader.LayoutHash(plain.Domain, plain.Params, plain.Size, plain.Lanes)
	legacy = asset.EncodeShader(plain)
	legacy = append(legacy[:12], legacy[16:]...) // AESH2 did not serialize feature flags.
	binary.LittleEndian.PutUint16(legacy[4:6], 2)
	rehash(legacy)
	if _, err = asset.DecodeShader(legacy); err != nil {
		t.Fatalf("legacy AESH2 stopped decoding: %v", err)
	}
}

const materialShaderID = "00000000000000000000000000000001"

func makeShader(t *testing.T, src string) *asset.ShaderArtifact {
	t.Helper()
	u, ds := shaderx.Parse([]byte(src), "test.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	return asset.BuildShader(u)
}

func mixedShader(t *testing.T) *asset.ShaderArtifact {
	return makeShader(t, `#pragma aego abi 1
#pragma variant A B
#pragma material
uniform float Speed; // @default(1)
uniform int Count; // @default(-2147483648)
uniform bool Active; // @default(true)
uniform vec4 Tint; // @color
uniform vec4 Raw;
uniform int InstanceCount; // @instance @default(2147483647)
uniform float NewName; // @former(OldName) @instance
#pragma end
void fragment(){ aego_out=Tint*Speed; }
`)
}

func TestMaterialDocumentBinaryAndSemanticRoundTrip(t *testing.T) {
	src := `material version=1 shader=asset("` + materialShaderID + `") futureAttr="retained" {
  futureField={ counter=4 }
  variants=[B, A]
  params {
    Speed=1
    Count=2147483647
    InstanceCount=-2147483648
    Active=true
    Tint=#80808080
    Raw=#80808080
    OldName=0.75
    Removed={ settings=[1, 2, 3] }
    RemovedTexture=asset("00000000000000000000000000000002")
  }
  future { data="retained" }
}
`
	d, err := asset.ParseMaterial([]byte(src), "test.aemat")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := d.Value("Count"); !ok || v.Int != math.MaxInt32 {
		t.Fatal("source int32 lost precision")
	}
	if err := d.SetValue("Speed", aetext.Float32(2)); err != nil {
		t.Fatal(err)
	}
	d.Blend, d.HasBlend = shader.BlendAdditive, true
	data := asset.EncodeMaterial(d)
	if string(data[:4]) != "AEMT" || bytes.HasPrefix(data, []byte("material")) {
		t.Fatal("runtime artifact contains source text")
	}
	got, err := asset.DecodeMaterial(data, "test.aemat")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, asset.EncodeMaterial(got)) {
		t.Fatal("binary round trip is unstable")
	}
	out := string(asset.MarshalMaterial(got))
	for _, text := range []string{"futureAttr", "futureField", "Removed", "RemovedTexture", "future", "blend=additive"} {
		if !strings.Contains(out, text) {
			t.Fatalf("lost %s: %s", text, out)
		}
	}
	if !got.Unset("Speed") {
		t.Fatal("explicit value could not be reset")
	}
	if _, ok := got.Value("Speed"); ok {
		t.Fatal("reset remained explicit")
	}
	if !slices.ContainsFunc(got.Deps(), func(id ids.AssetID) bool { return id.Lo == 2 }) {
		t.Fatal("unknown texture reference identity was lost")
	}

	sh := mixedShader(t).Runtime(ids.AssetID{Lo: 1}, "mixed")
	m, problems := got.Build("test", sh, func(ids.AssetID) (material.Tex, bool) { return material.Tex{}, false })
	if m == nil {
		t.Fatal(problems)
	}
	if v, ok := m.GetInt("Count"); !ok || v != math.MaxInt32 {
		t.Fatalf("integer = %v %v", v, ok)
	}
	if v, ok := m.GetInt("InstanceCount"); !ok || v != math.MinInt32 {
		t.Fatalf("instance integer = %v %v", v, ok)
	}
	if v, _ := m.Get("NewName"); v[0] != 0.75 {
		t.Fatal("former parameter alias was not resolved")
	}
	color, _ := m.Get("Tint")
	raw, _ := m.Get("Raw")
	if math.Abs(float64(color[0]-mathx.SRGBToLinear(128.0/255))) > 1e-6 || raw[0] != 128.0/255 || color[3] != 128.0/255 {
		t.Fatalf("color=%v raw=%v", color, raw)
	}
}

func TestMaterialTypeErrorsAndUnresolvedReference(t *testing.T) {
	sh := mixedShader(t).Runtime(ids.AssetID{Lo: 1}, "mixed")
	for _, value := range []string{"Count=1.5", "Count=true", "Count=2147483648", "Active=1", "Tint=[1, 2]"} {
		d, err := asset.ParseMaterial([]byte(`material version=1 shader=asset("`+materialShaderID+`") { params { `+value+` } }`), "bad.aemat")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(d.Validate(sh), func(p material.Problem) bool { return !p.Warn }) {
			t.Fatalf("wrong typed value accepted: %s", value)
		}
	}
	texture := makeShader(t, "#pragma aego abi 1\n#pragma material\nuniform sampler2D Noise;\n#pragma end\nvoid fragment(){aego_out=texture(Noise,aego_local);}\n").Runtime(ids.AssetID{Lo: 1}, "texture")
	d, err := asset.ParseMaterial([]byte(`material version=1 shader=asset("`+materialShaderID+`") { params { Noise=asset("00000000000000000000000000000042") } }`), "missing.aemat")
	if err != nil {
		t.Fatal(err)
	}
	if problems := d.Validate(texture); len(problems) != 0 {
		t.Fatalf("pure validation required a GPU texture: %v", problems)
	}
	if m, problems := d.Build("missing", texture, nil); m != nil || len(problems) == 0 {
		t.Fatal("unresolved reference silently succeeded")
	}
	if v, ok := d.Value("Noise"); !ok || !strings.HasSuffix(v.Str, "42") {
		t.Fatal("unresolved reference identity lost")
	}
}

func TestShaderArtifactRejectsInvalidLayoutsAndMetadata(t *testing.T) {
	good := asset.EncodeShader(mixedShader(t))
	if _, err := asset.DecodeShader(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*asset.ShaderArtifact){
		"overlapping uniforms": func(a *asset.ShaderArtifact) { a.Params[1].Offset = a.Params[0].Offset },
		"bad domain":           func(a *asset.ShaderArtifact) { a.Domain = shader.Domain(255) },
		"duplicate params":     func(a *asset.ShaderArtifact) { a.Params[1].Name = a.Params[0].Name },
		"duplicate variants":   func(a *asset.ShaderArtifact) { a.Variants[1].Mask = a.Variants[0].Mask },
		"unknown keyword mask": func(a *asset.ShaderArtifact) { a.Variants[0].Mask = 128 },
		"instance budget":      func(a *asset.ShaderArtifact) { a.Lanes = 5 },
		"bad line map":         func(a *asset.ShaderArtifact) { a.Maps[0][0].File = 128 },
		"bad default":          func(a *asset.ShaderArtifact) { a.Params[0].Default[0] = float32(math.NaN()) },
		"unchecked":            func(a *asset.ShaderArtifact) { a.Validation = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a, err := asset.DecodeShader(good)
			if err != nil {
				t.Fatal(err)
			}
			mutate(a)
			if _, err := asset.DecodeShader(asset.EncodeShader(a)); err == nil {
				t.Fatal("malformed artifact accepted")
			}
		})
	}
	for _, data := range [][]byte{good[:len(good)/2], append(slices.Clone(good), 0), append([]byte("XXXX"), good[4:]...)} {
		if _, err := asset.DecodeShader(data); err == nil {
			t.Fatal("corrupted artifact accepted")
		}
	}
}

func FuzzShaderArtifactDecoder(f *testing.F) {
	u, _ := shaderx.Parse([]byte("#pragma aego abi 1\nvoid fragment(){aego_out=vec4(1.0);}\n"), "f.aeshader", nil)
	f.Add(asset.EncodeShader(asset.BuildShader(u)))
	f.Add([]byte("AESH"))
	f.Fuzz(func(t *testing.T, b []byte) {
		b = resealArtifact(b)
		a, err := asset.DecodeShader(b)
		if err == nil {
			if _, err := asset.DecodeShader(asset.EncodeShader(a)); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func FuzzMaterialArtifactDecoder(f *testing.F) {
	d, _ := asset.ParseMaterial([]byte(`material version=1 shader=asset("`+materialShaderID+`") { params { X=1 } }`), "f.aemat")
	f.Add(asset.EncodeMaterial(d))
	f.Add([]byte("AEMT"))
	f.Fuzz(func(t *testing.T, b []byte) {
		b = resealArtifact(b)
		d, err := asset.DecodeMaterial(b, "f.aemat")
		if err == nil {
			if _, err := asset.DecodeMaterial(asset.EncodeMaterial(d), "f.aemat"); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func resealArtifact(b []byte) []byte {
	if len(b) < 40 {
		return b
	}
	b = slices.Clone(b)
	sum := sha256.Sum256(b[:len(b)-32])
	copy(b[len(b)-32:], sum[:])
	return b
}

func TestCustomPostFXPhaseArtifactRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want shader.Features
	}{{"", 0}, {" display_linear", 0}, {" scene_linear", shader.FeaturePostSceneLinear}} {
		src := "#pragma aego abi 1\n#pragma postfx" + tc.flag + "\nvoid fragment(){ aego_out=aego_sample(aego_uv); }\n"
		a := makeShader(t, src)
		got, err := asset.DecodeShader(asset.EncodeShader(a))
		if err != nil || got.Domain != shader.DomainPostFX || got.Features != tc.want {
			t.Fatal(tc.flag, err)
		}
	}
}
