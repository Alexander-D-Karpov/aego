package texture_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/asset"
	"aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

func sheet(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func open(t *testing.T) (*project.Project, *pipeline.Pipeline) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Subs", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "hero.png"), sheet(t, 32, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(texture.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.ImportAll(); err != nil {
		t.Fatal(err)
	}
	return p, pl
}

func cells(n int) []pipeline.SubAsset {
	out := make([]pipeline.SubAsset, n)
	for i := range out {
		out[i] = pipeline.SubAsset{Name: "hero_00" + string(rune('0'+i)), ID: ids.NewAssetID(), Rect: [4]int{i * 16, 0, 16, 16}}
	}
	return out
}

func TestSlicedSheetImportsCells(t *testing.T) {
	_, pl := open(t)
	sheetID, _ := pl.Lookup("assets/hero.png")
	subs := cells(2)
	if err := pl.SetSubs(sheetID, subs); err != nil {
		t.Fatal(err)
	}
	b, err := pl.Reimport(sheetID)
	if err != nil || len(b.Failed) > 0 {
		t.Fatalf("reimport %v %v", err, b.Failed)
	}
	for i, sa := range subs {
		id, ok := pl.Lookup("assets/hero.png#" + sa.Name)
		if !ok || id != sa.ID {
			t.Fatalf("lookup of cell %d: %v %v", i, id, ok)
		}
		if p, ok := pl.PathOf(sa.ID); !ok || p != "assets/hero.png#"+sa.Name {
			t.Fatalf("path of cell %d: %q", i, p)
		}
		e, ok := pl.Entry(sa.ID)
		if !ok || e.Kind != asset.ArtSpriteRegion || e.Name != "assets/hero/"+sa.Name {
			t.Fatalf("entry %+v", e)
		}
	}
	meta, _ := os.ReadFile(filepath.Join(pl.Abs("assets/hero.png") + ".meta"))
	if !strings.Contains(string(meta), `sub "hero_001"`) {
		t.Fatalf("meta:\n%s", meta)
	}
	if err := pl.SetSubs(sheetID, subs[:1]); err != nil {
		t.Fatal(err)
	}
	b, _ = pl.Reimport(sheetID)
	if len(b.Removed) != 1 || b.Removed[0] != subs[1].ID {
		t.Fatalf("removed %v", b.Removed)
	}
	if _, ok := pl.Grave(subs[1].ID); !ok {
		t.Fatal("a cell that is gone must be buried")
	}
}

func TestDeletingTheSheetBuriesItsCells(t *testing.T) {
	_, pl := open(t)
	sheetID, _ := pl.Lookup("assets/hero.png")
	subs := cells(2)
	if err := pl.SetSubs(sheetID, subs); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.Reimport(sheetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.Delete(sheetID); err != nil {
		t.Fatal(err)
	}
	for _, sa := range subs {
		if g, ok := pl.Grave(sa.ID); !ok || g.Path != "assets/hero.png#"+sa.Name {
			t.Fatalf("cell %s not buried: %+v", sa.Name, g)
		}
		if _, ok := pl.Lookup("assets/hero.png#" + sa.Name); ok {
			t.Fatal("a buried cell must not resolve")
		}
	}
}

func TestSlicedSheetCannotJoinAnAtlas(t *testing.T) {
	_, pl := open(t)
	sheetID, _ := pl.Lookup("assets/hero.png")
	m, _ := pl.MetaOf(sheetID)
	m.Settings["atlas"] = aetext.String("chars")
	if err := pl.SetSettings(sheetID, m.Settings); err != nil {
		t.Fatal(err)
	}
	if err := pl.SetSubs(sheetID, cells(1)); err != nil {
		t.Fatal(err)
	}
	b, _ := pl.Reimport(sheetID)
	if err := b.Failed[sheetID]; err == nil || !strings.Contains(err.Error(), "atlas group") {
		t.Fatalf("failed %v", b.Failed)
	}
}

func TestSubsSurviveReopen(t *testing.T) {
	p, pl := open(t)
	sheetID, _ := pl.Lookup("assets/hero.png")
	subs := cells(2)
	if err := pl.SetSubs(sheetID, subs); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.Reimport(sheetID); err != nil {
		t.Fatal(err)
	}
	pl.Close()
	reg := pipeline.NewImporterRegistry()
	_ = reg.Register(texture.New())
	again, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, _, err := again.Scan(); err != nil {
		t.Fatal(err)
	}
	if id, ok := again.Lookup("assets/hero.png#hero_001"); !ok || id != subs[1].ID {
		t.Fatal("cells must resolve after reopening from the meta")
	}
}
