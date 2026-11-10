package shell

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

	gameexport "aego/app/export"
	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/importers/atlas"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/artedit"
	edcontext "aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/panels/console"
	"aego/engine/problems"
	"aego/format/aetext"
	"aego/project"
)

type linkedSaveFixture struct {
	p                    *project.Project
	pl                   *pipeline.Pipeline
	ed                   *edcontext.Editor
	doc                  *document.AssetDocument
	model                *artedit.Document
	color, normal, artID ids.AssetID
	frames               []pipeline.SubAsset
	mapping              []artsource.TextureLinkFrame
	guarded              map[string][]byte
}

func linkedSavePNG(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func newLinkedSaveFixture(t *testing.T) *linkedSaveFixture {
	t.Helper()
	f := &linkedSaveFixture{}
	var err error
	f.p, err = project.Create(t.TempDir(), "Linked art save", false)
	if err != nil {
		t.Fatal(err)
	}
	colorPNG := linkedSavePNG(t, color.NRGBA{R: 32, G: 64, B: 96, A: 255})
	normalPNG := linkedSavePNG(t, color.NRGBA{R: 128, G: 128, B: 255, A: 255})
	colorMeta := pipeline.NewMeta("texture", artsource.NewTexture().Version(), asset.TextureDefaults())
	normalMeta := pipeline.NewMeta("texture", artsource.NewTexture().Version(), asset.TextureDefaults())
	normalMeta.Settings["semantic"] = aetext.Ident("normal")
	f.color, f.normal = colorMeta.ID, normalMeta.ID
	for n, name := range []string{"left", "right"} {
		colorMeta.Subs = append(colorMeta.Subs, pipeline.SubAsset{ID: ids.NewAssetID(), Name: name, Rect: [4]int{n * 2, 0, 2, 2}})
		normalMeta.Subs = append(normalMeta.Subs, pipeline.SubAsset{ID: ids.NewAssetID(), Name: name, Rect: [4]int{n * 2, 0, 2, 2}})
	}
	colorMeta.Settings["normal"] = aetext.AssetRef(normalMeta.ID.String())
	for n := range colorMeta.Subs {
		if err := asset.SetTextureChannel(colorMeta, colorMeta.Subs[n].ID, "normal", normalMeta.Subs[n].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	f.frames = append([]pipeline.SubAsset(nil), colorMeta.Subs...)
	for rel, data := range map[string][]byte{
		"assets/hero.png": colorPNG, "assets/hero.png.meta": pipeline.WriteMeta(colorMeta),
		"assets/hero_n.png": normalPNG, "assets/hero_n.png.meta": pipeline.WriteMeta(normalMeta),
	} {
		if err := os.WriteFile(filepath.Join(f.p.Root, rel), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	registry := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{atlas.Wrap(artsource.NewTexture()), artsource.New()} {
		if err := registry.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	f.pl, err = pipeline.Open(f.p, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.pl.Close)
	if _, _, err := f.pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := f.pl.ImportAll()
	checkLinkedSaveBatch(t, "initial", batch, err)
	meta, ok := f.pl.MetaOf(f.color)
	if !ok {
		t.Fatal("color source is not indexed")
	}
	drawing, frames, err := artsource.ImportTextureDocument(&pipeline.ImportContext{
		ID: f.color, Path: "assets/hero.png", Source: colorPNG, Meta: &meta,
		Read: f.pl.SourceBytes, Lookup: f.pl.Lookup, SourcePath: f.pl.PathOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	f.mapping = frames
	artMeta, err := f.pl.NewSourceMeta("assets/hero.aeart")
	if err != nil {
		t.Fatal(err)
	}
	f.artID = artMeta.ID
	if err := artsource.SetTextureLink(&meta, f.artID, colorPNG, frames); err != nil {
		t.Fatal(err)
	}
	f.guarded = make(map[string][]byte)
	for _, rel := range []string{"assets/hero.png", "assets/hero.png.meta", "assets/hero_n.png", "assets/hero_n.png.meta"} {
		f.guarded[rel], err = f.pl.SourceBytes(rel)
		if err != nil {
			t.Fatal(err)
		}
	}
	edits := []pipeline.SourceEdit{
		{Path: "assets/hero.aeart", After: encoded},
		{Path: "assets/hero.aeart.meta", After: pipeline.WriteMeta(artMeta)},
		{Path: "assets/hero.png.meta", Before: f.guarded["assets/hero.png.meta"], After: pipeline.WriteMeta(&meta)},
		{Path: "assets/hero.png", Before: colorPNG, After: colorPNG, ReadOnlyGuard: true},
		{Path: "assets/hero_n.png", Before: normalPNG, After: normalPNG, ReadOnlyGuard: true},
		{Path: "assets/hero_n.png.meta", Before: f.guarded["assets/hero_n.png.meta"], After: f.guarded["assets/hero_n.png.meta"], ReadOnlyGuard: true},
	}
	batch, err = f.pl.ApplySources(edits, false)
	checkLinkedSaveBatch(t, "link", batch, err)
	f.guarded["assets/hero.png.meta"] = edits[2].After
	f.ed = &edcontext.Editor{Pipeline: f.pl, Docs: document.NewManager(document.Options{Root: f.p.Root})}
	if err := f.ed.Docs.Handlers().Register(document.Handler{ID: "art.linked.test", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: document.View | document.EditSource | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	(&App{ctx: f.ed, consolePanel: console.New(nil)}).attachCode()
	f.doc, err = f.ed.OpenAsset(f.artID)
	if err != nil {
		t.Fatal(err)
	}
	f.model, err = artedit.Bind(f.ed.Docs, f.doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.model.Close)
	if err := f.pl.VerifyCurrent(f.color); err != nil {
		t.Fatalf("linked fixture is not initially current: %v", err)
	}
	return f
}

func checkLinkedSaveBatch(t *testing.T, stage string, batch *pipeline.Batch, err error) {
	t.Helper()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("%s batch: %+v, %v", stage, batch, err)
	}
	t.Logf("%s batch: imported=%d failed=%d retry=%d", stage, len(batch.Imported), len(batch.Failed), len(batch.Retry))
}

func (f *linkedSaveFixture) pixels(t *testing.T) []byte {
	t.Helper()
	e, ok := f.pl.Entry(f.color)
	if !ok {
		t.Fatal("linked color source disappeared")
	}
	a, ok := e.Artifact(asset.ArtTexture)
	if !ok {
		t.Fatal("linked color texture disappeared")
	}
	data, err := f.pl.Artifacts().ReadFile(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	tex, err := asset.DecodeTexture(data)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(tex.Pixels)
}

func (f *linkedSaveFixture) paintAndSave(t *testing.T) {
	t.Helper()
	_, err := f.model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 0, Y: 0}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ed.SaveAsset(f.doc); err != nil || f.doc.Dirty() {
		t.Fatalf("save drawing: dirty=%v err=%v", f.doc.Dirty(), err)
	}
}

func linkedSaveMaterialErrors(f *linkedSaveFixture) []problems.Diagnostic {
	var out []problems.Diagnostic
	for _, d := range gameexport.Check(f.p, gameexport.Take(f.pl), "release", false) {
		if d.Code == gameexport.CodeMaterial && d.Severity == problems.Error {
			out = append(out, d)
		}
	}
	return out
}

func TestLinkedArtSaveRefreshesSlicedPNGWithoutWatcher(t *testing.T) {
	f := newLinkedSaveFixture(t)
	before := f.pixels(t)
	if errs := linkedSaveMaterialErrors(f); len(errs) != 0 {
		t.Fatalf("initial normal associations blocked: %+v", errs)
	}
	f.paintAndSave(t)
	if err := f.pl.VerifyCurrent(f.color); err != nil {
		t.Fatalf("saved drawing left its linked PNG stale: %v", err)
	}
	after := f.pixels(t)
	if !bytes.Equal(after[:4], []byte{255, 0, 0, 255}) || !bytes.Equal(after[4:], before[4:]) {
		t.Fatalf("linked PNG pixels were not updated exactly: before=%v after=%v", before, after)
	}
	for i, frame := range f.frames {
		if err := f.pl.VerifyCurrent(frame.ID); err != nil {
			t.Fatalf("frame %s is not current: %v", frame.Name, err)
		}
		e, ok := f.pl.Entry(frame.ID)
		if !ok {
			t.Fatalf("frame %s lost its stable identity", frame.Name)
		}
		a, ok := e.Artifact(asset.ArtSpriteRegion)
		if !ok {
			t.Fatal("frame region disappeared")
		}
		b, err := f.pl.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := asset.DecodeRegion(b)
		wantNormal := artsource.StableID(f.artID, artsource.FrameName(f.mapping[i].Frame, art.Normal))
		if err != nil || r.Normal != wantNormal {
			t.Fatalf("frame lost its normal association: %+v, %v", r, err)
		}
	}
	for rel, want := range f.guarded {
		if got, err := f.pl.SourceBytes(rel); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Save changed original %s: %v", rel, err)
		}
	}
	if errs := linkedSaveMaterialErrors(f); len(errs) != 0 {
		t.Fatalf("current normal associations blocked: %+v", errs)
	}
}

func TestLinkedArtSaveDoesNotOverwriteExternalMetadata(t *testing.T) {
	f := newLinkedSaveFixture(t)
	meta, err := pipeline.ParseMeta(f.guarded["assets/hero.png.meta"], "assets/hero.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	meta.Settings["filter"] = aetext.Ident("linear")
	external := pipeline.WriteMeta(meta)
	if err := os.WriteFile(filepath.Join(f.p.Root, "assets/hero.png.meta"), external, 0644); err != nil {
		t.Fatal(err)
	}
	f.paintAndSave(t)
	if got, err := f.pl.SourceBytes("assets/hero.png.meta"); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("external metadata was overwritten: %v", err)
	}
	for rel, want := range f.guarded {
		if rel == "assets/hero.png.meta" {
			continue
		}
		if got, err := f.pl.SourceBytes(rel); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Save changed original %s: %v", rel, err)
		}
	}
	if err := f.pl.VerifyCurrent(f.color); err == nil {
		t.Fatal("external metadata change was reported as current")
	}
	if errs := linkedSaveMaterialErrors(f); len(errs) == 0 {
		t.Fatal("strict export accepted external metadata changes")
	}
}

func TestLinkedArtSaveDoesNotHideExternalNormalChange(t *testing.T) {
	f := newLinkedSaveFixture(t)
	external := linkedSavePNG(t, color.NRGBA{R: 160, G: 128, B: 255, A: 255})
	if err := os.WriteFile(filepath.Join(f.p.Root, "assets/hero_n.png"), external, 0644); err != nil {
		t.Fatal(err)
	}
	f.paintAndSave(t)
	if got, err := f.pl.SourceBytes("assets/hero_n.png"); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("external normal change was overwritten: %v", err)
	}
	for rel, want := range f.guarded {
		if rel == "assets/hero_n.png" {
			continue
		}
		if got, err := f.pl.SourceBytes(rel); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Save changed original %s: %v", rel, err)
		}
	}
	if err := f.pl.VerifyCurrent(f.normal); err == nil {
		t.Fatal("external normal source was reported as current")
	}
	if errs := linkedSaveMaterialErrors(f); len(errs) == 0 {
		t.Fatal("strict export accepted the stale normal source")
	}
}

func TestLinkedArtSaveRejectsExternalPNGChange(t *testing.T) {
	f := newLinkedSaveFixture(t)
	before := f.pixels(t)
	external := linkedSavePNG(t, color.NRGBA{R: 2, G: 3, B: 4, A: 255})
	if err := os.WriteFile(filepath.Join(f.p.Root, "assets/hero.png"), external, 0644); err != nil {
		t.Fatal(err)
	}
	f.paintAndSave(t)
	if notice, ok := f.ed.LastNotice(); !ok || notice.Level != edcontext.LevelError || !strings.Contains(notice.Text, "failed imports") {
		t.Fatalf("failed dependent import was not reported: %+v", notice)
	}
	if got, err := f.pl.SourceBytes("assets/hero.png"); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("external PNG change was overwritten: %v", err)
	}
	for rel, want := range f.guarded {
		if rel == "assets/hero.png" {
			continue
		}
		if got, err := f.pl.SourceBytes(rel); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("failed dependent import changed %s: %v", rel, err)
		}
	}
	if !bytes.Equal(before, f.pixels(t)) {
		t.Fatal("failed linked PNG import replaced the previous texture")
	}
	if err := f.pl.VerifyCurrent(f.color); err == nil {
		t.Fatal("changed external PNG was reported as current")
	}
	if errs := linkedSaveMaterialErrors(f); len(errs) == 0 {
		t.Fatal("strict export accepted the stale normal association")
	}
}
