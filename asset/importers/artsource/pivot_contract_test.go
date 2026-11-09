package artsource

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"

	"aego/art"
	"aego/asset"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
)

func pivotContractRegion(t *testing.T, result *pipeline.Result, root, id ids.AssetID) (*asset.RegionArtifact, []ids.AssetID) {
	t.Helper()
	artifacts, deps := result.Artifacts, result.Deps
	if id != root {
		found := false
		for _, sub := range result.Subs {
			if sub.ID == id {
				artifacts, deps, found = sub.Artifacts, sub.Deps, true
				break
			}
		}
		if !found {
			t.Fatalf("missing region subasset %s", id)
		}
	}
	for _, artifact := range artifacts {
		if artifact.Kind == asset.ArtSpriteRegion {
			region, err := asset.DecodeRegion(artifact.Data)
			if err != nil {
				t.Fatal(err)
			}
			return region, deps
		}
	}
	t.Fatalf("missing region artifact for %s", id)
	return nil, nil
}

// PNG pivots are relative to each source rectangle; generated Art regions use
// the full Art canvas. Unchanged fractional PNG metadata must not be quantized.
func TestLinkedPivotCoordinateContract(t *testing.T) {
	for _, tc := range []struct {
		name       string
		width      int
		height     int
		rootPivot  [2]float32
		rootHas    bool
		sliced     bool
		wantPixels []image.Point
	}{
		{name: "whole fractional", width: 5, height: 7, rootPivot: [2]float32{.33, .61}, rootHas: true, wantPixels: []image.Point{{2, 4}}},
		{name: "whole default", width: 5, height: 7, wantPixels: []image.Point{{3, 4}}},
		{name: "different frame sizes", width: 9, height: 7, sliced: true, wantPixels: []image.Point{{1, 2}, {3, 4}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, tc.width, tc.height))
			for y := 0; y < tc.height; y++ {
				for x := 0; x < tc.width; x++ {
					img.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x), G: uint8(40 + y), B: 60, A: 255})
				}
			}
			var pngBytes bytes.Buffer
			if err := png.Encode(&pngBytes, img); err != nil {
				t.Fatal(err)
			}
			meta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
			meta.Pivot, meta.HasPivot = tc.rootPivot, tc.rootHas
			if tc.sliced {
				meta.Subs = []pipeline.SubAsset{
					{ID: ids.NewAssetID(), Name: "small", Rect: [4]int{0, 0, 3, 4}, Pivot: [2]float32{.375, .375}, HasPivot: true},
					{ID: ids.NewAssetID(), Name: "large", Rect: [4]int{4, 0, 5, 7}},
				}
			}
			ctx := &pipeline.ImportContext{ID: meta.ID, Path: "assets/pivot.png", Source: bytes.Clone(pngBytes.Bytes()), Meta: meta}
			beforePNG, beforeMeta := bytes.Clone(ctx.Source), pipeline.WriteMeta(meta)
			original, err := texImporter.New().Import(ctx)
			if err != nil || original == nil || original.Error != nil {
				t.Fatalf("original texture import: %+v, %v", original, err)
			}
			doc, mapping, err := ImportTextureDocument(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			if !bytes.Equal(ctx.Source, beforePNG) || !bytes.Equal(pipeline.WriteMeta(meta), beforeMeta) {
				t.Fatal("opening the PNG changed its bytes or metadata")
			}
			if len(mapping) != len(tc.wantPixels) {
				t.Fatalf("frame count = %d, want %d", len(mapping), len(tc.wantPixels))
			}
			for i, frame := range mapping {
				wantID := meta.ID
				if tc.sliced {
					wantID = meta.Subs[i].ID
				}
				pivot, ok := doc.Info().FramePivot(frame.Frame)
				if frame.Asset != wantID || !ok || pivot != tc.wantPixels[i] {
					t.Fatalf("frame %d identity/pixel pivot = %+v, %v, %v; want %s, %v", i, frame, pivot, ok, wantID, tc.wantPixels[i])
				}
			}

			artMeta := pipeline.NewMeta("artsource", Version, New().Defaults())
			channelIDs := map[art.Channel]ids.AssetID{}
			for _, channel := range []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission} {
				id := ids.NewAssetID()
				channelIDs[channel] = id
				artMeta.Subs = append(artMeta.Subs, pipeline.SubAsset{Name: FrameName(mapping[0].Frame, channel), ID: id})
			}
			encoded, err := doc.Encode()
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"assets/pivot.aeart": encoded, "assets/pivot.aeart.meta": pipeline.WriteMeta(artMeta)}
			ctx.Read = func(name string) ([]byte, error) {
				if data, ok := files[name]; ok {
					return bytes.Clone(data), nil
				}
				return nil, fmt.Errorf("missing fixture %s", name)
			}
			ctx.SourcePath = func(id ids.AssetID) (string, bool) { return "assets/pivot.aeart", id == artMeta.ID }
			if err := SetTextureLink(meta, artMeta.ID, ctx.Source, mapping); err != nil {
				t.Fatal(err)
			}
			linkedMeta := pipeline.WriteMeta(meta)
			initial, err := NewTexture().Import(ctx)
			if err != nil || initial == nil || initial.Error != nil {
				t.Fatalf("initial linked import: %+v, %v", initial, err)
			}
			for _, frame := range mapping {
				before, _ := pivotContractRegion(t, original, meta.ID, frame.Asset)
				after, _ := pivotContractRegion(t, initial, meta.ID, frame.Asset)
				if before.Pivot != after.Pivot || before.HasPivot != after.HasPivot || before.X != after.X || before.Y != after.Y || before.W != after.W || before.H != after.H {
					t.Fatalf("initial link changed source geometry: before=%+v after=%+v", before, after)
				}
			}

			info := doc.Info()
			var pivotSlice art.Slice
			for _, candidate := range info.Slices {
				if candidate.Name == "pivot" && candidate.Frame == mapping[0].Frame {
					pivotSlice = candidate
				}
			}
			if pivotSlice.ID == "" {
				t.Fatal("missing frame-specific pivot slice")
			}
			pivotSlice.Pivot = pivotSlice.Pivot.Add(image.Pt(1, 1))
			edit, err := doc.Begin(doc.Revision(), "edit pivot and surface channels")
			if err != nil {
				t.Fatal(err)
			}
			defer edit.Cancel()
			if _, err := edit.SetSlice(pivotSlice); err != nil {
				t.Fatal(err)
			}
			for _, channel := range []art.Channel{art.Normal, art.Height, art.Emission} {
				cel, err := edit.EnsureCel(info.Layers[0].ID, mapping[0].Frame, channel)
				if err != nil {
					t.Fatal(err)
				}
				if err := edit.SetPixel(cel, 0, 0, art.Paint{Color: art.Color{R: 80, G: 100, B: 120, A: 255}}, nil, false); err != nil {
					t.Fatal(err)
				}
			}
			change, err := edit.Prepare()
			if err != nil || change == nil {
				t.Fatalf("prepare pivot edit: %v", err)
			}
			if err := change.Apply(); err != nil {
				t.Fatal(err)
			}
			files["assets/pivot.aeart"], err = doc.Encode()
			if err != nil {
				t.Fatal(err)
			}
			linked, err := NewTexture().Import(ctx)
			if err != nil || linked == nil || linked.Error != nil {
				t.Fatalf("edited linked import: %+v, %v", linked, err)
			}
			prepared, err := New().Import(&pipeline.ImportContext{ID: artMeta.ID, Path: "assets/pivot.aeart", Source: files["assets/pivot.aeart"], Meta: artMeta})
			if err != nil || prepared == nil || prepared.Error != nil {
				t.Fatalf("edited art import: %+v, %v", prepared, err)
			}
			region, deps := pivotContractRegion(t, linked, meta.ID, mapping[0].Asset)
			w, h := tc.width, tc.height
			if tc.sliced {
				w, h = 3, 4
			}
			wantLinked := [2]float32{float32(pivotSlice.Pivot.X) / float32(w), float32(pivotSlice.Pivot.Y) / float32(h)}
			if !region.HasPivot || region.Pivot != wantLinked || region.Normal != channelIDs[art.Normal] || region.Emission != channelIDs[art.Emission] {
				t.Fatalf("linked region changed pivot or surface identity: %+v, want pivot %v", region, wantLinked)
			}
			if !slices.Contains(deps, channelIDs[art.Normal]) || !slices.Contains(deps, channelIDs[art.Emission]) {
				t.Fatalf("linked region lost surface dependencies: %v", deps)
			}
			wantArt := [2]float32{float32(pivotSlice.Pivot.X) / float32(info.Canvas.Width), float32(pivotSlice.Pivot.Y) / float32(info.Canvas.Height)}
			for _, channel := range []art.Channel{art.Albedo, art.Normal, art.Height, art.Emission} {
				artRegion, _ := pivotContractRegion(t, prepared, artMeta.ID, channelIDs[channel])
				if !artRegion.HasPivot || artRegion.Pivot != wantArt {
					t.Fatalf("%s Art region pivot = %+v, want %v", channel, artRegion, wantArt)
				}
			}
			for _, frame := range mapping[1:] {
				before, _ := pivotContractRegion(t, original, meta.ID, frame.Asset)
				after, _ := pivotContractRegion(t, linked, meta.ID, frame.Asset)
				if before.Pivot != after.Pivot || before.HasPivot != after.HasPivot {
					t.Fatalf("editing one pivot changed another: before=%+v after=%+v", before, after)
				}
			}
			if !bytes.Equal(ctx.Source, beforePNG) || !bytes.Equal(pipeline.WriteMeta(meta), linkedMeta) {
				t.Fatal("importing edited art changed the original PNG or linked metadata")
			}
		})
	}
}
