package tryassets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
)

const normalExample = "../../../modules/render2d/res/docs/try/lighting/NormalMapping.assets"

func TestNormalTryPreparedDeterministicallyFromSources(t *testing.T) {
	first, err := Prepare(normalExample)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Prepare(normalExample)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("fresh import is not reproducible", err)
	}
	bundle, err := asset.DecodeTryArtifacts(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Assets) != 3 {
		t.Fatalf("expected albedo, height-derived normal and material, got %d", len(bundle.Assets))
	}
	normalFound, materialFound := false, false
	for _, entry := range bundle.Assets {
		for _, artifact := range entry.Artifacts {
			switch artifact.Kind {
			case asset.ArtTexture:
				texture, err := asset.DecodeTexture(artifact.Data)
				if err != nil {
					t.Fatal(err)
				}
				if texture.Semantic == asset.TextureNormal {
					changed := 0
					for i := 0; i < len(texture.Pixels); i += 4 {
						if texture.Pixels[i] != 128 || texture.Pixels[i+1] != 128 {
							changed++
						}
					}
					if changed < 1000 {
						t.Fatalf("normal Try has a flat placeholder: %d changed texels", changed)
					}
					normalFound = true
				}
			case asset.ArtMaterial:
				d, err := asset.DecodeMaterial(artifact.Data, entry.Name)
				if err != nil || d.Shader != asset.BuiltinShaderLit || d.Textures["NormalMap"].IsZero() {
					t.Fatal("prepared material lost normal binding", err)
				}
				materialFound = true
			}
		}
	}
	if !normalFound || !materialFound {
		t.Fatal("incomplete normal mapping Try")
	}
	if _, err := Generate(normalExample, true); err != nil {
		t.Fatal(err)
	}
}

func TestTryPreparationDetectsSettingsChangesAndPreservesSourceTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Test.assets")
	if err := os.CopyFS(root, os.DirFS(normalExample)); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".aego")); !os.IsNotExist(err) {
		t.Fatal("preparation changed source project cache")
	}
	meta := filepath.Join(root, "assets/surface_height.png.meta")
	data, err := os.ReadFile(meta)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("height_strength=12.0"), []byte("height_strength=2.0"))
	if err := os.WriteFile(meta, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(root, true); err == nil {
		t.Fatal("source settings change did not invalidate prepared payload")
	}
	if _, err := Generate(root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(root, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(meta); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(root); err == nil {
		t.Fatal("missing metadata silently allocated a new identity")
	}
}

func TestTryPreparationRejectsSymlinksAndOversizedSources(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Test.assets")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(normalExample, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(root); err == nil {
		t.Fatal("source symlink accepted")
	}
	if err := os.Remove(filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.png"), make([]byte, asset.MaxTryArtifactBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(root); err == nil {
		t.Fatal("oversized source accepted")
	}
}
