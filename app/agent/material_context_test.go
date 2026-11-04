package agent

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditMaterialGetReportsFormerExplicitValue(t *testing.T) {
	s := materialSession(t)
	const shfile = "assets/former.aeshader"
	materialCommand(t, s, "shader.create", Args{"file": shfile})
	src := "#pragma aego abi 1\n#pragma material\nuniform int Count; // @former(OldCount) @default(5)\n#pragma end\nvoid fragment(){aego_out=vec4(float(Count));}\n"
	if err := os.WriteFile(filepath.Join(s.Project.Root, shfile), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	id, _ := s.Pipeline.Lookup(shfile)
	const file = "assets/former.aemat"
	materialCommand(t, s, "material.create", Args{"file": file, "shader": shfile})
	mat := fmt.Sprintf("material version=1 shader=asset(%q) { params { OldCount=2147483647 } }", id.String())
	if err := os.WriteFile(filepath.Join(s.Project.Root, file), []byte(mat), 0644); err != nil {
		t.Fatal(err)
	}
	out := materialCommand(t, s, "material.get", Args{"file": file})
	params := out.Value.(map[string]any)["parameters"].([]materialParamView)
	if len(params) != 1 || !params[0].Explicit || params[0].Value != "2147483647" {
		t.Fatalf("explicit Former assignment reported as inherited: %+v", params)
	}
}

type auditMaterialHost struct {
	Host
	text  map[string]string
	edits []ScriptEdit
}

func (h *auditMaterialHost) ScriptText(rel string) (string, bool) { v, ok := h.text[rel]; return v, ok }

func (h *auditMaterialHost) ApplyScript(e ScriptEdit, _ string) error {
	if h.text[e.Rel] != e.Before {
		return fmt.Errorf("dirty document changed before applying material mutation")
	}
	h.text[e.Rel] = e.After
	h.edits = append(h.edits, e)
	return nil
}

func TestMaterialCommandsUseDirtyDocumentAndRejectItsStaleRevision(t *testing.T) {
	s := materialSession(t)
	const file = "assets/dirty.aemat"
	materialCommand(t, s, "material.create", Args{"file": file, "shader": "engine/shaders/flash"})
	disk, err := os.ReadFile(filepath.Join(s.Project.Root, file))
	if err != nil {
		t.Fatal(err)
	}
	dirty := strings.Replace(string(disk), "params {}", "params { Flash=0.4 Removed={keep=true} }", 1)
	if dirty == string(disk) {
		t.Fatal("fixture could not create a distinct dirty material")
	}
	host := &auditMaterialHost{text: map[string]string{file: dirty}}
	s.host = host
	out := materialCommand(t, s, "material.get", Args{"file": file})
	result := out.Value.(map[string]any)
	hash := result["sourceHash"].(string)
	if hash != sourceHash([]byte(dirty)) || hash == sourceHash(disk) {
		t.Fatal("material.get reported the saved document instead of the dirty one")
	}
	materialCommand(t, s, "material.set", Args{"file": file, "param": "Flash", "value": "0.7", "expected": hash, "dry-run": true})
	if host.text[file] != dirty || len(host.edits) != 0 {
		t.Fatal("mutation preview changed the dirty material")
	}
	materialCommand(t, s, "material.set", Args{"file": file, "param": "Flash", "value": "0.7", "expected": hash})
	if len(host.edits) != 1 || !strings.Contains(host.text[file], "Removed") || !strings.Contains(host.text[file], "0.7") {
		t.Fatal("dirty mutation lost unknown data or did not form one host edit")
	}
	before := host.text[file]
	if _, err := Run(s, "material.set", Args{"file": file, "param": "Flash", "value": "0.2", "expected": hash}); err == nil || host.text[file] != before {
		t.Fatal("stale dirty revision changed the document")
	}
	after, err := os.ReadFile(filepath.Join(s.Project.Root, file))
	if err != nil || !bytes.Equal(after, disk) {
		t.Fatal("editor mutation changed the saved file")
	}
}

func TestAuditShaderValidateReadsUnsavedInclude(t *testing.T) {
	s := materialSession(t)
	const shfile = "assets/overlay.aeshader"
	const incfile = "assets/shaders/include/live.glsl"
	materialCommand(t, s, "shader.create", Args{"file": shfile})
	if err := os.MkdirAll(filepath.Dir(filepath.Join(s.Project.Root, incfile)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Project.Root, incfile), []byte("float live(){return 1.;}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := "#pragma aego abi 1\n#include \"live.glsl\"\nvoid fragment(){aego_out=vec4(live());}\n"
	if err := os.WriteFile(filepath.Join(s.Project.Root, shfile), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	materialCommand(t, s, "shader.validate", Args{"shader": shfile})
	s.host = &auditMaterialHost{text: map[string]string{incfile: "#include \"live.glsl\"\n"}}
	if _, err := Run(s, "shader.validate", Args{"shader": shfile}); err == nil {
		t.Fatal("unsaved cyclic include ignored")
	}
}

func TestAuditMaterialValidationReportsFailedTextureImport(t *testing.T) {
	s := materialSession(t)
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	id, err := s.create("assets/noise.png", pixels.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	const file = "assets/dissolve.aemat"
	materialCommand(t, s, "material.create", Args{"file": file, "shader": "engine/shaders/dissolve"})
	materialCommand(t, s, "material.set", Args{"file": file, "param": "Noise", "value": fmt.Sprintf("asset(%q)", id.String())})
	if err := os.WriteFile(filepath.Join(s.Project.Root, "assets/noise.png"), []byte("broken current PNG"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := s.Pipeline.Import(id)
	if err != nil || b == nil || b.Failed[id] == nil {
		t.Fatalf("missing failed source import: %v %+v", err, b)
	}
	out := materialCommand(t, s, "material.validate", Args{"file": file})
	for _, p := range out.Problems {
		if p.Error {
			return
		}
	}
	t.Fatalf("material validation accepted failed current texture import: %+v", out.Problems)
}
