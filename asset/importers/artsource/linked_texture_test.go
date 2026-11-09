package artsource

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"aego/art"
	"aego/asset"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/aetext"
)

func linkedTextureFixture(t *testing.T, sliced bool) (*pipeline.ImportContext, *art.Session, map[string][]byte, []TextureLinkFrame) {
	t.Helper()
	meta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
	if sliced {
		meta.Subs = []pipeline.SubAsset{
			{ID: ids.NewAssetID(), Name: "left", Rect: [4]int{0, 0, 2, 2}, Pivot: [2]float32{.25, .75}, HasPivot: true},
			{ID: ids.NewAssetID(), Name: "right", Rect: [4]int{2, 0, 2, 2}},
		}
	}
	im := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			im.SetNRGBA(x, y, color.NRGBA{R: uint8(10 + x*20), G: uint8(30 + y*25), B: 90, A: 200})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	ctx := &pipeline.ImportContext{ID: meta.ID, Path: "assets/hero.png", Source: b.Bytes(), Meta: meta}
	d, mapping, err := ImportTextureDocument(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	s, err := art.NewSession(d, art.NewJournal(art.DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	artMeta := pipeline.NewMeta("artsource", Version, New().Defaults())
	encoded, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"assets/hero.aeart": encoded, "assets/hero.aeart.meta": pipeline.WriteMeta(artMeta)}
	ctx.Read = func(name string) ([]byte, error) { return bytes.Clone(files[name]), nil }
	ctx.SourcePath = func(id ids.AssetID) (string, bool) {
		if id == artMeta.ID {
			return "assets/hero.aeart", true
		}
		return "", false
	}
	if err := SetTextureLink(meta, artMeta.ID, ctx.Source, mapping); err != nil {
		t.Fatal(err)
	}
	return ctx, s, files, mapping
}

func preparedTexturePixels(t *testing.T, result *pipeline.Result) []byte {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Kind == asset.ArtTexture {
			texture, err := asset.DecodeTexture(artifact.Data)
			if err != nil {
				t.Fatal(err)
			}
			return bytes.Clone(texture.Pixels)
		}
	}
	t.Fatal("missing runtime texture")
	return nil
}

func TestSourceArtNormalUsesExistingDownYPreparation(t *testing.T) {
	imageBytes := func(c color.NRGBA) []byte {
		im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				im.SetNRGBA(x, y, c)
			}
		}
		var out bytes.Buffer
		if err := png.Encode(&out, im); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	normal := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
	normal.Settings["role"] = aetext.Ident("normal")
	normal.Settings["normal_y"] = aetext.Ident("down")
	colorMeta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
	colorMeta.Settings["normal"] = aetext.AssetRef(normal.ID.String())
	normalPNG := imageBytes(color.NRGBA{R: 31, G: 69, B: 250, A: 128})
	files := map[string][]byte{"assets/source_n.png": normalPNG, "assets/source_n.png.meta": pipeline.WriteMeta(normal)}
	ctx := &pipeline.ImportContext{ID: colorMeta.ID, Path: "assets/source.png", Source: imageBytes(color.NRGBA{R: 255, A: 255}), Meta: colorMeta,
		Read: func(name string) ([]byte, error) { return files[name], nil }, SourcePath: func(id ids.AssetID) (string, bool) { return "assets/source_n.png", id == normal.ID }}
	doc, frames, err := ImportTextureDocument(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	prepared, err := texImporter.New().Import(&pipeline.ImportContext{ID: normal.ID, Path: "assets/source_n.png", Source: normalPNG, Meta: normal})
	if err != nil {
		t.Fatal(err)
	}
	pixels, err := doc.Render(frames[0].Frame, art.RenderOptions{Channel: art.Normal})
	if err != nil || !bytes.Equal(pixels.Pix, preparedTexturePixels(t, prepared)) {
		t.Fatal("linked source changed down-Y normal interpretation or alpha", err)
	}
	if pixels.NRGBAAt(0, 0) != (color.NRGBA{R: 31, G: 186, B: 250, A: 128}) {
		t.Fatal("normal was not converted to canonical +Y pixels")
	}
	delete(colorMeta.Settings, "normal")
	colorMeta.Settings["emission"] = aetext.AssetRef(normal.ID.String())
	normal.Settings["role"] = aetext.Ident("emission-mask")
	files["assets/source_n.png.meta"] = pipeline.WriteMeta(normal)
	if doc, _, err := ImportTextureDocument(ctx); err == nil {
		doc.Close()
		t.Fatal("source conversion silently changed emission-mask color semantics")
	}
}

func TestImportTextureDocumentPreservesStraightPalettePixels(t *testing.T) {
	palette := color.Palette{
		color.NRGBA{R: 71, G: 101, B: 151, A: 0},
		color.NRGBA{R: 201, G: 19, B: 89, A: 1},
		color.NRGBA{R: 143, G: 57, B: 211, A: 128},
		color.NRGBA{R: 13, G: 203, B: 29, A: 255},
	}
	im := image.NewPaletted(image.Rect(0, 0, 4, 1), palette)
	for x := 0; x < 4; x++ {
		im.SetColorIndex(x, 0, uint8(x))
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	meta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
	meta.Subs = []pipeline.SubAsset{
		{ID: ids.NewAssetID(), Name: "first", Rect: [4]int{0, 0, 2, 1}},
		{ID: ids.NewAssetID(), Name: "second", Rect: [4]int{2, 0, 2, 1}},
	}
	beforeSource, beforeMeta := bytes.Clone(encoded.Bytes()), pipeline.WriteMeta(meta)
	doc, frames, err := ImportTextureDocument(&pipeline.ImportContext{ID: meta.ID, Path: "assets/indexed.png", Source: encoded.Bytes(), Meta: meta})
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if !bytes.Equal(beforeSource, encoded.Bytes()) || !bytes.Equal(beforeMeta, pipeline.WriteMeta(meta)) || len(frames) != 2 {
		t.Fatal("opening the PNG mutated its source, metadata or frame mapping")
	}
	check := func(d *art.Document) {
		t.Helper()
		info := d.Info()
		for i, frame := range frames {
			var cel art.ID
			for _, candidate := range info.Cels {
				if candidate.Frame == frame.Frame && candidate.Channel == art.Albedo {
					cel = candidate.ID
					break
				}
			}
			for x := 0; x < 2; x++ {
				want := palette[i*2+x].(color.NRGBA)
				got, err := d.Pixel(cel, x, 0)
				if err != nil || got != (art.Color{R: want.R, G: want.G, B: want.B, A: want.A}) {
					t.Fatalf("frame %d pixel %d: got %+v (%v), want %+v", i, x, got, err, want)
				}
			}
		}
	}
	check(doc)
	saved, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := art.Decode(saved, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	check(reopened)
}

func TestImportTextureDocumentRejectsSixteenBitWithoutMutation(t *testing.T) {
	im := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	im.SetNRGBA64(0, 0, color.NRGBA64{R: 0x1234, G: 0xabcd, B: 0x5678, A: 0xcdef})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	meta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
	beforeSource, beforeMeta := bytes.Clone(encoded.Bytes()), pipeline.WriteMeta(meta)
	doc, _, err := ImportTextureDocument(&pipeline.ImportContext{ID: meta.ID, Path: "assets/high-depth.png", Source: encoded.Bytes(), Meta: meta})
	if doc != nil {
		doc.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "16-bit") {
		t.Fatalf("source precision loss was not rejected: %v", err)
	}
	if !bytes.Equal(beforeSource, encoded.Bytes()) || !bytes.Equal(beforeMeta, pipeline.WriteMeta(meta)) {
		t.Fatal("rejected PNG import changed source or metadata")
	}
}

func TestImportTextureDocumentRejectsSixteenBitSurfaceMaps(t *testing.T) {
	main := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	main.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	var mainPNG, surfacePNG bytes.Buffer
	if err := png.Encode(&mainPNG, main); err != nil {
		t.Fatal(err)
	}
	surface := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	surface.SetNRGBA64(0, 0, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
	if err := png.Encode(&surfacePNG, surface); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"normal", "height", "emission"} {
		t.Run(channel, func(t *testing.T) {
			meta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
			surfaceMeta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
			meta.Settings[channel] = aetext.AssetRef(surfaceMeta.ID.String())
			beforeMeta := pipeline.WriteMeta(meta)
			files := map[string][]byte{
				"assets/surface.png":      bytes.Clone(surfacePNG.Bytes()),
				"assets/surface.png.meta": pipeline.WriteMeta(surfaceMeta),
			}
			ctx := &pipeline.ImportContext{ID: meta.ID, Path: "assets/color.png", Source: bytes.Clone(mainPNG.Bytes()), Meta: meta,
				Read:       func(name string) ([]byte, error) { return bytes.Clone(files[name]), nil },
				SourcePath: func(id ids.AssetID) (string, bool) { return "assets/surface.png", id == surfaceMeta.ID },
			}
			doc, _, err := ImportTextureDocument(ctx)
			if doc != nil {
				doc.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "16-bit") {
				t.Fatalf("%s precision loss was not rejected: %v", channel, err)
			}
			if !bytes.Equal(ctx.Source, mainPNG.Bytes()) || !bytes.Equal(files["assets/surface.png"], surfacePNG.Bytes()) || !bytes.Equal(beforeMeta, pipeline.WriteMeta(meta)) {
				t.Fatal("rejected surface import changed source or metadata")
			}
		})
	}
}

func TestLinkedPNGKeepsRuntimeIDsAndChangesOnlySelectedFrame(t *testing.T) {
	for _, sliced := range []bool{false, true} {
		t.Run(map[bool]string{false: "whole sprite", true: "sheet"}[sliced], func(t *testing.T) {
			ctx, session, files, mapping := linkedTextureFixture(t, sliced)
			original, err := texImporter.New().Import(ctx)
			if err != nil {
				t.Fatal(err)
			}
			linked, err := NewTexture().Import(ctx)
			if err != nil {
				t.Fatal(err)
			}
			before := preparedTexturePixels(t, original)
			if !bytes.Equal(before, preparedTexturePixels(t, linked)) {
				t.Fatal("initial linking changed prepared pixels")
			}
			if len(original.Subs) != len(linked.Subs) {
				t.Fatal("linking changed sliced frame count")
			}
			for i, sub := range original.Subs {
				if sub.ID != linked.Subs[i].ID || sub.Name != linked.Subs[i].Name || !bytes.Equal(sub.Artifacts[0].Data, linked.Subs[i].Artifacts[0].Data) {
					t.Fatal("linking changed an existing frame identity, pivot or region")
				}
			}
			frame := mapping[len(mapping)-1].Frame
			if err := session.Select("", frame, art.Albedo); err != nil {
				t.Fatal(err)
			}
			beforeSalt, err := NewTexture().CacheSalt(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, B: 255, A: 255}}}})
			if err != nil {
				t.Fatal(err)
			}
			files["assets/hero.aeart"], err = session.Document.Encode()
			if err != nil {
				t.Fatal(err)
			}
			after, err := NewTexture().Import(ctx)
			if err != nil {
				t.Fatal(err)
			}
			pixels := preparedTexturePixels(t, after)
			paintedOffset := (1*4 + 1) * 4
			if sliced {
				paintedOffset += 2 * 4
			}
			for i := range pixels {
				if i < paintedOffset || i >= paintedOffset+4 {
					if pixels[i] != before[i] {
						t.Fatalf("unselected source pixel changed at byte %d", i)
					}
				}
			}
			if !bytes.Equal(pixels[paintedOffset:paintedOffset+4], []byte{255, 0, 255, 255}) {
				t.Fatal("painting did not reach the existing runtime texture")
			}
			afterSalt, err := NewTexture().CacheSalt(ctx)
			if err != nil || bytes.Equal(beforeSalt, afterSalt) {
				t.Fatalf("authoring source changes did not invalidate import cache: %v", err)
			}
			if err := session.Undo(); err != nil {
				t.Fatal(err)
			}
			files["assets/hero.aeart"], _ = session.Document.Encode()
			undone, err := NewTexture().Import(ctx)
			if err != nil || !bytes.Equal(preparedTexturePixels(t, undone), before) {
				t.Fatalf("undo did not restore runtime source pixels: %v", err)
			}
		})
	}
}

