package art

import (
	"context"
	"image"
	"testing"
)

func TestFramePivotCommandsPreservePriorityThroughSaveAndUndo(t *testing.T) {
	s, _ := sessionForTest(t)
	ctx := context.Background()
	frame := s.Frame
	specific := Slice{Name: "pivot", Frame: frame, Rect: image.Rect(0, 0, 32, 32), Pivot: image.Pt(8, 29)}
	id, err := s.Execute(ctx, Operation{Kind: "slice", Slice: &specific})
	if err != nil {
		t.Fatal(err)
	}
	global := Slice{Name: "pivot", Rect: specific.Rect, Pivot: image.Pt(16, 16)}
	if _, err = s.Execute(ctx, Operation{Kind: "slice", Slice: &global}); err != nil {
		t.Fatal(err)
	}
	data, err := s.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Decode(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if pivot, ok := loaded.Info().FramePivot(frame); !ok || pivot != specific.Pivot {
		t.Fatal("later global pivot overrode the frame-specific pivot")
	}
	if _, err = s.Execute(ctx, Operation{Kind: "delete-slice", Slice: &Slice{ID: id}}); err != nil {
		t.Fatal(err)
	}
	if pivot, ok := s.Document.Info().FramePivot(frame); !ok || pivot != global.Pivot {
		t.Fatal("removing the frame override did not reveal the global pivot")
	}
	if err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	if pivot, ok := s.Document.Info().FramePivot(frame); !ok || pivot != specific.Pivot {
		t.Fatal("Undo did not restore the frame-specific pivot")
	}
}
