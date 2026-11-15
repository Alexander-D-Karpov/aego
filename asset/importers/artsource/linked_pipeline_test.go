package artsource

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/atlas"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
)

func TestLinkedSourceRealPipelinePreservesPNGIDsAndUndo(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Linked PNG", false)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x*30), G: uint8(30 + y*50), B: 40, A: 255})
		}
	}
	var pngBuffer bytes.Buffer
	if err := png.Encode(&pngBuffer, img); err != nil {
		t.Fatal(err)
	}
	const pngPath, artPath = "assets/hero.png", "assets/hero.aeart"
	pngBytes := bytes.Clone(pngBuffer.Bytes())
	meta := pipeline.NewMeta("texture", NewTexture().Version(), asset.TextureDefaults())
	meta.Subs = []pipeline.SubAsset{
		{ID: ids.NewAssetID(), Name: "left", Rect: [4]int{0, 0, 2, 2}},
		{ID: ids.NewAssetID(), Name: "right", Rect: [4]int{2, 0, 2, 2}},
	}
	metaBytes := pipeline.WriteMeta(meta)
	for rel, data := range map[string][]byte{pngPath: pngBytes, pngPath + pipeline.MetaSuffix: metaBytes} {
		if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(rel)), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	registry := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{atlas.Wrap(NewTexture()), New()} {
		if err := registry.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	checkBatch := func(batch *pipeline.Batch, err error) {
		t.Helper()
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("pipeline batch failed or needs retry: %+v, %v", batch, err)
		}
	}
	checkBatch(pl.ImportAll())
	root, ok := pl.Lookup(pngPath)
	if !ok || root != meta.ID {
		t.Fatal("PNG source identity changed")
	}
	for _, frame := range meta.Subs {
		id, ok := pl.Lookup(pngPath + pipeline.SubSep + frame.Name)
		if !ok || id != frame.ID {
			t.Fatalf("original frame identity missing: %s", frame.Name)
		}
	}
	imageMeta, ok := pl.MetaOf(root)
	if !ok {
		t.Fatal("PNG metadata missing")
	}
	doc, mapping, err := ImportTextureDocument(&pipeline.ImportContext{
		ID: root, Path: pngPath, Source: pngBytes, Meta: &imageMeta,
		Read: pl.SourceBytes, Lookup: pl.Lookup, SourcePath: pl.PathOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	encoded, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	artMeta, err := pl.NewSourceMeta(artPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetTextureLink(&imageMeta, artMeta.ID, pngBytes, mapping); err != nil {
		t.Fatal(err)
	}
	create := []pipeline.SourceEdit{
		{Path: artPath, After: encoded},
		{Path: artPath + pipeline.MetaSuffix, After: pipeline.WriteMeta(artMeta)},
		{Path: pngPath + pipeline.MetaSuffix, Before: metaBytes, After: pipeline.WriteMeta(&imageMeta)},
		{Path: pngPath, Before: pngBytes, After: pngBytes, ReadOnlyGuard: true},
	}
	checkBatch(pl.ApplySources(create, false))
	texture := func() []byte {
		t.Helper()
		entry, ok := pl.Entry(root)
		if !ok {
			t.Fatal("PNG entry disappeared")
		}
		for _, artifact := range entry.Artifacts {
			if artifact.Kind != asset.ArtTexture {
				continue
			}
			data, err := pl.Artifacts().ReadFile(artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := asset.DecodeTexture(data)
			if err != nil {
				t.Fatal(err)
			}
			return bytes.Clone(decoded.Pixels)
		}
		t.Fatal("PNG texture artifact missing")
		return nil
	}
	before := texture()
	linkedMetaBytes, err := pl.SourceBytes(pngPath + pipeline.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	changedMeta, err := pipeline.ParseMeta(linkedMetaBytes, pngPath+pipeline.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	changedMeta.Subs[0].Rect, changedMeta.Subs[1].Rect = changedMeta.Subs[1].Rect, changedMeta.Subs[0].Rect
	invalidEdit := []pipeline.SourceEdit{{Path: pngPath + pipeline.MetaSuffix, Before: linkedMetaBytes, After: pipeline.WriteMeta(changedMeta)}}
	if batch, err := pl.ApplySources(invalidEdit, false); err == nil || !strings.Contains(err.Error(), "slicing changed") {
		t.Fatalf("changing linked slicing silently remapped source frames: batch=%+v, error=%v", batch, err)
	}
	if got, err := pl.SourceBytes(pngPath + pipeline.MetaSuffix); err != nil || !bytes.Equal(got, linkedMetaBytes) {
		t.Fatalf("rejected slicing edit did not restore its metadata: %v", err)
	}
	if got, err := pl.SourceBytes(pngPath); err != nil || !bytes.Equal(got, pngBytes) {
		t.Fatalf("rejected slicing edit changed the original PNG: %v", err)
	}
	if got := texture(); !bytes.Equal(got, before) {
		t.Fatal("rejected slicing edit changed the prepared texture")
	}
	session, err := art.NewSession(doc, art.NewJournal(art.DefaultLimits()))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Select("", mapping[1].Frame, art.Albedo); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, B: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	painted, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	paint := []pipeline.SourceEdit{{Path: artPath, Before: encoded, After: painted}}
	checkBatch(pl.ApplySources(paint, false))
	if got := texture(); bytes.Equal(got, before) {
		t.Fatal("reimporting the art did not update the existing PNG texture")
	}
	if err := pl.VerifyCurrent(root); err != nil {
		t.Fatalf("linked PNG became stale after art import: %v", err)
	}
	checkBatch(pl.ApplySources(paint, true))
	if got := texture(); !bytes.Equal(got, before) {
		t.Fatal("Undo did not restore the PNG runtime texture")
	}
	checkBatch(pl.ApplySources(create, true))
	if got, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(pngPath))); err != nil || !bytes.Equal(got, pngBytes) {
		t.Fatalf("linking or Undo changed the original PNG: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(pngPath+pipeline.MetaSuffix))); err != nil || !bytes.Equal(got, metaBytes) {
		t.Fatalf("Undo did not restore the original PNG metadata: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(p.Root, filepath.FromSlash(artPath))); !os.IsNotExist(err) {
		t.Fatalf("Undo left the created source behind: %v", err)
	}
	for _, frame := range meta.Subs {
		id, ok := pl.Lookup(pngPath + pipeline.SubSep + frame.Name)
		if !ok || id != frame.ID {
			t.Fatalf("frame identity changed after Undo: %s", frame.Name)
		}
	}
}
