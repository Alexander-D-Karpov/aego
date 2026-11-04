//go:build aego_gltest

package integration

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aego/app/edit"
	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/preview"
	"aego/editor/core/workspace"
	"aego/project"
)

func TestPlatformerMaterialTilesAndUndo(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	base := copyExample(t, "platformer")
	if err := os.MkdirAll(filepath.Join(base, "native"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "native", "example.go"), []byte("package native\n\nfunc DeclareAbility() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "assets", "gallery-test")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"sample.glsl":     "vec4 gallery_color(vec4 c){return c;}\n",
		"sample.aeshader": "#pragma aego abi 1\n#pragma domain sprite\n#include \"engine/sprite.glsl\"\nvoid fragment(){aego_out=aego_sample();}\n",
		"sample.aepreset": "prefab version=1 { node \"n\" name=\"p\" { component \"engine.SpriteRenderer\" version=1 {} } }",
		"sample.aetable":  "table version=1 type=\"mygame.EnemyStats\" { row \"normal\" {speed=1} }",
		"sample.obj":      "v 0 0 0\nv 0 1 0\nv 1 0 0\nf 1 2 3\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := project.Load(base)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: 960, Height: 640, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	grid := s.App.AssetsPanel()
	s.App.Workspace().Show(workspace.Assets)
	s.App.Workspace().ToggleMaximize(workspace.Assets)
	grid.SetTileSize(96)
	grid.SetFolder("assets/materials")
	get := func(id ids.AssetID) (*image.NRGBA, string) {
		t.Helper()
		stop := time.Now().Add(12 * time.Second)
		for time.Now().Before(stop) {
			s.Step()
			im, key, err := s.App.Previews().Content(id)
			if errors.Is(err, preview.ErrPending) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := s.App.Previews().ByID(id); !ok {
				t.Fatal("no tile texture")
			}
			return im.(*image.NRGBA), key
		}
		t.Fatal("material preview did not finish")
		return nil, ""
	}
	tiles := make(map[string]*image.NRGBA)
	var lamp ids.AssetID
	var firstKey string
	for _, name := range []string{"character_lit", "lamp_lit", "world_lit"} {
		id, ok := ed.Pipeline.Lookup("assets/materials/" + name + ".aemat")
		if !ok {
			t.Fatal(name)
		}
		im, key := get(id)
		if _, _, ok := grid.TileRect(id); !ok {
			t.Fatalf("tile %s not visible", name)
		}
		tiles[name] = im
		if name == "lamp_lit" {
			lamp, firstKey = id, key
		}
	}
	if bytes.Equal(tiles["world_lit"].Pix, tiles["lamp_lit"].Pix) {
		t.Fatal("emissive material reproduced the unlit surface")
	}
	if dir := os.Getenv("AEGO_THUMBNAIL_SHOTS"); dir != "" {
		var screen *image.RGBA
		s.Engine.CaptureFrame(func(im *image.RGBA, err error) {
			if err != nil {
				t.Fatal(err)
			}
			screen = im
		})
		s.Step()
		if screen == nil {
			t.Fatal("editor frame was not captured")
		}
		f, err := os.Create(filepath.Join(dir, "assets-materials.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		for name, im := range tiles {
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, im); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	d, err := ed.Docs.OpenScript("assets/materials/lamp_lit.aemat")
	if err != nil {
		t.Fatal(err)
	}
	src := d.Code.Bytes()
	i := bytes.Index(src, []byte("EmissionIntensity=1.4"))
	if i < 0 {
		t.Fatal("no emission")
	}
	d.Code.Replace(i, len("EmissionIntensity=1.4"), "EmissionIntensity=0")
	changed, key := get(lamp)
	if key == firstKey || bytes.Equal(changed.Pix, tiles["lamp_lit"].Pix) {
		t.Fatal("dirty document did not change the same Assets tile")
	}
	d.Code.Undo()
	restored, _ := get(lamp)
	if !bytes.Equal(restored.Pix, tiles["lamp_lit"].Pix) {
		t.Fatal("undo did not restore pixels")
	}
	seen := map[asset.ArtifactKind]bool{}
	for _, info := range ed.Pipeline.Assets() {
		if info.Error != "" && strings.Contains(info.Path, "gallery-test") {
			t.Errorf("%s: %s", info.Path, info.Error)
		}
		if seen[info.Kind] || info.Error != "" || info.Kind == "" {
			continue
		}
		if _, ok := ed.Pipeline.Entry(info.ID); !ok {
			continue
		}
		im, _ := get(info.ID)
		if im.Bounds().Empty() {
			t.Errorf("empty thumbnail for %s", info.Path)
		}
		seen[info.Kind] = true
	}
	ed.Pipeline.Index().Each(func(e *asset.Entry) {
		if e.Kind == asset.ArtAtlasPage && !seen[e.Kind] {
			get(e.ID)
			seen[e.Kind] = true
		}
	})
	for _, kind := range []asset.ArtifactKind{asset.ArtMesh, asset.ArtShader, asset.ArtShaderInclude, asset.ArtPreset, asset.ArtTable, asset.ArtAtlasPage, asset.ArtScript, asset.ArtTexture, asset.ArtScene, asset.ArtPrefab, asset.ArtAudio, asset.ArtData, asset.ArtAnim, asset.ArtCtrl, asset.ArtMaterial} {
		if !seen[kind] {
			t.Errorf("fixture has no %s preview", kind)
		}
	}
	for _, file := range []string{"project.aego", "native/example.go"} {
		id, ok := s.App.Previews().SourceID(file)
		if !ok {
			t.Fatalf("missing editor source %s", file)
		}
		if _, ok := ed.Pipeline.Entry(id); ok {
			t.Fatal("editor-only file became runtime asset")
		}
		pic, _ := get(id)
		if pic.Bounds().Empty() {
			t.Fatal("empty source card")
		}
		d, err := ed.OpenAssetPath(file)
		if err != nil || d.Source == nil || !d.Source.ReadOnly {
			t.Fatalf("source handler %s: %v", file, err)
		}
	}
	t.Logf("platformer imported kinds: %v", seen)
	t.Logf("three material thumbnails rendered with %s", s.Engine.Caps().Renderer)
}
