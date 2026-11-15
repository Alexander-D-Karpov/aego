package editor

import (
	"image"
	"image/color"
	"testing"

	"aego/art"
)

func TestArtToolSampleUsesCoreWithoutChangingDrawing(t *testing.T) {
	p := artActionFixture(t)
	s := p.model.Session
	if err := s.SetSelection("ellipse", []image.Point{{0, 0}, {2, 2}}, 0); err != nil {
		t.Fatal(err)
	}
	selection, revision, history, dirty := s.Selection, s.Document.Revision(), p.model.History.Cursor(), p.doc.Dirty()
	brush := art.Brush{Size: 1, Source: "solid", Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}
	img, err := artRenderToolSample(s, brush, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.NRGBAAt(28, 26); got != (color.NRGBA{R: 255, A: 255}) {
		t.Fatalf("stroke sample did not use the selected paint: %+v", got)
	}
	if s.Selection != selection || s.Document.Revision() != revision || p.model.History.Cursor() != history || p.doc.Dirty() != dirty {
		t.Fatal("sample changed the active drawing, selection or history")
	}
	erased, err := artRenderToolSample(s, brush, 1, false)
	if err != nil || erased.NRGBAAt(28, 26).A != 0 || erased.NRGBAAt(0, 0).A == 0 {
		t.Fatalf("eraser sample did not reveal transparency: %v", err)
	}
}

func TestArtDeselectLeavesPixelsAndHistoryUntouched(t *testing.T) {
	p := artActionFixture(t)
	s := p.model.Session
	if err := s.SetSelection("ellipse", []image.Point{{0, 0}, {2, 2}}, 0); err != nil {
		t.Fatal(err)
	}
	revision, history, dirty := s.Document.Revision(), p.model.History.Cursor(), p.doc.Dirty()
	p.deselect()
	if p.err != nil || s.Selection != nil || s.Document.Revision() != revision || p.model.History.Cursor() != history || p.doc.Dirty() != dirty {
		t.Fatal("deselect changed the drawing or failed to clear the selection", p.err)
	}
}