func TestLinkedPNGRejectsExternalSourceAndBrokenMappings(t *testing.T) {
	for _, scenario := range []string{"PNG changed", "missing art", "metadata identity", "frame removed", "slicing changed", "slicing rectangles exchanged", "duplicate mapping", "duplicate authoring frame", "frame mapping exchanged"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, session, files, mapping := linkedTextureFixture(t, true)
			switch scenario {
			case "PNG changed":
				ctx.Source = append(bytes.Clone(ctx.Source), 1)
			case "missing art":
				delete(files, "assets/hero.aeart")
			case "metadata identity":
				files["assets/hero.aeart.meta"] = pipeline.WriteMeta(pipeline.NewMeta("artsource", Version, New().Defaults()))
			case "frame removed":
				if _, err := session.Execute(context.Background(), art.Operation{Kind: "delete-frame", Frame: mapping[1].Frame}); err != nil {
					t.Fatal(err)
				}
				files["assets/hero.aeart"], _ = session.Document.Encode()
			case "slicing changed":
				ctx.Meta.Subs = ctx.Meta.Subs[:1]
			case "slicing rectangles exchanged":
				// The IDs and rectangle sizes remain valid, but accepting this
				// would silently paint each authored frame into the other region.
				ctx.Meta.Subs[0].Rect, ctx.Meta.Subs[1].Rect = ctx.Meta.Subs[1].Rect, ctx.Meta.Subs[0].Rect
			case "duplicate mapping":
				values := ctx.Meta.Settings["art_frames"]
				values.List[1] = values.List[0]
				ctx.Meta.Settings["art_frames"] = values
			case "duplicate authoring frame":
				values := ctx.Meta.Settings["art_frames"]
				values.List[1] = aetext.Struct(
					aetext.Field{Name: "asset", Value: aetext.AssetRef(mapping[1].Asset.String())},
					aetext.Field{Name: "frame", Value: aetext.String(string(mapping[0].Frame))},
				)
				ctx.Meta.Settings["art_frames"] = values
			case "frame mapping exchanged":
				values := ctx.Meta.Settings["art_frames"]
				values.List[0] = aetext.Struct(
					aetext.Field{Name: "asset", Value: aetext.AssetRef(mapping[0].Asset.String())},
					aetext.Field{Name: "frame", Value: aetext.String(string(mapping[1].Frame))},
				)
				values.List[1] = aetext.Struct(
					aetext.Field{Name: "asset", Value: aetext.AssetRef(mapping[1].Asset.String())},
					aetext.Field{Name: "frame", Value: aetext.String(string(mapping[0].Frame))},
				)
				ctx.Meta.Settings["art_frames"] = values
			}
			if _, err := NewTexture().Import(ctx); err == nil {
				t.Fatal("unsafe linked source was accepted")
			}
		})
	}
}

