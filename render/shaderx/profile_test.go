package shaderx

import (
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aego/render/shader"
)

func TestCommentsDoNotDeclareMetadataOrEntries(t *testing.T) {
	src := head + `/*
#pragma material
uniform mat4 Fake;
#pragma end
void vertex() {}
texture2D();
*/
#pragma material
uniform /* type */ float
    Real; // @default(0.5) @doc("a (parenthesis) and a quote \"")
#pragma end
void fragment() { aego_out=vec4(Real); }
`
	u, ds := Parse([]byte(src), "comments.aeshader", nil)
	if len(ds) != 0 || u.Vertex || len(u.Params) != 1 || u.Params[0].Name != "Real" || u.Params[0].Default[0] != 0.5 {
		t.Fatalf("params=%+v vertex=%v diagnostics=%v", u.Params, u.Vertex, ds)
	}
	for _, fake := range []string{"/* void fragment() {} */", "#define FAKE void fragment() {}\n"} {
		_, ds := Parse([]byte(head+fake), "fake.aeshader", nil)
		if !slices.Contains(diagCodes(ds), SHD001) {
			t.Fatalf("fake entry accepted: %q", fake)
		}
	}
}

func TestTypedInstanceParametersAndLexicalScope(t *testing.T) {
	src := head + `#pragma material
uniform int Count; // @instance @default(2147483647)
uniform bool Enabled; // @instance @default(true)
uniform float Flash; // @instance @former(OldFlash) @default(0.25)
#pragma end
struct Sample { float Flash; };
void fragment() {
    Sample sampleValue; sampleValue.Flash=0.75;
    float Flash=0.5;
    aego_out=vec4(Enabled && Count==2147483647 ? sampleValue.Flash+Flash : 0.0);
}
`
	u, ds := Parse([]byte(src), "scope.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	if u.Params[0].IntDefault != math.MaxInt32 || u.Params[1].Default[0] != 1 || !slices.Equal(u.Params[2].Former, []string{"OldFlash"}) {
		t.Fatalf("defaults/aliases: %+v", u.Params)
	}
	for _, es := range []bool{false, true} {
		vert, _ := u.Source(false, es, 0)
		frag, _ := u.Source(true, es, 0)
		if !strings.Contains(vert, "in uvec4 aego_i_lane0") || !strings.Contains(frag, "flat in uvec4 aego_lane0") || strings.Contains(frag, "#define Flash") || !strings.Contains(frag, "sampleValue.Flash") {
			t.Fatal("typed transport or lexical scope was lost")
		}
		bin := glslang(t)
		dir := t.TempDir()
		glslangRun(t, bin, filepath.Join(dir, "scope.vert"), vert)
		glslangRun(t, bin, filepath.Join(dir, "scope.frag"), frag)
	}
}

