package integration

import (
	"os"
	"path/filepath"
	"testing"

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
	ok := false
	for i := 0; i < 60 && !ok; i++ {
		_, ok = pv.ByID(created[0])
		s.Step()
	}
	if !ok {
		t.Fatal("texture preview never became ready")
	}
	e, _ := ctx.Pipeline.Entry(created[0])
	key, _ := pv.Key(e)
	if _, err := os.Stat(filepath.Join(ctx.Project.CacheDir(), preview.CacheDir, key+".png")); err != nil {
		t.Fatalf("preview png is not cached: %v", err)
	}
	for i := 0; i < 10 && fileExists(legacy); i++ {
		s.Step()
	}
	if fileExists(legacy) {
		t.Fatal("the legacy thumbnail cache must be removed")
	}
}
