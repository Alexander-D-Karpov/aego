package shader_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"aego/asset"
	shimp "aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/ids"
	rshader "aego/render/shader"
)

func relativeContext(source string, files map[string]string) *pipeline.ImportContext {
	return &pipeline.ImportContext{
		ID: ids.AssetID{Lo: 7}, Path: "assets/effects/main.aeshader", Source: []byte(source),
		Read: func(name string) ([]byte, error) {
			if source, ok := files[name]; ok {
				return []byte(source), nil
			}
			return nil, fmt.Errorf("missing %s", name)
		},
	}
}

func TestRelativeNestedIncludesPreserveTargetsMapsAndCache(t *testing.T) {
	if _, err := exec.LookPath("glslangValidator"); err != nil {
		t.Skip("glslangValidator unavailable: reference targets unverified")
	}
	const source = "#pragma aego abi 1\n#pragma variant GLOW\n#include \"./a/main.glsl\"\n#include \"./b/main.glsl\"\n#include \"shared.glsl\"\nvoid fragment(){aego_out=vec4(valueA()+valueB()+legacyValue());}\n"
	const leaf = "assets/effects/a/leaf.glsl"
	files := map[string]string{
		"assets/effects/a/main.glsl":         "#include \"./leaf.glsl\"\n",
		leaf:                                 "float valueA(){\n return 1.;\n}\n",
		"assets/effects/b/main.glsl":         "#include \"../shared/b.glsl\"\n",
		"assets/effects/shared/b.glsl":       "float valueB(){return 2.;}\n",
		"assets/shaders/include/shared.glsl": "float legacyValue(){return 3.;}\n",
		// Same basename in the legacy root must not steal an owner-relative include.
		"assets/shaders/include/leaf.glsl": "not a valid GLSL declaration\n",
	}
	ctx := relativeContext(source, files)
	lookup := map[string]ids.AssetID{}
	for _, name := range []string{"assets/effects/a/main.glsl", leaf, "assets/effects/b/main.glsl", "assets/effects/shared/b.glsl", "assets/shaders/include/shared.glsl"} {
		lookup[name] = ids.AssetID{Lo: uint64(len(lookup) + 20)}
	}
	ctx.Lookup = func(name string) (ids.AssetID, bool) { id, ok := lookup[name]; return id, ok }
	i := shimp.New()
	i.Require = true
	before, err := i.CacheSalt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := i.Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := asset.DecodeShader(result.Artifacts[0].Data)
	if err != nil || a.Validation&rshader.ReferenceChecked == 0 || len(a.Variants) != 2 {
		t.Fatalf("relative shader did not validate both target pairs/variants: %v %+v", err, a)
	}
	if len(result.Deps) != len(lookup) || len(a.Dependencies) != len(lookup)+1 {
		t.Fatalf("relative dependencies missing: %+v %+v", result.Deps, a.Dependencies)
	}
	for _, v := range a.Variants {
		for stage, source := range v.Sources {
			if strings.Contains(source, "#include") || (stage%2 == 1 && (!strings.Contains(source, "legacyValue") || !strings.Contains(source, "valueA") || !strings.Contains(source, "valueB"))) {
				t.Fatal("prepared source lost an include or kept authoring directives")
			}
		}
	}
	files[leaf] = "float valueA(){\n return 4.;\n}\n"
	after, err := i.CacheSalt(ctx)
	if err != nil || bytes.Equal(before, after) {
		t.Fatalf("relative transitive edit reused old cache salt: %v", err)
	}
	files[leaf] = "float valueA(){\n return undeclaredRelativeValue;\n}\n"
	if _, err := i.Import(ctx); err == nil {
		t.Fatal("invalid relative include passed reference validation")
	}
	ds, _ := i.Diagnostics()
	seen := map[string]map[uint32]bool{}
	for _, d := range ds {
		if d.File != leaf || d.Line != 2 || d.Stage != "fragment" || d.AssetID != ctx.ID.String() || !strings.Contains(d.RawLog, "undeclaredRelativeValue") {
			continue
		}
		if seen[d.Backend] == nil {
			seen[d.Backend] = map[uint32]bool{}
		}
		seen[d.Backend][d.Variant] = true
	}
	for _, backend := range []string{"GLSL330", "ESSL300"} {
		if !seen[backend][0] || !seen[backend][1] {
			t.Fatalf("relative include source-map/target/variant context lost: %+v", ds)
		}
	}
}

func TestRelativeIncludeCyclesTraversalAndNoFallback(t *testing.T) {
	i := shimp.New()
	i.Require, i.Validator = false, "aego-test-no-validator"
	files := map[string]string{
		"assets/effects/a.glsl":        "#include \"./nested/b.glsl\"\n",
		"assets/effects/nested/b.glsl": "#include \"../a.glsl\"\n",
	}
	ctx := relativeContext("#pragma aego abi 1\n#include \"./a.glsl\"\nvoid fragment(){aego_out=vec4(1.);}\n", files)
	if _, err := i.Import(ctx); err == nil {
		t.Fatal("canonical owner-relative cycle accepted")
	}
	ds, _ := i.Diagnostics()
	if !slices.ContainsFunc(ds, func(d rshader.Diag) bool {
		return d.Code == "SHD005" && d.File == "assets/effects/nested/b.glsl" && d.Line == 1
	}) {
		t.Fatalf("cycle did not identify its owning include: %+v", ds)
	}
	for _, name := range []string{"../../outside.glsl", "../../../outside.glsl", "/tmp/outside.glsl", "./http://outside.glsl", "..\\outside.glsl"} {
		reads := 0
		ctx := relativeContext("#pragma aego abi 1\n#include \""+name+"\"\nvoid fragment(){aego_out=vec4(1.);}\n", nil)
		ctx.Read = func(string) ([]byte, error) { reads++; return []byte(""), nil }
		if _, err := i.Import(ctx); err == nil || reads != 0 {
			t.Fatalf("outside-namespace include reached VFS: %q reads=%d err=%v", name, reads, err)
		}
	}
	ctx = relativeContext("#pragma aego abi 1\n#include \"./missing.glsl\"\nvoid fragment(){aego_out=vec4(1.);}\n", map[string]string{"assets/shaders/include/missing.glsl": "float unintended(){return 1.;}\n"})
	if _, err := i.Import(ctx); err == nil {
		t.Fatal("missing owner-relative include silently fell back to the legacy root")
	}
}

func TestEngineRelativeIncludesStayInRegisteredShaderNamespace(t *testing.T) {
	i := shimp.New()
	i.Require, i.Validator = false, "aego-test-no-validator"
	files := map[string]string{
		"engine://shaders/include/parent.glsl":       "#include \"./nested/child.glsl\"\n",
		"engine://shaders/include/nested/child.glsl": "#include \"../leaf.glsl\"\n",
		"engine://shaders/include/leaf.glsl":         "float helper(){return 1.;}\n",
	}
	ctx := relativeContext("#pragma aego abi 1\n#include \"engine/parent.glsl\"\nvoid fragment(){aego_out=vec4(helper());}\n", files)
	result, err := i.Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := asset.DecodeShader(result.Artifacts[0].Data)
	if err != nil || !slices.Contains(a.Files, "engine://shaders/include/leaf.glsl") {
		t.Fatalf("engine relative source missing: %v %+v", err, a)
	}
	files["engine://shaders/include/nested/child.glsl"] = "#include \"../../../fonts/private.glsl\"\n"
	if _, err := i.Import(ctx); err == nil {
		t.Fatal("engine relative include escaped its shader namespace")
	}
}
