package gpupreview

import (
	"bytes"
	"image"
	"math"
	"reflect"
	"testing"

	"aego/art"
	"aego/core/mathx"
	"aego/modules/particles"
	"aego/scene"
)

func TestEffectPreviewUsesWholeRuntimePresetAndPreparedFlipbook(t *testing.T) {
	a, err := New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for index, template := range art.EffectTemplates() {
		t.Run(template.ID, func(t *testing.T) {
			doc, err := art.NewEffectDocument(template.ID, art.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			data, err := doc.Encode()
			if err != nil {
				t.Fatal(err)
			}
			if err = a.Publish(uint64(index+1), doc.Revision(), data); err != nil {
				t.Fatal(err)
			}
			o := DefaultOptions()
			o.Mode = "particles"
			if err = a.Configure(o); err != nil {
				t.Fatal(err)
			}
			raw, err := particles.PresetSource(template.ParticlePreset)
			if err != nil {
				t.Fatal(err)
			}
			want, err := particles.ReadPreset(a.types, a.loader, raw)
			if err != nil {
				t.Fatal(err)
			}
			pc, _ := scene.Get[particles.ParticleSystem2D](a.sc, a.emitter)
			if len(pc.Frames) != 8 || a.Status().EffectTemplate != template.ID || a.Status().ParticlePreset != template.ParticlePreset {
				t.Fatal("catalog did not select the actual runtime preset and eight frames")
			}
			for i, frame := range doc.Info().Frames {
				if pc.Frames[i].ID != a.data.frames[frame.ID][art.Albedo] {
					t.Fatal("particle flipbook is not the prepared authoring frame")
				}
			}
			// Only the image binding and timing may differ from the engine preset.
			want.Frames, want.Sprite, want.FrameFPS = pc.Frames, pc.Sprite, pc.FrameFPS
			if !reflect.DeepEqual(*pc, want) {
				t.Fatal("preview replaced preset behavior with a generic emitter")
			}
			service := particles.Of(a.world)
			peak := service.Count(a.emitter)
			for range 60 {
				a.Step()
				peak = max(peak, service.Count(a.emitter))
			}
			if peak == 0 || service.Emitted(a.emitter) == 0 {
				t.Fatal("real CPU particle service did not emit the template")
			}
			zero := float32(0)
			o.RateOverride = &zero
			if err = a.Configure(o); err != nil {
				t.Fatal(err)
			}
			want.Rate = 0
			if !reflect.DeepEqual(*pc, want) {
				t.Fatal("rate override discarded bursts, curves or another preset property")
			}
			if err = a.RestartParticles(); err != nil {
				t.Fatal(err)
			}
			after, err := doc.Encode()
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("preview/restart mutated the Art Document")
			}
		})
	}
}

func TestEffectPreviewRejectsInvalidConfigurationBeforePublishing(t *testing.T) {
	d, data := fixture(t)
	a, err := New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.Publish(1, d.Revision(), data); err != nil {
		t.Fatal(err)
	}
	before := a.options
	o := DefaultOptions()
	o.Mode, o.EffectTemplate = "particles", "https://arbitrary.example/effect"
	if err = a.Configure(o); err == nil || !reflect.DeepEqual(a.options, before) {
		t.Fatal("unknown preset mutated the active preview")
	}
	o.EffectTemplate = "explosion"
	nan := float32(math.NaN())
	o.RateOverride = &nan
	if err = a.Configure(o); err == nil || !reflect.DeepEqual(a.options, before) {
		t.Fatal("non-finite override mutated the active preview")
	}
	if err = a.RestartParticles(); err == nil {
		t.Fatal("restart accepted a sprite-only preview")
	}
}

func TestTemplatePixelPublicationDoesNotResetPreviewParameters(t *testing.T) {
	d, err := art.NewEffectDocument("explosion", art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	data, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Publish(1, d.Revision(), data); err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.Mode = "particles"
	if err = a.Configure(o); err != nil {
		t.Fatal(err)
	}
	pc, _ := scene.Get[particles.ParticleSystem2D](a.sc, a.emitter)
	// A host may expose more emitter fields than the compact web toolbar.
	// Rebinding edited texture pages must preserve those private preview edits.
	pc.Rate, pc.Gravity, pc.Size = 7, mathx.V2(3, 4), mathx.V2(.5, 1.5)
	o.Frame = d.Info().Frames[1].ID
	if err = a.Configure(o); err != nil {
		t.Fatal(err)
	}
	s, err := art.NewSession(d, art.NewJournal(art.Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BeginGesture("pencil", image.Pt(1, 1), art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}, false); err != nil {
		t.Fatal(err)
	}
	if err = s.EndGesture(); err != nil {
		t.Fatal(err)
	}
	data, err = d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Publish(2, d.Revision(), data); err != nil {
		t.Fatal(err)
	}
	if pc.Rate != 7 || pc.Gravity != mathx.V2(3, 4) || pc.Size != mathx.V2(.5, 1.5) || a.options.Frame != o.Frame {
		t.Fatal("pixel/frame update reapplied defaults to the preview emitter")
	}
}

func TestManualDustRestartUsesRuntimeBurstWithoutChangingPreset(t *testing.T) {
	doc, err := art.NewEffectDocument("dust", art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	data, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.Publish(1, doc.Revision(), data); err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.Mode = "particles"
	if err = a.Configure(o); err != nil {
		t.Fatal(err)
	}
	pc, _ := scene.Get[particles.ParticleSystem2D](a.sc, a.emitter)
	if pc.PlayOnStart || pc.Rate != 0 || len(pc.Bursts) != 0 {
		t.Fatal("manual Dust preset was changed to an automatic emitter")
	}
	service := particles.Of(a.world)
	if service.Count(a.emitter) != 8 || service.Emitted(a.emitter) != 8 {
		t.Fatal("manual Dust preview did not send the native Burst(8) input event")
	}
	for range 60 {
		a.Step()
	}
	if service.Count(a.emitter) != 0 {
		t.Fatal("manual Dust did not expire")
	}
	if err = a.RestartParticles(); err != nil {
		t.Fatal(err)
	}
	if service.Count(a.emitter) != 8 || service.Emitted(a.emitter) != 8 || pc.PlayOnStart || pc.Rate != 0 || len(pc.Bursts) != 0 {
		t.Fatal("repeated restart failed or modified source preset settings")
	}
}
