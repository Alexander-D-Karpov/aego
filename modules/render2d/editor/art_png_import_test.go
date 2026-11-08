package editor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset/pipeline"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
)

func TestPNGEditableCopyPreservesOriginalAndGuardsChanges(t *testing.T) {
	for _, scenario := range []string{"copy", "source changed", "target appeared", "target metadata appeared", "document closed"} {
		t.Run(scenario, func(t *testing.T) {
			ed, _, p := importTestEditor(t)
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			want := color.NRGBA{R: 200, G: 120, B: 80, A: 200}
			img.SetNRGBA(1, 1, want)
			var original bytes.Buffer
			if err := png.Encode(&original, img); err != nil {
				t.Fatal(err)
			}
			id, err := assetops.CreateFile(ed, "assets/source.png", original.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			doc, err := ed.OpenAsset(id)
			if err != nil || doc.Handler != imageHandlerID || !canCopyPNGToArt(ed, doc) {
				t.Fatalf("PNG did not open in Image Viewer: %+v, %v", doc, err)
			}
			meta := filepath.Join(p.Root, "assets", "source.png.meta")
			metaBefore, err := os.ReadFile(meta)
			if err != nil {
				t.Fatal(err)
			}
			if err := askPNGArtCopy(ed, doc); err != nil {
				t.Fatal(err)
			}
			q := ed.TakeQuestions()
			if len(q) != 1 || q[0].Submit == nil || q[0].Values[0] != "assets/source.aeart" {
				t.Fatalf("missing copy confirmation and default path: %+v", q)
			}
			pngPath := filepath.Join(p.Root, "assets", "source.png")
			target := filepath.Join(p.Root, "assets", "source.aeart")
			var otherMeta []byte
			switch scenario {
			case "source changed":
				if err := os.WriteFile(pngPath, []byte("external edit"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "target appeared":
				if err := os.WriteFile(target, []byte("other work"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "target metadata appeared":
				meta, err := ed.Pipeline.NewSourceMeta("assets/source.aeart")
				if err != nil {
					t.Fatal(err)
				}
				otherMeta = pipeline.WriteMeta(meta)
				if err := os.WriteFile(target+pipeline.MetaSuffix, otherMeta, 0600); err != nil {
					t.Fatal(err)
				}
			case "document closed":
				if err := ed.Docs.CloseAsset(id, false); err != nil {
					t.Fatal(err)
				}
			}
			q[0].Submit([]string{"assets/source.aeart"})
			if scenario != "copy" {
				if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
					t.Fatalf("unsafe copy was not reported: %+v", notice)
				}
				if scenario == "target appeared" {
					if data, err := os.ReadFile(target); err != nil || string(data) != "other work" {
						t.Fatalf("existing target was overwritten: %q, %v", data, err)
					}
				} else if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("unsafe copy created a target: %v", err)
				}
				if otherMeta != nil {
					if got, err := os.ReadFile(target + pipeline.MetaSuffix); err != nil || !bytes.Equal(got, otherMeta) {
						t.Fatalf("existing target metadata was overwritten: %v", err)
					}
				}
				return
			}
			copyID, ok := ed.Pipeline.Lookup("assets/source.aeart")
			if !ok || copyID == id {
				t.Fatal("copy did not receive its own asset identity")
			}
			copyDoc := ed.Docs.AssetDocument(copyID)
			if copyDoc == nil {
				t.Fatal("copy did not open in Art Studio")
			}
			model, ok := copyDoc.Binding.Model.(*artedit.Document)
			if !ok {
				t.Fatal("copy is not editable")
			}
			frame := model.Session.Document.Info().Frames[0].ID
			pixels, _, err := model.Capture(frame, art.Albedo)
			if err != nil || pixels.NRGBAAt(1, 1) != want {
				t.Fatalf("PNG pixels not preserved in copy: %v, %v", pixels, err)
			}
			if data, err := os.ReadFile(pngPath); err != nil || !bytes.Equal(data, original.Bytes()) {
				t.Fatalf("original PNG changed: %v", err)
			}
			if data, err := os.ReadFile(meta); err != nil || !bytes.Equal(data, metaBefore) {
				t.Fatalf("original PNG metadata changed: %v", err)
			}
		})
	}
}
