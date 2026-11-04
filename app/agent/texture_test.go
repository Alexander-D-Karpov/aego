package agent

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/format/aetext"
)

func textureCommandFixture(t *testing.T) *Session {
	t.Helper()
	s := materialSession(t)
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: byte(x * 60), A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Project.Root, "assets/height.png"), b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Pipeline.Scan(); err != nil {
		t.Fatal(err)
	}
	if batch, err := s.Pipeline.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("fixture import %v %v", err, batch)
	}
	return s
}
func TestTextureCommandsShareAtomicMetadataMutationsAndImageRevision(t *testing.T) {
	s := textureCommandFixture(t)
	file := "assets/height.png"
	get := materialCommand(t, s, "texture.get", Args{"file": file}).Value.(map[string]any)
	hash := get["sourceHash"].(string)
	materialCommand(t, s, "texture.set", Args{"file": file, "setting": "role", "value": "height", "expected": hash})
	before, _ := os.ReadFile(filepath.Join(s.Project.Root, file+".meta"))
	if _, err := Run(s, "texture.set", Args{"file": file, "setting": "height_strength", "value": "2", "expected": hash}); err == nil {
		t.Fatal("stale metadata hash accepted")
	}
	if _, err := Run(s, "texture.apply", Args{"file": file, "tx": `tx version=1 {op "texture-setting" {path="generate_normal" value=true} op "texture-setting" {path="height_strength" value="bad"}}`}); err == nil {
		t.Fatal("invalid typed batch accepted")
	}
	after, _ := os.ReadFile(filepath.Join(s.Project.Root, file+".meta"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed batch partially wrote metadata")
	}
	materialCommand(t, s, "texture.set", Args{"file": file, "setting": "generate_normal", "value": "true", "dry-run": true})
	after, _ = os.ReadFile(filepath.Join(s.Project.Root, file+".meta"))
	if !bytes.Equal(before, after) {
		t.Fatal("dry run wrote metadata")
	}
	hash = materialCommand(t, s, "texture.get", Args{"file": file}).Value.(map[string]any)["sourceHash"].(string)
	image, _ := os.ReadFile(filepath.Join(s.Project.Root, file))
	image = append(image, 0)
	if err := os.WriteFile(filepath.Join(s.Project.Root, file), image, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(s, "texture.set", Args{"file": file, "setting": "generate_normal", "value": "true", "expected": hash}); err == nil {
		t.Fatal("source image change was not in optimistic revision")
	}
	materialCommand(t, s, "texture.set", Args{"file": file, "setting": "generate_normal", "value": "true"})
	out := materialCommand(t, s, "texture.validate", Args{"file": file})
	if out.Value.(map[string]any)["importChecked"] != true || out.Value.(map[string]any)["gpuLinked"] != false {
		t.Fatal("validation claimed GPU link or did not run importer")
	}
}
func TestTextureCommandsUseDirtySharedMetadataDocument(t *testing.T) {
	s := textureCommandFixture(t)
	file := "assets/height.png"
	before, _ := os.ReadFile(filepath.Join(s.Project.Root, file+".meta"))
	m, err := asset.ParseSourceMeta(before, file+".meta")
	if err != nil {
		t.Fatal(err)
	}
	m.Settings["role"], m.Settings["generate_normal"] = aetext.Ident("height"), aetext.Bool(true)
	host := &auditMaterialHost{text: map[string]string{file + ".meta": string(asset.MarshalSourceMeta(m))}}
	s.host = host
	materialCommand(t, s, "texture.set", Args{"file": file, "setting": "height_strength", "value": "4"})
	edited, err := asset.ParseSourceMeta([]byte(host.text[file+".meta"]), file+".meta")
	if err != nil || edited.Settings.Float("height_strength", 1) != 4 || !edited.Settings.Bool("generate_normal", false) {
		t.Fatal("mutation ignored dirty metadata", err)
	}
	after, _ := os.ReadFile(filepath.Join(s.Project.Root, file+".meta"))
	if !bytes.Equal(before, after) {
		t.Fatal("editor-session texture mutation wrote disk behind history")
	}
}
