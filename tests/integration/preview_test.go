package integration

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/preview"
	"aego/project"
)

func TestPreviewCachesTextureAsPNG(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Preview", false)
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(p.CacheDir(), "thumbnails")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	ctx := s.App.Context()
	src := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(src, testPNG(t, 300, 200), 0o644); err != nil {
		t.Fatal(err)
	}
	created, batch, err := ctx.Pipeline.ImportExternal([]string{src}, "assets")
	ctx.ApplyBatch(batch)
	if err != nil || len(created) != 1 {
		t.Fatalf("import: %v %v", created, err)
	}
	pv := s.App.Previews()
	// The null backend prepares CPU pixels and the disk cache but must not
	// pretend to provide a GPU texture. Wait for the actual async content.
	var img image.Image
	var key string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		img, key, err = pv.Content(created[0])
		if err == nil {
			break
		}
		if !errors.Is(err, preview.ErrPending) {
			t.Fatalf("prepare texture preview: %v", err)
		}
		s.Step()
		time.Sleep(time.Millisecond)
	}
	if err != nil || img == nil {
		t.Fatalf("texture preview never became ready: %v", err)
	}
	if _, ok := pv.ByID(created[0]); ok {
		t.Fatal("headless preview must not claim a GPU texture")
	}
	e, _ := ctx.Pipeline.Entry(created[0])
	wantKey, ok := pv.Key(e)
	if !ok || key != wantKey {
		t.Fatalf("preview key %q does not match imported texture key %q", key, wantKey)
	}
	f, err := os.Open(filepath.Join(ctx.Project.CacheDir(), preview.CacheDir, key+".png"))
	if err != nil {
		t.Fatalf("preview png is not cached: %v", err)
	}
	defer f.Close()
	cached, err := png.Decode(f)
	if err != nil || cached.Bounds() != img.Bounds() || img.Bounds().Dx() != preview.Size {
		t.Fatalf("invalid cached preview: bounds %v, error %v", img.Bounds(), err)
	}
	x, y := img.Bounds().Dx()/2, img.Bounds().Dy()/2
	r1, g1, b1, a1 := img.At(x, y).RGBA()
	r2, g2, b2, a2 := cached.At(x, y).RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
		t.Fatal("cached preview pixels do not match prepared content")
	}
	for fileExists(legacy) && time.Now().Before(deadline) {
		s.Step()
		time.Sleep(time.Millisecond)
	}
	if fileExists(legacy) {
		t.Fatal("the legacy thumbnail cache must be removed")
	}
}
