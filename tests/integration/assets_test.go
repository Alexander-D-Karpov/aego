package integration

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"aego/app/run"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

func TestAtlasGroupProducesOnePage(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Atlas", false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		name := filepath.Join(p.AssetsDir(), "sprite"+itoa(i)+".png")
		if err := os.WriteFile(name, testPNG(t, 16+i%8, 16), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	imps, err := run.DefaultImporters()
	if err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}

	metas, err := filepath.Glob(filepath.Join(p.AssetsDir(), "*.png"+pipeline.MetaSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 30 {
		t.Fatalf("scan produced %d meta files", len(metas))
	}
	for _, metaPath := range metas {
		data, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatal(err)
		}
		m, err := pipeline.ParseMeta(data, metaPath)
		if err != nil {
			t.Fatal(err)
		}
		m.Settings["atlas"] = aetext.String("characters")
		if err := os.WriteFile(metaPath, pipeline.WriteMeta(m), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pl2, err := pipeline.Open(p, imps, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl2.Close()
	pl2.Scan()
	batch, err := pl2.ImportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Failed) != 0 {
		t.Fatalf("failed = %v", batch.Failed)
	}
	pages := 0
	regions := 0
	pl2.Index().Each(func(e *asset.Entry) {
		switch e.Kind {
		case asset.ArtAtlasPage:
			pages++
		case asset.ArtSpriteRegion:
			regions++
		}
	})
	if pages != 1 {
		t.Fatalf("expected a single atlas page, got %d", pages)
	}
	if regions != 30 {
		t.Fatalf("expected 30 sprite regions, got %d", regions)
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: 0x40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
