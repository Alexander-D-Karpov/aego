package editor

import (
	"bytes"
	stdctx "context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/anim"
	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/editor/core/artedit"
	"aego/editor/core/context"
)

// The real example is copied into a fresh project. No example or user file is
// edited: a new asset must not steal the PNG's existing frame identities.
func TestSlicedPNGAnimationCopyPreservesRealRunnerAndGuardsChanges(t *testing.T) {
	for _, scenario := range []string{"copy", "cancel", "source changed", "metadata changed", "metadata buffer changed", "target appeared", "target metadata appeared", "document closed", "invalid duration"} {
		t.Run(scenario, func(t *testing.T) {
			ed, _, p := importTestEditor(t)
			sourceDir := filepath.Join("..", "..", "..", "examples", "platformer", "assets", "surfaces")
			for _, name := range []string{"runner.png", "runner.png.meta", "runner_n.png", "runner_n.png.meta"} {
				b, err := os.ReadFile(filepath.Join(sourceDir, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p.AssetsDir(), name), b, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := ed.Pipeline.Scan(); err != nil {
				t.Fatal(err)
			}
			batch, err := ed.Pipeline.ImportAll()
			if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
				t.Fatalf("original import: %+v, %v", batch, err)
			}
			id, ok := ed.Pipeline.Lookup("assets/runner.png")
			if !ok {
				t.Fatal("real runner PNG was not indexed")
			}
			doc, err := ed.OpenAsset(id)
			if err != nil || doc.Handler != imageHandlerID {
				t.Fatalf("runner did not open in Image Viewer: %v", err)
			}
			pngPath := filepath.Join(p.AssetsDir(), "runner.png")
			metaPath := pngPath + pipeline.MetaSuffix
			original, err := os.ReadFile(pngPath)
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := os.ReadFile(metaPath)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := asset.ParseSourceMeta(metadata, metaPath)
			if err != nil || len(meta.Subs) != 8 {
				t.Fatalf("runner metadata: %+v, %v", meta, err)
			}
			if err := askPNGFramesArtCopy(ed, doc); err != nil {
				t.Fatal(err)
			}
			questions := ed.TakeQuestions()
			if len(questions) != 1 || questions[0].Then == nil || !strings.Contains(questions[0].Message, "normal") {
				t.Fatalf("missing confirmation or loss warning: %+v", questions)
			}
			if scenario == "cancel" {
				questions[0].Then(1)
				if len(ed.TakeQuestions()) != 0 {
					t.Fatal("cancel still opened a creation prompt")
				}
				if _, err := os.Stat(filepath.Join(p.AssetsDir(), "runner-animation.aeart")); !os.IsNotExist(err) {
					t.Fatalf("cancel created an animation copy: %v", err)
				}
				return
			}
			questions[0].Then(0)
			questions = ed.TakeQuestions()
			if len(questions) != 1 || questions[0].Submit == nil || len(questions[0].Values) != 2 {
				t.Fatalf("missing path and duration prompt after confirmation: %+v", questions)
			}
			target := filepath.Join(p.AssetsDir(), "runner-animation.aeart")
			var otherMeta []byte
			switch scenario {
			case "source changed":
				if err := os.WriteFile(pngPath, []byte("external PNG"), 0600); err != nil {
					t.Fatal(err)
				}
			case "metadata changed":
				if err := os.WriteFile(metaPath, []byte("external metadata"), 0600); err != nil {
					t.Fatal(err)
				}
			case "metadata buffer changed":
				doc.Metadata.Code.Replace(0, 0, "# draft\n")
			case "target appeared":
				if err := os.WriteFile(target, []byte("other work"), 0600); err != nil {
					t.Fatal(err)
				}
			case "target metadata appeared":
				m, err := ed.Pipeline.NewSourceMeta("assets/runner-animation.aeart")
				if err != nil {
					t.Fatal(err)
				}
				otherMeta = pipeline.WriteMeta(m)
				if err := os.WriteFile(target+pipeline.MetaSuffix, otherMeta, 0600); err != nil {
					t.Fatal(err)
				}
			case "document closed":
				if err := ed.Docs.CloseAsset(id, false); err != nil {
					t.Fatal(err)
				}
			}
			duration := "80"
			if scenario == "invalid duration" {
				duration = "0"
			}
			questions[0].Submit([]string{"assets/runner-animation.aeart", duration})
			if scenario != "copy" {
				if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
					t.Fatalf("unsafe copy was not reported: %+v", notice)
				}
				if scenario == "target appeared" {
					if got, err := os.ReadFile(target); err != nil || string(got) != "other work" {
						t.Fatalf("target was overwritten: %q, %v", got, err)
					}
				} else if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("failed copy created a target: %v", err)
				}
				if otherMeta != nil {
					if got, err := os.ReadFile(target + pipeline.MetaSuffix); err != nil || !bytes.Equal(got, otherMeta) {
						t.Fatalf("target metadata was overwritten: %v", err)
					}
				}
				return
			}
			copyID, ok := ed.Pipeline.Lookup("assets/runner-animation.aeart")
			if !ok || copyID == id {
				t.Fatal("animation copy did not get its own source identity")
			}
			copyDoc := ed.Docs.AssetDocument(copyID)
			if copyDoc == nil {
				t.Fatal("the copy did not open in Art Studio")
			}
			model, ok := copyDoc.Binding.Model.(*artedit.Document)
			if !ok {
				t.Fatal("the copy is not editable")
			}
			info := model.Session.Document.Info()
			if info.Canvas.Width != 16 || info.Canvas.Height != 16 || len(info.Frames) != 8 {
				t.Fatalf("wrong imported timeline: %+v", info)
			}
			originalImage, err := png.Decode(bytes.NewReader(original))
			if err != nil {
				t.Fatal(err)
			}
			for i, frame := range info.Frames {
				if frame.DurationMS != 80 {
					t.Fatalf("frame %d duration is %d", i, frame.DurationMS)
				}
				prepared := artsource.StableID(copyID, artsource.FrameName(frame.ID, art.Albedo))
				if prepared == meta.Subs[i].ID {
					t.Fatal("the new source stole an existing PNG frame ID")
				}
				if _, ok := ed.Pipeline.Index().Lookup(meta.Subs[i].ID); !ok {
					t.Fatal("the original scene frame disappeared from the index")
				}
				pixels, _, err := model.Capture(frame.ID, art.Albedo)
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < 16; y++ {
					for x := 0; x < 16; x++ {
						want := color.NRGBAModel.Convert(originalImage.At(meta.Subs[i].Rect[0]+x, meta.Subs[i].Rect[1]+y)).(color.NRGBA)
						got := pixels.NRGBAAt(x, y)
						// Compositing may normalize invisible RGB; visible pixels must match.
						if want.A != 0 && got != want {
							t.Fatalf("frame %d (%d,%d): got %v, want %v", i, x, y, got, want)
						}
					}
				}
			}
			clipID := artsource.StableID(copyID, artsource.ClipName(""))
			entry, ok := ed.Pipeline.Entry(clipID)
			if !ok {
				t.Fatal("animation clip was not imported")
			}
			artifact, ok := entry.Artifact(asset.ArtAnim)
			if !ok {
				t.Fatal("runtime animation artifact is missing")
			}
			data, err := ed.Pipeline.Artifacts().ReadFile(artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			clip, err := anim.Unmarshal(data)
			if err != nil || len(clip.Tracks) == 0 {
				t.Fatalf("runtime animation clip is invalid: %+v, %v", clip, err)
			}
			var sampler anim.Sampler
			sampler.Reset(clip)
			for i, frame := range info.Frames {
				want := artsource.StableID(copyID, artsource.FrameName(frame.ID, art.Albedo))
				if got := sampler.Track(clip, 0, float32(i)*0.08+0.04).A; got != want {
					t.Fatalf("runtime clip frame %d: got %s, want %s", i, got, want)
				}
			}
			before, _, _ := model.Capture(info.Frames[0].ID, art.Albedo)
			if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 0, Y: 0}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
				t.Fatal(err)
			}
			if err := model.SessionUndo(); err != nil {
				t.Fatal(err)
			}
			after, _, err := model.Capture(info.Frames[0].ID, art.Albedo)
			if err != nil || !bytes.Equal(before.Pix, after.Pix) {
				t.Fatalf("Undo did not restore imported pixels: %v", err)
			}
			if err := model.SessionRedo(); err != nil {
				t.Fatal(err)
			}
			if err := ed.SaveAsset(copyDoc); err != nil {
				t.Fatalf("save animation copy: %v", err)
			}
			if copyDoc.Dirty() {
				t.Fatal("saved animation copy remained dirty")
			}
			if err := ed.Docs.CloseAsset(copyID, false); err != nil {
				t.Fatal(err)
			}
			reopened, err := ed.OpenAsset(copyID)
			if err != nil {
				t.Fatal(err)
			}
			reopenedModel, ok := reopened.Binding.Model.(*artedit.Document)
			if !ok || len(reopenedModel.Session.Document.Info().Frames) != 8 || reopenedModel.Session.Document.Info().Frames[0].ID != info.Frames[0].ID {
				t.Fatal("saved animation did not reopen with stable frames")
			}
			if got, err := os.ReadFile(pngPath); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("original PNG changed: %v", err)
			}
			if got, err := os.ReadFile(metaPath); err != nil || !bytes.Equal(got, metadata) {
				t.Fatalf("original sidecar changed: %v", err)
			}
		})
	}
}
