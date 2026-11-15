package editor

import (
	"bytes"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/modules/particles"
)

func TestEffectTemplatesCreateLinkedPresetWithStableFrames(t *testing.T) {
	f := newDrawingFixture(t)
	for _, template := range art.EffectTemplates() {
		t.Run(template.ID, func(t *testing.T) {
			builtin, err := particles.PresetSource(template.ParticlePreset)
			if err != nil {
				t.Fatal(err)
			}
			base, err := particles.ReadPreset(f.ed.Types, f.ed.Loader, builtin)
			if err != nil {
				t.Fatalf("invalid built-in particle preset: %v", err)
			}
			rel := "assets/" + template.ID + ".aeart"
			edits, err := effectSourceEdits(f.ed, rel, template.ID)
			if err != nil {
				t.Fatal(err)
			}
			var history assetops.Ops
			created, err := history.CreateSources(f.ed, edits)
			if err != nil || len(created) != 2 {
				t.Fatalf("create source pair: %v %v", created, err)
			}
			data, err := f.ed.Pipeline.SourceBytes(rel)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := art.Decode(data, art.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			preset, err := particles.LoadPreset(f.ed.Types, f.ed.Loader, created[1])
			if err != nil || len(preset.Frames) != 8 || preset.FrameFPS != 10 {
				t.Fatalf("imported particle flipbook: %+v %v", preset, err)
			}
			if preset.Rate != base.Rate || preset.Loop != base.Loop || preset.Shape != base.Shape || len(preset.Bursts) != len(base.Bursts) || preset.Material.ID != base.Material.ID {
				t.Fatal("binding the artwork replaced the actual emitter preset")
			}
			for i, frame := range doc.Info().Frames {
				want := artsource.StableID(created[0], artsource.FrameName(frame.ID, art.Albedo))
				if preset.Frames[i].ID != want {
					t.Fatalf("frame %d changed its stable identity", i)
				}
				if _, ok := f.ed.Loader.Texture(want); !ok {
					t.Fatalf("frame %d is not a real imported sprite", i)
				}
			}
			if preset.Sprite.ID != preset.Frames[0].ID {
				t.Fatal("preset still uses an unrelated stock sprite")
			}
			before := map[string][]byte{}
			for _, edit := range edits {
				before[edit.Path], err = f.ed.Pipeline.SourceBytes(edit.Path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := history.Undo(f.ed); err != nil {
				t.Fatal(err)
			}
			for path := range before {
				if got, err := f.ed.Pipeline.SourceBytes(path); err != nil || got != nil {
					t.Fatalf("undo left a partial pair: %s %v", path, err)
				}
			}
			if err := history.Redo(f.ed); err != nil {
				t.Fatal(err)
			}
			for path, want := range before {
				got, _ := f.ed.Pipeline.SourceBytes(path)
				if !bytes.Equal(got, want) {
					t.Fatalf("redo changed linked source data: %s", path)
				}
			}
		})
	}
	// Rain is a pre-existing emitter preset, not one of the eight new drawing
	// templates. Extending authoring must not remove or reinterpret it.
	rain, err := particles.PresetSource("Rain")
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := particles.ReadPreset(f.ed.Types, f.ed.Loader, rain); err != nil || cfg.Rate != 48 {
		t.Fatalf("existing Rain preset regressed: %+v %v", cfg, err)
	}
}

func TestEffectSourcesRejectUnknownTemplateAndWrongExtension(t *testing.T) {
	f := newDrawingFixture(t)
	for _, item := range [][2]string{{"assets/test.aeart", "../../fire"}, {"assets/test.png", "fire"}} {
		if _, err := effectSourceEdits(f.ed, item[0], item[1]); err == nil {
			t.Fatal("invalid authoring request was accepted", item)
		}
		if data, err := f.ed.Pipeline.SourceBytes(item[0]); err != nil || data != nil {
			t.Fatal("preparation wrote source files", err)
		}
	}
	if _, err := f.ed.Pipeline.SourceBytes("assets/test.aeart" + pipeline.MetaSuffix); err != nil {
		t.Fatal(err)
	}
}

func TestNewEffectPromptRejectsProjectSwitch(t *testing.T) {
	f := newDrawingFixture(t)
	called := false
	f.ed.CreateAssetSources = func([]pipeline.SourceEdit) ([]ids.AssetID, error) { called = true; return nil, nil }
	a := &adapter{ed: f.ed}
	if err := a.askNewEffect("assets"); err != nil {
		t.Fatal(err)
	}
	q := f.ed.TakeQuestions()
	if len(q) != 1 || q[0].Submit == nil {
		t.Fatal("missing creation prompt")
	}
	f.ed.Project = nil
	q[0].Submit([]string{"assets/new.aeart", "Explosion"})
	if called {
		t.Fatal("project switch created files")
	}
	if notice, ok := f.ed.LastNotice(); !ok || notice.Level != context.LevelError {
		t.Fatal("project switch was not reported")
	}
}
