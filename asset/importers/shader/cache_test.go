package shader_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"aego/asset"
	"aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
	rshader "aego/render/shader"
)

func TestCacheSaltSeesTransitiveEngineIncludes(t *testing.T) {
	sources := map[string]string{
		"engine://shaders/include/a.glsl": "#include \"engine/b.glsl\"\n",
		"engine://shaders/include/b.glsl": "float helper(){return 1.;}\n",
	}
	ctx := &pipeline.ImportContext{ID: ids.AssetID{Lo: 1}, Path: "salt.aeshader", Source: []byte("#pragma aego abi 1\n#include \"engine/a.glsl\"\nvoid fragment(){aego_out=vec4(helper());}\n"), Read: func(path string) ([]byte, error) {
		s, ok := sources[path]
		if !ok {
			return nil, fmt.Errorf("missing %s", path)
		}
		return []byte(s), nil
	}}
	i := shader.New()
	i.Validator = "aego-test-no-validator"
	i.Require = false
	before, err := i.CacheSalt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sources["engine://shaders/include/b.glsl"] = "float helper(){return 2.;}\n"
	after, err := i.CacheSalt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("engine include edit reused the prior cache key")
	}
	result, err := i.Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := asset.DecodeShader(result.Artifacts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if a.Validation != rshader.SchemaChecked {
		t.Fatalf("missing validator falsely checked flags %b", a.Validation)
	}
	if len(a.Dependencies) != 3 || !strings.Contains(a.Variants[0].Sources[1], "return 2.") {
		t.Fatal("prepared artifact omitted a transitive engine include")
	}
	i.Require = true
	if _, err := i.CacheSalt(ctx); err == nil {
		t.Fatal("require-validator reused an unchecked cache")
	}
}

func TestReferenceDiagnosticsKeepTargetStageIncludeAndRawLog(t *testing.T) {
	if _, err := exec.LookPath("glslangValidator"); err != nil {
		t.Skip("glslangValidator unavailable")
	}
	ctx := &pipeline.ImportContext{ID: ids.AssetID{Lo: 7}, Path: "broken.aeshader", Source: []byte("#pragma aego abi 1\n#include \"broken.glsl\"\nvoid fragment(){aego_out=vec4(helper());}\n"), Read: func(string) ([]byte, error) { return []byte("float helper(){\n return missingName;\n}\n"), nil }}
	i := shader.New()
	i.Require = true
	if _, err := i.Import(ctx); err == nil {
		t.Fatal("invalid shader passed reference compilation")
	}
	ds, _ := i.Diagnostics()
	found := map[string]bool{}
	for _, d := range ds {
		if d.File == "assets/shaders/include/broken.glsl" && d.Line == 2 && d.Stage == "fragment" && d.AssetID == ctx.ID.String() && strings.Contains(d.RawLog, "missingName") {
			found[d.Backend] = true
		}
	}
	if !found["GLSL330"] || !found["ESSL300"] {
		t.Fatalf("incomplete diagnostic context: %+v", ds)
	}
}

// Seed a live Pipeline with the salt produced by generator v7 for this exact
// input. The next lookup goes through the current generator on the same cache.
type upgradingPostShader struct {
	*shader.Importer
	useV7 bool
}

func (i *upgradingPostShader) CacheSalt(ctx *pipeline.ImportContext) ([]byte, error) {
	if i.useV7 {
		return hex.DecodeString("531d938d0d0a1b4d6cd6e9bd3fb18316ff0568b5ffb417a36ea11fa3d43338d7")
	}
	return i.Importer.CacheSalt(ctx)
}

func TestPostFXReimportsAfterGeneratorUpgrade(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Cached post effect", false)
	if err != nil {
		t.Fatal(err)
	}
	write(t, p.Root, "assets/cold.aeshader", "#pragma aego abi 1\n#pragma postfx\nvoid fragment(){ aego_out = aego_sample(aego_uv); }\n")
	imp := &upgradingPostShader{Importer: shader.New(), useV7: true}
	imp.Validator, imp.Require = "aego-no-reference-validator", false
	reg := pipeline.NewImporterRegistry()
	if err = reg.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err = pl.Scan(); err != nil {
		t.Fatal(err)
	}
	check := func(want int) {
		t.Helper()
		batch, err := pl.ImportAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(batch.Failed) != 0 || len(batch.Imported) != want {
			t.Fatalf("want %d shader imports, got %d; failures %v", want, len(batch.Imported), batch.Failed)
		}
	}
	check(1)
	check(0)
	imp.useV7 = false
	check(1)
	check(0)
}
