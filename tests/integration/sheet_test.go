package integration

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/slice"
	"aego/format/aetext"
)

// writeSheet adds a w×h PNG to assets/ and imports it.
func writeSheet(t *testing.T, s *edit.Session, name string, w, h int) ids.AssetID {
	t.Helper()
	pl := s.Host.Pipeline
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.NRGBA{R: uint8(x * 2), G: uint8(y * 4), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(pl.Abs("assets"), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	id, ok := pl.Lookup("assets/" + name)
	if !ok {
		t.Fatal("the sheet was not scanned")
	}
	b, err := pl.Import(id)
	if err != nil {
		t.Fatal(err)
	}
	s.App.Context().ApplyBatch(b)
	return id
}

// Import settings are one History step like slicing: Apply rewrites the meta and reimports, Undo puts the old meta back.
func TestImportSettingsUndoRestoresTheMeta(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	pl := s.Host.Pipeline
	sheet := writeSheet(t, s, "tiles.png", 64, 16)
	metaPath := pl.Abs("assets/tiles.png") + pipeline.MetaSuffix
	before, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := pl.MetaOf(sheet)
	after := m.Settings.Clone()
	after["filter"] = aetext.Ident("linear")
	after["mips"] = aetext.Bool(true)
	if err := ctx.Doc().History.Exec(&slice.Edit{Pipeline: pl, Sheet: sheet, BeforeSettings: m.Settings.Clone(), AfterSettings: after, Apply: ctx.ApplyBatch}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(metaPath)
	if !strings.Contains(string(got), "linear") {
		t.Fatalf("Apply did not write the meta:\n%s", got)
	}
	cells := slice.Plan("tiles", nil, slice.Grid(64, 16, [2]int{16, 16}, [2]int{}, [2]int{}, 0))
	if err := ctx.Doc().History.Exec(&slice.Edit{Pipeline: pl, Sheet: sheet, After: cells, Apply: ctx.ApplyBatch}); err != nil {
		t.Fatal(err)
	}
	if m, _ := pl.MetaOf(sheet); m.Settings.String("filter", "") != "linear" || len(m.Subs) != 4 {
		t.Fatalf("slicing lost the import settings: %v, %d cells", m.Settings, len(m.Subs))
	}
	for range 2 {
		if err := ctx.Doc().History.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := os.ReadFile(metaPath); !bytes.Equal(got, before) {
		t.Fatalf("Undo did not restore the meta:\n%s\nwant\n%s", got, before)
	}
}
