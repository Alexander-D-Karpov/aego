package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aego/asset"
	matimp "aego/asset/importers/material"
	shimp "aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine/problems"
	"aego/gpu"
	"aego/project"
	"aego/render/shader"
	"aego/render/shaderx"
	"aego/vfs"
	"aego/vfs/memfs"
)

const exportShaderSource = `#pragma aego abi 1
#pragma variant A B
#pragma variants A, B
#include "shared.glsl"
#pragma material
uniform float Speed; // @default(1)
uniform int Count; // @default(2147483647)
uniform sampler2D Noise;
#pragma end
void fragment(){aego_out=vec4(materialFactor()*Speed);}
`

func writeMaterialExportSource(t *testing.T, root, name, src string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func materialExportProject(t *testing.T) (*project.Project, *pipeline.Pipeline, ids.AssetID) {
	t.Helper()
	if _, err := exec.LookPath("glslangValidator"); err != nil {
		t.Skip("glslangValidator unavailable: real source-validation test not executed")
	}
	p, err := project.Create(t.TempDir(), "MaterialExport", false)
	if err != nil {
		t.Fatal(err)
	}
	writeMaterialExportSource(t, p.Root, "assets/shaders/include/leaf.glsl", "float leaf(){return 1.;}\n")
	writeMaterialExportSource(t, p.Root, "assets/shaders/include/shared.glsl", "#include \"leaf.glsl\"\nfloat materialFactor(){return leaf();}\n")
	writeMaterialExportSource(t, p.Root, "assets/material.aeshader", exportShaderSource)
	reg := pipeline.NewImporterRegistry()
	sh := shimp.New()
	sh.Require = true
	for _, imp := range []pipeline.Importer{sh, shimp.NewInclude(), matimp.New()} {
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
	if result, err := pl.ImportAll(); err != nil || len(result.Failed) > 0 {
		t.Fatalf("shader import: %v %+v", err, result.Failed)
	}
	id, ok := pl.Lookup("assets/material.aeshader")
	if !ok {
		t.Fatal("shader was not indexed")
	}
	writeMaterialExportSource(t, p.Root, "assets/material.aemat", `material version=1 shader=asset("`+id.String()+`") { variants=[A] params { Speed=2 Count=2147483647 Removed={keep=[1,2,3]} } }`)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if result, err := pl.ImportAll(); err != nil || len(result.Failed) > 0 {
		t.Fatalf("material import: %v %+v", err, result.Failed)
	}
	return p, pl, id
}

func hasExportError(ds []problems.Diagnostic, code, contains string) bool {
	for _, d := range ds {
		if d.Severity == problems.Error && (code == "" || d.Code == code) && strings.Contains(d.Message, contains) {
			return true
		}
	}
	return false
}

func TestMaterialExportChecksCurrentSourceWithoutDiscardingLastGood(t *testing.T) {
	p, pl, id := materialExportProject(t)
	s := Take(pl)
	if ds := append(checkMaterialSources(s), checkMaterialArtifacts(s)...); Blocking(ds) != 0 {
		t.Fatalf("valid current project blocked: %+v", ds)
	}
	e, _ := s.idx.Lookup(id)
	a, _ := e.Artifact(asset.ArtShader)
	before, err := s.files.ReadFile(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	// This occurs before the watcher/importer sees the edit. A valid cached
	// program and a clean asset index must not authorize a stale export.
	writeMaterialExportSource(t, p.Root, "assets/material.aeshader", strings.Replace(exportShaderSource, "materialFactor()*Speed", "undefinedSymbol", 1))
	if ds := checkMaterialSources(s); !hasExportError(ds, CodeShader, "last-good cannot be exported") {
		t.Fatalf("broken current source exported the previous artifact: %+v", ds)
	}
	result, err := pl.Reimport(id)
	if err != nil || result.Failed[id] == nil {
		t.Fatalf("broken source did not fail import: %v %+v", err, result.Failed)
	}
	e, _ = pl.Index().Lookup(id)
	a, _ = e.Artifact(asset.ArtShader)
	after, err := pl.Artifacts().ReadFile(a.Path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed import replaced the editor's last-good artifact")
	}
	if ds := checkMaterialSources(Take(pl)); !hasExportError(ds, CodeShader, "last-good cannot be exported") {
		t.Fatalf("failed import made stale source exportable: %+v", ds)
	}
	writeMaterialExportSource(t, p.Root, "assets/material.aeshader", exportShaderSource)
	if result, err := pl.Reimport(id); err != nil || len(result.Failed) != 0 {
		t.Fatalf("fixed source failed import: %v %+v", err, result.Failed)
	}
	if ds := checkMaterialSources(Take(pl)); Blocking(ds) != 0 {
		t.Fatalf("fixed current source remains blocked: %+v", ds)
	}
}

func TestMaterialExportSeesTransitiveMissingAndUnsavedDependencies(t *testing.T) {
	p, pl, _ := materialExportProject(t)
	s := Take(pl)
	leaf := "assets/shaders/include/leaf.glsl"
	writeMaterialExportSource(t, p.Root, leaf, "float leaf(){return 0.25;}\n")
	if ds := checkMaterialSources(s); !hasExportError(ds, CodeShader, "differs from the export snapshot") {
		t.Fatalf("transitive include edit reused stale artifact: %+v", ds)
	}
	if err := os.Remove(filepath.Join(p.Root, leaf)); err != nil {
		t.Fatal(err)
	}
	if ds := checkMaterialSources(s); !hasExportError(ds, CodeShader, "last-good cannot be exported") {
		t.Fatalf("missing transitive include was exported: %+v", ds)
	}
	writeMaterialExportSource(t, p.Root, leaf, "float leaf(){return 1.;}\n")
	s.DirtySources = []string{leaf, "assets/material.aemat"}
	ds := checkMaterialSources(s)
	if Blocking(ds) != 2 || !hasExportError(ds, CodeDirtySource, "save and validate") {
		t.Fatalf("unsaved authoring buffers did not block: %+v", ds)
	}
}

func TestMaterialExportSnapshotOwnsArtifactRevision(t *testing.T) {
	p, pl, id := materialExportProject(t)
	s := Take(pl)
	e, _ := s.idx.Lookup(id)
	a, _ := e.Artifact(asset.ArtShader)
	before, err := s.files.ReadFile(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	before = bytes.Clone(before)
	writeMaterialExportSource(t, p.Root, "assets/material.aeshader", strings.Replace(exportShaderSource, "materialFactor()*Speed", "materialFactor()*Speed*0.5", 1))
	if result, err := pl.Reimport(id); err != nil || len(result.Failed) != 0 {
		t.Fatalf("reimport failed: %v %+v", err, result.Failed)
	}
	frozen, err := s.files.ReadFile(a.Path)
	if err != nil || !bytes.Equal(frozen, before) {
		t.Fatal("reimport changed bytes owned by an older export snapshot")
	}
	if ds := checkMaterialSources(s); !hasExportError(ds, CodeShader, "differs from the export snapshot") {
		t.Fatalf("old export revision accepted new source: %+v", ds)
	}
	fresh := Take(pl)
	if ds := append(checkMaterialSources(fresh), checkMaterialArtifacts(fresh)...); Blocking(ds) != 0 {
		t.Fatalf("fresh export revision rejected: %+v", ds)
	}
}

// This fixture deliberately assigns validation provenance without invoking a
// driver. It tests the export gate, not GLSL/ESSL compile/link itself.
func preparedMaterialSnapshot(t *testing.T, params, variants string) (*Snapshot, map[string][]byte, ids.AssetID) {
	t.Helper()
	u, ds := shaderx.Parse([]byte(strings.Replace(exportShaderSource, "#include \"shared.glsl\"", "float materialFactor(){return 1.;}", 1)), "fixture.aeshader", nil)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	a := asset.BuildShader(u)
	a.Validation = shader.SchemaChecked | shader.ReferenceChecked
	shID := ids.AssetID{Lo: 100}
	matID := ids.AssetID{Lo: 101}
	doc, err := asset.ParseMaterial([]byte(`material version=1 shader=asset("`+shID.String()+`") { `+variants+` params { `+params+` } extension { keep=true } }`), "fixture.aemat")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"shader.aesh": asset.EncodeShader(a), "material.aemt": asset.EncodeMaterial(doc)}
	fs, err := vfs.New(vfs.Mount{Scheme: "cache", Source: memfs.New(files)})
	if err != nil {
		t.Fatal(err)
	}
	idx := asset.NewIndex()
	idx.Put(asset.Entry{ID: shID, Kind: asset.ArtShader, Source: "fixture.aeshader", Artifacts: []asset.Artifact{{Kind: asset.ArtShader, Path: "cache://shader.aesh"}}})
	idx.Put(asset.Entry{ID: matID, Kind: asset.ArtMaterial, Source: "fixture.aemat", Deps: []ids.AssetID{shID}, Artifacts: []asset.Artifact{{Kind: asset.ArtMaterial, Path: "cache://material.aemt"}}})
	return &Snapshot{idx: idx, files: fs}, files, shID
}

func TestMaterialExportRejectsUnvalidatedTargetsAndBadVariants(t *testing.T) {
	for _, tc := range []struct{ name, params, variants, want string }{
		{"variant-combination", "", "variants=[A,B]", "variant"},
		{"unknown-keyword", "", "variants=[FUTURE]", "variant"},
		{"fractional-int", "Count=1.5", "variants=[A]", "Count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := preparedMaterialSnapshot(t, tc.params, tc.variants)
			if ds := checkMaterialArtifacts(s); !hasExportError(ds, CodeMaterial, tc.want) {
				t.Fatalf("invalid material passed strict export: %+v", ds)
			}
		})
	}
	s, files, _ := preparedMaterialSnapshot(t, "", "variants=[A]")
	a, err := asset.DecodeShader(files["shader.aesh"])
	if err != nil {
		t.Fatal(err)
	}
	a.Validation = shader.SchemaChecked
	files["shader.aesh"] = asset.EncodeShader(a)
	if ds := checkMaterialArtifacts(s); !hasExportError(ds, CodeShader, "reference validation") {
		t.Fatalf("headless schema check masqueraded as target validation: %+v", ds)
	}
	a.Validation |= shader.ReferenceChecked
	a.Variants[0].Sources[3] = ""
	files["shader.aesh"] = asset.EncodeShader(a)
	if ds := checkMaterialArtifacts(s); !hasExportError(ds, CodeShader, "") {
		t.Fatalf("missing ESSL fragment source passed: %+v", ds)
	}
}

func TestMaterialExportPreservesUnknownAndChecksTexturePayload(t *testing.T) {
	textureID := ids.AssetID{Lo: 102}
	s, files, _ := preparedMaterialSnapshot(t, `Count=2147483647 Removed={keep=[1,2,3]} Noise=asset("`+textureID.String()+`")`, "variants=[A]")
	before := bytes.Clone(files["material.aemt"])
	if ds := checkMaterialArtifacts(s); !hasExportError(ds, CodeMaterial, "texture Noise is missing") {
		t.Fatalf("missing texture reference passed: %+v", ds)
	}
	s.idx.Put(asset.Entry{ID: textureID, Kind: asset.ArtTexture, Source: "noise.png"})
	if ds := checkMaterialArtifacts(s); !hasExportError(ds, CodeMaterial, "texture Noise is missing") {
		t.Fatalf("texture entry without artifact passed: %+v", ds)
	}
	s.idx.Put(asset.Entry{ID: textureID, Kind: asset.ArtTexture, Source: "noise.png", Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://noise.aetx"}}})
	files["noise.aetx"] = []byte("corrupt")
	if ds := checkMaterialArtifacts(s); !hasExportError(ds, CodeMaterial, "texture Noise is missing") {
		t.Fatalf("corrupt texture artifact passed: %+v", ds)
	}
	files["noise.aetx"] = asset.EncodeTexture(&asset.TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Pixels: []byte{255, 255, 255, 255}})
	if ds := checkMaterialArtifacts(s); Blocking(ds) != 0 {
		t.Fatalf("valid texture with preserved unknown fields blocked: %+v", ds)
	}
	if !bytes.Equal(before, files["material.aemt"]) {
		t.Fatal("export validation rewrote unknown material data")
	}
	doc, err := asset.DecodeMaterial(files["material.aemt"], "exported.aemt")
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := doc.Value("Count"); !ok || count.Int != 2147483647 {
		t.Fatal("exported material rounded an int32")
	}
	if _, ok := doc.Value("Removed"); !ok {
		t.Fatal("export dropped unknown semantic data")
	}
	null, _, _ := preparedMaterialSnapshot(t, `Noise=asset("`+ids.AssetID{}.String()+`")`, "variants=[A]")
	if ds := checkMaterialArtifacts(null); Blocking(ds) != 0 {
		t.Fatalf("explicit null texture fallback rejected: %+v", ds)
	}
}

func TestExportFreezeRejectsArtifactBytesFromAnotherRevision(t *testing.T) {
	s, files, id := preparedMaterialSnapshot(t, "", "variants=[A]")
	e, _ := s.idx.Lookup(id)
	sum := sha256.Sum256(files["shader.aesh"])
	e.Artifacts[0].Hash = hex.EncodeToString(sum[:8])
	e.Artifacts[0].Size = int64(len(files["shader.aesh"]))
	files["shader.aesh"] = append(files["shader.aesh"], 0)
	_, ds := freezeArtifacts(s.idx, s.files)
	if !hasExportError(ds, CodeImport, "do not match the imported revision") {
		t.Fatalf("snapshot accepted a cache race: %+v", ds)
	}
}
