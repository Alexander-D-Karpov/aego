package art

import (
	"bytes"
	"context"
	"image"
	"reflect"
	"testing"
)

func TestMergeDownPreservesFramesChannelsLinksAndUndo(t *testing.T) {
	s, _ := sessionForTest(t)
	ctx := context.Background()
	base := s.Layer
	run := func(op Operation) ID {
		t.Helper()
		id, err := s.Execute(ctx, op)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	lower := run(Operation{Kind: "add-layer", Name: "lower", LayerKind: Raster})
	run(Operation{Kind: "stroke", Points: []image.Point{{3, 4}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{200, 10, 20, 255}}}})
	lowerCel := s.ActiveCel()
	run(Operation{Kind: "copy-cel", Cel: lowerCel, Layer: base, Linked: true})
	upper := run(Operation{Kind: "add-layer", Name: "upper", LayerKind: Raster})
	run(Operation{Kind: "stroke", Points: []image.Point{{3, 4}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{5, 20, 220, 128}}}})
	run(Operation{Kind: "stroke", Channel: Emission, Points: []image.Point{{3, 4}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{255, 90, 0, 0}}}})
	run(Operation{Kind: "duplicate-frame", Linked: true})
	before := s.Document.Info()
	images := map[string][]byte{}
	for _, frame := range before.Frames {
		for _, ch := range []Channel{Albedo, Emission} {
			im, err := s.Document.Render(frame.ID, RenderOptions{Channel: ch})
			if err != nil {
				t.Fatal(err)
			}
			images[string(frame.ID)+string(ch)] = im.Pix
		}
	}
	undo := s.Info().Undo
	if id := run(Operation{Kind: "merge-down", Layer: upper}); id != lower || s.Layer != lower || s.Info().Undo != undo+1 {
		t.Fatal("merge did not keep the lower layer and one history operation")
	}
	after := s.Document.Info()
	if len(after.Layers) != len(before.Layers)-1 || !reflect.DeepEqual(after.Frames, before.Frames) {
		t.Fatal("merge changed frame identities or retained the upper layer")
	}
	linked := map[Channel]ID{}
	for _, cel := range after.Cels {
		if cel.Layer == lower {
			if previous := linked[cel.Channel]; previous != "" && previous != cel.Image {
				t.Fatal("linked source pairs became independent merged frames")
			}
			linked[cel.Channel] = cel.Image
		}
		for _, old := range before.Cels {
			if old.ID == cel.ID && cel.Layer == base && old != cel {
				t.Fatal("merge rewrote a cel shared with another layer")
			}
		}
	}
	for _, frame := range after.Frames {
		for _, ch := range []Channel{Albedo, Emission} {
			im, err := s.Document.Render(frame.ID, RenderOptions{Channel: ch})
			if err != nil || !bytes.Equal(im.Pix, images[string(frame.ID)+string(ch)]) {
				t.Fatalf("merge changed %s frame output: %v", ch, err)
			}
		}
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	restored := s.Document.Info()
	if !reflect.DeepEqual(before.Layers, restored.Layers) || !reflect.DeepEqual(before.Cels, restored.Cels) {
		t.Fatal("Undo did not restore layer and linked-cel topology")
	}
	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Cels, s.Document.Info().Cels) {
		t.Fatal("Redo changed stable merged image identities")
	}
}

func TestLayerIsolationIsOnlyPresentation(t *testing.T) {
	s, _ := sessionForTest(t)
	ctx := context.Background()
	base := s.Layer
	stroke := Operation{Kind: "stroke", Points: []image.Point{{2, 2}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{255, 0, 0, 255}}}}
	if _, err := s.Execute(ctx, stroke); err != nil {
		t.Fatal(err)
	}
	group, err := s.Execute(ctx, Operation{Kind: "add-layer", Name: "group", LayerKind: Group})
	if err != nil {
		t.Fatal(err)
	}
	top, err := s.Execute(ctx, Operation{Kind: "add-layer", Name: "blue", LayerKind: Raster, Parent: group})
	if err != nil {
		t.Fatal(err)
	}
	stroke.Brush.Paint.Color = Color{0, 0, 255, 255}
	if _, err := s.Execute(ctx, stroke); err != nil {
		t.Fatal(err)
	}
	before := s.Info()
	if err := s.SetIsolation(base); err != nil {
		t.Fatal(err)
	}
	isolated, err := s.Preview("", false, false)
	if err != nil || isolated.NRGBAAt(2, 2).R != 255 {
		t.Fatal("isolation did not filter other groups")
	}
	if err = s.SetIsolation(top); err != nil {
		t.Fatal(err)
	}
	isolated, err = s.Preview("", false, false)
	if err != nil || isolated.NRGBAAt(2, 2).B != 255 {
		t.Fatal("nested isolation did not include ancestors")
	}
	if s.Document.Revision() != before.Document.Revision || s.Info().Undo != before.Undo {
		t.Fatal("isolation mutated canonical state/history")
	}
	exported, err := s.Document.Render(s.Frame, RenderOptions{})
	if err != nil || exported.NRGBAAt(2, 2).B != 255 {
		t.Fatal("view isolation leaked into normal export")
	}
}

func TestMergeRejectsBackdropDependentBlendWithoutPublishing(t *testing.T) {
	s, _ := sessionForTest(t)
	id, err := s.Execute(context.Background(), Operation{Kind: "add-layer", Name: "multiply", LayerKind: Raster})
	if err != nil {
		t.Fatal(err)
	}
	layer := s.Document.Info().Layers[1]
	layer.Blend = BlendMultiply
	if _, err = s.Execute(context.Background(), Operation{Kind: "set-layer", LayerValue: &layer}); err != nil {
		t.Fatal(err)
	}
	before := s.Info()
	if _, err = s.Execute(context.Background(), Operation{Kind: "merge-down", Layer: id}); err == nil {
		t.Fatal("backdrop-dependent merge silently changed semantics")
	}
	if s.Document.Revision() != before.Document.Revision || s.Info().Undo != before.Undo {
		t.Fatal("rejected merge modified the document")
	}
}