func TestInstanceHelpersHaveInitializedTypedGlobalsBeforeBothEntries(t *testing.T) {
	src := head + `#pragma material
uniform int Count; // @instance @default(2147483647)
uniform bool Enabled; // @instance @default(true)
uniform float Flash; // @instance @default(0.25)
uniform vec2 Offset; // @instance
uniform vec3 Direction; // @instance
uniform vec4 Tint; // @instance
#pragma end
float sampleEffective() { return float(Count) + (Enabled ? Flash : 0.0) + Offset.x + Direction.y + Tint.z; }
void vertex() { aego_world.x += sampleEffective(); }
void fragment() { aego_out=vec4(sampleEffective()); }
`
	u, ds := Parse([]byte(src), "helpers.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	bin := glslang(t)
	for _, es := range []bool{false, true} {
		for _, frag := range []bool{false, true} {
			source, _ := u.Source(frag, es, 0)
			for _, declaration := range []string{"int Count = 0;", "bool Enabled = false;", "float Flash = 0.0;", "vec2 Offset = vec2(0.0);", "vec3 Direction = vec3(0.0);", "vec4 Tint = vec4(0.0);"} {
				if !strings.Contains(source, declaration) {
					t.Fatalf("missing typed constant initializer %q", declaration)
				}
			}
			main := source[strings.LastIndex(source, "void main() {"):]
			entry := "    vertex();"
			if frag {
				entry = "    fragment();"
			}
			for _, name := range []string{"Count", "Enabled", "Flash", "Offset", "Direction", "Tint"} {
				assigned := strings.Index(main, "    "+name+" = ")
				if assigned < 0 || assigned > strings.Index(main, entry) {
					t.Fatalf("%s not assigned its effective lanes before %s", name, entry)
				}
			}
			glslangRun(t, bin, filepath.Join(t.TempDir(), "helpers."+stageName(frag, es)), source)
		}
	}
}

func TestSchemaCannotDependOnPreprocessorOrLocalScope(t *testing.T) {
	cases := []string{
		head + "#if 0\n#pragma material\nuniform float X;\n#pragma end\n#endif" + frag,
		head + "#pragma material\n#if 0\nuniform float X;\n#endif\n#pragma end" + frag,
		head + "void helper() {\n#pragma material\nuniform float X;\n#pragma end\n}" + frag,
		head + "#pragma material\nuniform int Count; // @default(2147483648)\n#pragma end" + frag,
		head + "#pragma material\nuniform float X; // @default(NaN)\n#pragma end" + frag,
		head + "#pragma material\nuniform float X; // @doc(\"broken)\n#pragma end" + frag,
		head + "#pragma material\nuniform float X; // @default(1) @default(2)\n#pragma end" + frag,
		head + "#pragma material\nuniform float packed;\n#pragma end" + frag,
		head + "#pragma variant " + strings.Repeat("A", shader.MaxNameLen+1) + "\n" + frag,
	}
	for _, src := range cases {
		if _, ds := Parse([]byte(src), "bad.aeshader", nil); len(ds) == 0 {
			t.Fatalf("invalid schema accepted: %s", src)
		}
	}
}

func TestExpandedIncludesAreBoundedAndHashed(t *testing.T) {
	inc := testIncluder(map[string]string{"a.glsl": "#include \"b.glsl\"\n", "b.glsl": "float child(){return 1.;}\n"})
	u, ds := Parse([]byte(head+"#include \"a.glsl\"\n"+frag), "deps.aeshader", inc)
	if len(ds) != 0 || len(u.Dependencies) != 3 {
		t.Fatalf("deps=%v diagnostics=%v", u.Dependencies, ds)
	}
	changed := testIncluder(map[string]string{"a.glsl": "#include \"b.glsl\"\n", "b.glsl": "float child(){return 2.;}\n"})
	v, ds := Parse([]byte(head+"#include \"a.glsl\"\n"+frag), "deps.aeshader", changed)
	if len(ds) != 0 || u.Dependencies[2].Hash == v.Dependencies[2].Hash {
		t.Fatal("transitive source mutation did not change dependency hash")
	}
	huge := testIncluder(map[string]string{"big.glsl": "/*" + strings.Repeat(" ", shader.MaxExpandedBytes) + "*/"})
	_, ds = Parse([]byte(head+"#include \"big.glsl\"\n"+frag), "large.aeshader", huge)
	if !slices.Contains(diagCodes(ds), SHD004) {
		t.Fatal("expanded source budget was not enforced")
	}
}

func TestRepeatedIncludesKeepOneDependencyAndRejectChangingReads(t *testing.T) {
	src := head + strings.Repeat("#include \"constant.glsl\"\n", shader.MaxIncludeFiles+1) + frag
	u, ds := Parse([]byte(src), "repeat.aeshader", testIncluder(map[string]string{"constant.glsl": "// stable source\n"}))
	if len(ds) != 0 || len(u.Dependencies) != 2 || len(u.Files) != 2 {
		t.Fatalf("repeated source produced a corrupt dependency table: %v %+v", ds, u.Dependencies)
	}
	read := 0
	inc := func(string) (string, []byte, error) {
		read++
		return "constant.glsl", []byte(strings.Repeat(" ", read)), nil
	}
	if _, ds := Parse([]byte(head+strings.Repeat("#include \"constant.glsl\"\n", 2)+frag), "race.aeshader", inc); !slices.Contains(diagCodes(ds), SHD002) {
		t.Fatalf("inconsistent include snapshot accepted: %v", ds)
	}
}

func TestPortableChecksUseTokens(t *testing.T) {
	if _, ds := Parse([]byte(head+"/* texture2D gl_FragColor buffer */\n"+frag), "comment.aeshader", nil); len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, code := range []string{"texture2D (aego_main,aego_uv)", "double x = 1.0;", "layout(std430) buffer Data { float x; };", "float packed = 1.0;"} {
		if _, ds := Parse([]byte(head+"void fragment(){ "+code+" }"), "profile.aeshader", nil); !slices.Contains(diagCodes(ds), SHD008) {
			t.Fatalf("unsupported code accepted: %s; %v", code, ds)
		}
	}
}

func TestMesh2DInstanceShaderReferenceValidation(t *testing.T) {
	u, ds := Parse([]byte(head+"#pragma domain mesh2d\n#pragma material\nuniform int Value; // @instance @default(2147483647)\n#pragma end\nvoid fragment(){aego_out=vec4(float(Value==2147483647));}\n"), "mesh.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	bin := glslang(t)
	dir := t.TempDir()
	for _, es := range []bool{false, true} {
		for _, fr := range []bool{false, true} {
			src, _ := u.Source(fr, es, 0)
			suffix := ".vert"
			if fr {
				suffix = ".frag"
			}
			glslangRun(t, bin, filepath.Join(dir, "mesh"+suffix), src)
		}
	}
}