func TestLinkedPNGGeometryGuardAllowsReorderAndRecognizesLegacyLinks(t *testing.T) {
	ctx, _, _, _ := linkedTextureFixture(t, true)
	link, err := TextureLinkOf(ctx.Meta)
	if err != nil || link == nil || link.LayoutSHA256 == "" {
		t.Fatalf("new link has no geometry guard: %+v, %v", link, err)
	}
	// Reordering, renaming and pivot edits do not change a region's rectangle
	// or identity. They must not invalidate the authoring-frame mapping.
	ctx.Meta.Subs[0], ctx.Meta.Subs[1] = ctx.Meta.Subs[1], ctx.Meta.Subs[0]
	ctx.Meta.Subs[0].Name = "renamed"
	ctx.Meta.Subs[0].Pivot, ctx.Meta.Subs[0].HasPivot = [2]float32{0.1, 0.8}, true
	values := ctx.Meta.Settings["art_frames"]
	values.List[0], values.List[1] = values.List[1], values.List[0]
	ctx.Meta.Settings["art_frames"] = values
	if link, err := TextureLinkOf(ctx.Meta); err != nil || link.LayoutSHA256 == "" {
		t.Fatalf("non-geometric metadata edit invalidated the link: %+v, %v", link, err)
	}
	// Existing projects may contain pre-guard metadata. Reading it is retained
	// for compatibility, but an absent fingerprint is not presented as guarded.
	delete(ctx.Meta.Settings, "art_source_layout_sha256")
	legacy, err := TextureLinkOf(ctx.Meta)
	if err != nil || legacy == nil || legacy.LayoutSHA256 != "" {
		t.Fatalf("legacy link could not be read as unguarded: %+v, %v", legacy, err)
	}
	result, err := NewTexture().Import(ctx)
	if err != nil || result == nil || result.Error != nil {
		t.Fatalf("legacy linked source no longer imports: %+v, %v", result, err)
	}
}

