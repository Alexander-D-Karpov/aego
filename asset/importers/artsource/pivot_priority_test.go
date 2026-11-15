package artsource

import (
	"image"
	"testing"

	"aego/art"
	"aego/asset"
)

func TestImportedFramePivotOverridesLaterGlobalPivot(t *testing.T) {
	ctx, session := testSource(t)
	defer session.Document.Close()
	frame := session.Frame
	e, err := session.Document.Begin(session.Document.Revision(), "pivot priority")
	if err != nil {
		t.Fatal(err)
	}
	for _, slice := range []art.Slice{
		{Name: "pivot", Frame: frame, Rect: image.Rect(0, 0, 16, 16), Pivot: image.Pt(4, 12)},
		{Name: "pivot", Rect: image.Rect(0, 0, 16, 16), Pivot: image.Pt(8, 8)},
	} {
		if _, err := e.SetSlice(slice); err != nil {
			t.Fatal(err)
		}
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	ctx.Source, err = session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	result, err := New().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range result.Subs {
		if sub.Name != FrameName(frame, art.Albedo) {
			continue
		}
		for _, artifact := range sub.Artifacts {
			if artifact.Kind != asset.ArtSpriteRegion {
				continue
			}
			region, err := asset.DecodeRegion(artifact.Data)
			if err != nil || !region.HasPivot || region.Pivot != ([2]float32{0.25, 0.75}) {
				t.Fatalf("frame-specific pivot was overridden: %+v, %v", region, err)
			}
			return
		}
	}
	t.Fatal("import did not produce the frame region")
}
