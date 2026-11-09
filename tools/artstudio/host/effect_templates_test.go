package host

import (
	"bytes"
	"image"
	"testing"

	"aego/art"
)

func TestEffectTemplatesUseOrdinaryDirtyDocumentsAndIndependentHistory(t *testing.T) {
	h := New()
	for _, template := range art.EffectTemplates() {
		t.Run(template.ID, func(t *testing.T) {
			a, err := h.NewEffect(template.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			b, err := h.NewEffect(template.ID, template.NameEN+".aeart")
			if err != nil {
				t.Fatal(err)
			}
			sa, sb := session(t, h, a), session(t, h, b)
			if a == b || sa.Document.Info().ID == sb.Document.Info().ID || !sa.Info().Dirty || !sb.Info().Dirty || sa.Info().Undo != 0 || sb.Info().Undo != 0 {
				t.Fatal("new template instances must have independent dirty document owners")
			}
			info := sa.Document.Info()
			if info.Properties[art.EffectTemplateProperty] != template.ID || len(info.Frames) != 8 {
				t.Fatal("shared template structure was lost")
			}
			before, err := sa.Document.PNG(sa.Frame, art.Albedo)
			if err != nil {
				t.Fatal(err)
			}
			draw(t, sa, 1)
			after, _ := sa.Document.PNG(sa.Frame, art.Albedo)
			other, _ := sb.Document.PNG(sb.Frame, art.Albedo)
			if bytes.Equal(before, after) || !bytes.Equal(before, other) || sa.Info().Undo != 1 || sb.Info().Undo != 0 {
				t.Fatal("template edit did not use independent canonical pixels and History")
			}
			if err = sa.Undo(); err != nil {
				t.Fatal(err)
			}
			restored, _ := sa.Document.PNG(sa.Frame, art.Albedo)
			if !bytes.Equal(restored, before) {
				t.Fatal("template Undo did not restore pixels")
			}
			saved := export(t, h, a)
			reopened, err := art.Decode(saved.Bytes, art.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			if reopened.Info().Properties[art.EffectTemplateProperty] != template.ID || len(reopened.Info().Frames) != 8 {
				t.Fatal("template identity/frames lost in native-compatible source")
			}
			reopened.Close()
			if err = h.Close(a, true); err != nil {
				t.Fatal(err)
			}
			if err = h.Close(b, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEffectCreationRejectsUnknownIDAndActiveGesture(t *testing.T) {
	h := New()
	key := document(t, h, "current.aeart")
	s := session(t, h, key)
	if _, err := h.NewEffect("../../external", ""); err == nil || h.Active() != key || len(h.Tabs()) != 1 {
		t.Fatal("unknown template changed open documents")
	}
	if err := s.BeginGesture("pencil", image.Pt(1, 1), art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.NewEffect("explosion", ""); err == nil || h.Active() != key || len(h.Tabs()) != 1 || !s.Info().Gesture {
		t.Fatal("New Effect orphaned an unfinished stroke")
	}
	s.CancelGesture()
}