func TestSetTextureLinkRejectsDuplicateFrameWithoutChangingMetadata(t *testing.T) {
	meta := pipeline.NewMeta("texture", texImporter.Version, asset.TextureDefaults())
	before := pipeline.WriteMeta(meta)
	frames := []TextureLinkFrame{{Asset: ids.NewAssetID(), Frame: "0000000000000001"},
		{Asset: ids.NewAssetID(), Frame: "0000000000000001"}}
	if err := SetTextureLink(meta, ids.NewAssetID(), []byte("source"), frames); err == nil {
		t.Fatal("two distinct source assets were mapped to the same frame")
	}
	if after := pipeline.WriteMeta(meta); !bytes.Equal(before, after) {
		t.Fatal("rejected link changed the original metadata")
	}
}

func TestLinkedPNGEmissionUsesRealPreparedArtFrames(t *testing.T) {
	ctx, session, files, mapping := linkedTextureFixture(t, true)
	if err := session.Select("", mapping[1].Frame, art.Emission); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, G: 40, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	files["assets/hero.aeart"], _ = session.Document.Encode()
	linked, err := NewTexture().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	link, _ := TextureLinkOf(ctx.Meta)
	artMeta, err := pipeline.ParseMeta(files["assets/hero.aeart.meta"], "assets/hero.aeart.meta")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := New().Import(&pipeline.ImportContext{ID: link.Source, Path: "assets/hero.aeart", Source: files["assets/hero.aeart"], Meta: artMeta})
	if err != nil {
		t.Fatal(err)
	}
	region, err := asset.DecodeRegion(linked.Subs[1].Artifacts[0].Data)
	if err != nil || region.Emission.IsZero() {
		t.Fatalf("original sprite region has no emission binding: %v", err)
	}
	for _, sub := range prepared.Subs {
		if sub.ID == region.Emission && sub.Name == FrameName(mapping[1].Frame, art.Emission) {
			return
		}
	}
	t.Fatal("emission binding does not resolve to a prepared channel frame")
}

