package art

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestEffectTemplatesAreEditableAnimatedSources(t *testing.T) {
	if len(EffectTemplates()) != 8 {
		t.Fatal("the authoring catalogue must expose all eight promised effects")
	}
	seen := map[string]bool{}
	for _, template := range EffectTemplates() {
		t.Run(template.ID, func(t *testing.T) {
			if seen[template.ID] || template.NameEN == "" || template.NameRU == "" || template.ParticlePreset == "" {
				t.Fatal("duplicate or incomplete localized template")
			}
			seen[template.ID] = true
			data, err := EffectTemplateBytes(template.ID)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Decode(data, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			info := doc.Info()
			if info.Properties[EffectTemplateProperty] != template.ID || info.Canvas.Width != 32 || info.Canvas.Height != 32 || len(info.Frames) != 8 || len(info.Layers) < 2 {
				t.Fatalf("not a complete, editable starter: %+v", info)
			}
			if len(info.Tags) != 1 || info.Tags[0].From != info.Frames[0].ID || info.Tags[0].To != info.Frames[7].ID || info.Tags[0].Direction != Forward {
				t.Fatal("starter lost its full-range playback tag")
			}
			frames := map[[32]byte]bool{}
			var emissionPixels int
			for _, frame := range info.Frames {
				if frame.DurationMS != 100 {
					t.Fatal("starter must have a uniform 10 FPS flipbook timing")
				}
				if pivot, ok := info.FramePivot(frame.ID); !ok || pivot.X < 0 || pivot.Y < 0 || pivot.X >= 32 || pivot.Y >= 32 {
					t.Fatal("starter has no usable pivot")
				}
				pixels, err := doc.Render(frame.ID, RenderOptions{})
				if err != nil {
					t.Fatal(err)
				}
				opaque := 0
				for i := 3; i < len(pixels.Pix); i += 4 {
					if pixels.Pix[i] != 0 {
						opaque++
					}
				}
				if opaque < 5 || opaque > 700 {
					t.Fatalf("frame is empty or lacks a transparent silhouette: %d", opaque)
				}
				frames[sha256.Sum256(pixels.Pix)] = true
				emission, err := doc.Render(frame.ID, RenderOptions{Channel: Emission})
				if err != nil {
					t.Fatal(err)
				}
				for i := 0; i < len(emission.Pix); i += 4 {
					if emission.Pix[i+3] > 0 && (emission.Pix[i] > 0 || emission.Pix[i+1] > 0 || emission.Pix[i+2] > 0) {
						emissionPixels++
					}
				}
			}
			if len(frames) < 4 {
				t.Fatal("effect must animate its artwork, not repeat a static image")
			}
			expectEmission := template.ID != "dust" && template.ID != "smoke"
			if (emissionPixels > 0) != expectEmission {
				t.Fatalf("incorrect emission channel for %s", template.ID)
			}
			encoded, err := doc.Encode()
			if err != nil || !bytes.Equal(data, encoded) {
				t.Fatalf("starter does not round-trip canonically: %v", err)
			}
		})
	}
}

func TestNewEffectsHaveIndependentIdentityAndPixels(t *testing.T) {
	a, err := NewEffectDocument("fire", Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := NewEffectDocument("fire", Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.Info().ID == b.Info().ID {
		t.Fatal("New Effect reused document identity")
	}
	untouched, _ := b.Encode()
	starter, _ := EffectTemplateBytes("fire")
	cel := a.Info().Cels[0].ID
	e, _ := a.Begin(a.Revision(), "edit this instance")
	if err := e.SetPixel(cel, 0, 0, Paint{Color: Color{1, 2, 3, 255}}, nil, false); err != nil {
		t.Fatal(err)
	}
	c, err := e.Prepare()
	if err != nil || c == nil {
		t.Fatalf("edit: %v", err)
	}
	if err := c.Apply(); err != nil {
		t.Fatal(err)
	}
	after, _ := b.Encode()
	reference, _ := EffectTemplateBytes("fire")
	if !bytes.Equal(untouched, after) || !bytes.Equal(starter, reference) {
		t.Fatal("editing an instance changed another document or the template")
	}
	catalogue := EffectTemplates()
	catalogue[0].ID = "changed"
	if _, ok := EffectTemplateOf("dust"); !ok {
		t.Fatal("caller mutated the shared catalogue")
	}
	if _, err := NewEffectDocument("../fire", Limits{}); err == nil {
		t.Fatal("template IDs were treated as arbitrary paths")
	}
	if _, err := NewEffectDocument("fire", Limits{MaxFrames: 4}); err == nil {
		t.Fatal("template creation bypassed document limits")
	}
}
