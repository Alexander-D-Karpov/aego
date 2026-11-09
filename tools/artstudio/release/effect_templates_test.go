package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
)

func effectSources(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	for _, item := range art.EffectTemplates() {
		data, err := art.EffectTemplateBytes(item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(path, item.ID+art.SourceExtension), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestReleaseEffectCatalogUsesSharedEditableSourcesAndRealThumbnails(t *testing.T) {
	out := t.TempDir()
	fixtures := effectSources(t)
	items, err := writeEffectTemplates(out, fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 8 {
		t.Fatal("all eight effect templates are required")
	}
	seen := make(map[string]bool)
	for _, item := range items {
		shared, ok := art.EffectTemplateOf(item.ID)
		if !ok || seen[item.ID] || item.ParticlePreset != shared.ParticlePreset || item.Title["en"] != shared.NameEN || item.Title["ru"] != shared.NameRU {
			t.Fatal("release catalog diverged from shared native/web templates")
		}
		seen[item.ID] = true
		data, err := os.ReadFile(filepath.Join(out, item.File))
		if err != nil {
			t.Fatal(err)
		}
		source, err := art.EffectTemplateBytes(item.ID)
		if err != nil || !bytes.Equal(data, source) || digest(data) != item.SHA256 || len(data) != item.Bytes {
			t.Fatal("release source is not the same checked-in Art Document")
		}
		pngBytes, err := os.ReadFile(filepath.Join(out, item.Thumbnail))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			t.Fatal(err)
		}
		nonempty := 0
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				_, _, _, alpha := im.At(x, y).RGBA()
				if alpha != 0 {
					nonempty++
				}
			}
		}
		if nonempty < 4 {
			t.Fatalf("%s thumbnail has no meaningful artwork", item.ID)
		}
	}
	other, err := writeEffectTemplates(t.TempDir(), fixtures)
	if err != nil || !bytes.Equal(jsonBytes(items), jsonBytes(other)) {
		t.Fatal("template release catalog is not reproducible")
	}
	if err = writeLanding(out, map[string]string{"studio.css": "assets/studio.css", "surface.css": "assets/surface.css"}, nil, items, "template-build", true); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if !strings.Contains(string(page), `src="/r/template-build/`+item.Thumbnail+`"`) || !strings.Contains(string(page), `href="/r/template-build/editor/?new=effect&amp;template=`+item.ID+`"`) {
			t.Fatal("template starter/thumbnail does not point to the same versioned artifact")
		}
	}
}

func TestEffectManifestRejectsMissingDuplicateAndWrongPreset(t *testing.T) {
	dir, m, _ := fixtureArtifact(t, "effect-manifest")
	items, err := writeEffectTemplates(dir, effectSources(t))
	if err != nil {
		t.Fatal(err)
	}
	files, err := inventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Files, m.Templates = files, items
	if _, err = decodeManifest(jsonBytes(m)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing", "duplicate", "preset", "source"} {
		t.Run(kind, func(t *testing.T) {
			var broken buildManifest
			if err := json.Unmarshal(jsonBytes(m), &broken); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing":
				broken.Templates = broken.Templates[:7]
			case "duplicate":
				broken.Templates[7] = broken.Templates[0]
			case "preset":
				broken.Templates[0].ParticlePreset = "Unknown"
			case "source":
				broken.Templates[0].SHA256 = digest([]byte("different"))
			}
			if _, err := decodeManifest(jsonBytes(broken)); err == nil {
				t.Fatal("invalid template catalog was accepted")
			}
		})
	}
	m.Templates = nil // An already published artifact remains valid for rollback.
	if _, err = decodeManifest(jsonBytes(m)); err != nil {
		t.Fatal(err)
	}
}

func TestEffectReleaseReadsSelectedSourceTreeInsteadOfBuilderEmbeddedBytes(t *testing.T) {
	fixtures := effectSources(t)
	baseline, err := writeEffectTemplates(t.TempDir(), fixtures)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(fixtures, "dust.aeart")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := art.Decode(before, art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	info := doc.Info()
	session, err := art.NewSession(doc, art.NewJournal(art.Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	if err = session.Select(info.Layers[0].ID, info.Frames[len(info.Frames)/2].ID, art.Albedo); err != nil {
		t.Fatal(err)
	}
	if _, err = session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, B: 128, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	changed, err := doc.Encode()
	if err != nil || bytes.Equal(before, changed) {
		t.Fatal("fixture did not create a distinct valid source document")
	}
	if err = os.WriteFile(file, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	actual, err := writeEffectTemplates(out, fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if actual[0].SHA256 == baseline[0].SHA256 || actual[0].SHA256 != digest(changed) {
		t.Fatal("release catalog used the builder's embedded starter")
	}
	packaged, err := os.ReadFile(filepath.Join(out, actual[0].File))
	if err != nil || !bytes.Equal(packaged, changed) {
		t.Fatal("release bytes differ from the selected source tree")
	}
}

func TestEffectSourceTreeRejectsMissingExtraAndMismatchedDocuments(t *testing.T) {
	for _, kind := range []string{"missing", "extra", "identity", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			fixtures := effectSources(t)
			var err error
			switch kind {
			case "missing":
				err = os.Remove(filepath.Join(fixtures, "dust.aeart"))
			case "extra":
				err = os.WriteFile(filepath.Join(fixtures, "external.aeart"), []byte("not in the catalog"), 0o644)
			case "identity":
				var data []byte
				data, err = art.EffectTemplateBytes("dust")
				if err == nil {
					err = os.WriteFile(filepath.Join(fixtures, "fire.aeart"), data, 0o644)
				}
			case "invalid":
				err = os.WriteFile(filepath.Join(fixtures, "dust.aeart"), []byte("invalid document"), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = writeEffectTemplates(t.TempDir(), fixtures); err == nil {
				t.Fatal("incompatible template source tree accepted")
			}
		})
	}
}