func TestTextureLinkMetadataValidation(t *testing.T) {
	ctx, _, _, _ := linkedTextureFixture(t, false)
	for _, bad := range []aetext.Value{aetext.String("arbitrary path"), aetext.AssetRef(ctx.ID.String()), aetext.AssetRef("bad")} {
		ctx.Meta.Settings["art_source"] = bad
		if _, err := TextureLinkOf(ctx.Meta); err == nil || !strings.Contains(err.Error(), "artsource") {
			t.Fatalf("invalid authoring reference accepted: %v", err)
		}
	}
}

func TestLinkedPNGExplicitPivotRetainsLegacyIdentityAndMetadata(t *testing.T) {
	ctx, session, files, mapping := linkedTextureFixture(t, true)
	before := pipeline.WriteMeta(ctx.Meta)
	var pivot art.Slice
	for _, slice := range session.Document.Info().Slices {
		if slice.Frame == mapping[0].Frame && slice.Name == "pivot" {
			pivot = slice
		}
	}
	pivot.Pivot = image.Pt(0, 2)
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "slice", Slice: &pivot}); err != nil {
		t.Fatal(err)
	}
	var err error
	files["assets/hero.aeart"], err = session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewTexture().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sub := range result.Subs {
		if sub.ID != mapping[0].Asset {
			continue
		}
		for _, artifact := range sub.Artifacts {
			if artifact.Kind != asset.ArtSpriteRegion {
				continue
			}
			region, err := asset.DecodeRegion(artifact.Data)
			if err != nil {
				t.Fatal(err)
			}
			if !region.HasPivot || region.Pivot != ([2]float32{0, 1}) {
				t.Fatal("authoring pivot did not reach prepared legacy region", region.Pivot)
			}
			found = true
		}
	}
	if !found || !bytes.Equal(before, pipeline.WriteMeta(ctx.Meta)) {
		t.Fatal("pivot edit replaced legacy ID or changed original metadata")
	}
	if err := session.Undo(); err != nil {
		t.Fatal(err)
	}
	files["assets/hero.aeart"], err = session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	result, err = NewTexture().Import(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range result.Subs {
		if sub.ID == mapping[0].Asset {
			region, err := asset.DecodeRegion(sub.Artifacts[0].Data)
			if err != nil || region.Pivot != ([2]float32{.25, .75}) {
				t.Fatal("pivot Undo did not preserve the exact original fractional pivot", err)
			}
		}
	}
}
