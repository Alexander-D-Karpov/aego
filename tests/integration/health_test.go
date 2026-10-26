package integration

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"aego/editor/core/health"
)

func TestHealthFindsPreparedProblems(t *testing.T) {
	_, ctx := openEditor(t)
	src := filepath.Join(t.TempDir(), "orphan.png")
	if err := os.WriteFile(src, testPNG(t, 8, 8), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, batch, err := ctx.Pipeline.ImportExternal([]string{src}, "assets/unused"); err != nil {
		t.Fatal(err)
	} else {
		ctx.ApplyBatch(batch)
	}
	codes := map[string]health.Item{}
	for _, it := range health.Run(health.Env{Pipeline: ctx.Pipeline, Project: ctx.Project}) {
		codes[it.Code] = it
	}
	it, ok := codes["HLT004"]
	if !ok {
		t.Fatal("an imported texture nothing references must be reported as unused")
	}
	found := false
	for _, p := range it.Paths {
		found = found || p == "assets/unused/orphan.png"
	}
	if !found {
		t.Fatalf("unused paths = %v", it.Paths)
	}
	for _, it := range health.Run(health.Env{Pipeline: ctx.Pipeline, Project: ctx.Project, Exclude: []string{"assets/unused"}}) {
		if it.Code != "HLT004" {
			continue
		}
		for _, p := range it.Paths {
			if p == "assets/unused/orphan.png" {
				t.Fatal("an excluded folder is never unused")
			}
		}
	}
	big := health.Scan(ctx.Pipeline, nil, 4, nil)
	if len(big) == 0 || big[0].Code != "HLT007" {
		t.Fatal("an 8 px texture is larger than a 4 px limit")
	}
	off := false
	for _, it := range health.Scan(ctx.Pipeline, nil, 0, []color.NRGBA{{1, 2, 3, 255}}) {
		for _, p := range it.Paths {
			off = off || it.Code == "HLT009" && health.PathOf(p) == "assets/unused/orphan.png"
		}
	}
	if !off {
		t.Fatal("a texture drawn in other colors than the art palette is not reported")
	}
}
