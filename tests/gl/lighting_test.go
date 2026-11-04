//go:build aego_gltest

package gl

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/tests/lightinggolden"
)

func TestLightingGPUContractImages(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	g, err := lightinggolden.New(e.GPU(), e.Caps(), e.VFS())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	report := lightinggolden.NewReport()
	lightinggolden.Run(g, report)
	lightinggolden.RunSurfaceResources(g, report)
	lightinggolden.RunSelection(g, report)
	lightinggolden.RunOutputs(g, report)
	lightinggolden.RunFallbacks(e.GPU(), e.VFS(), report)
	lightinggolden.RunLifetime(e.GPU(), e.VFS(), report)
	t.Logf("renderer=%s cases passed=%d failed=%d", e.Caps().Renderer, len(report.Passed), len(report.Failures))
	for _, failure := range report.Failures {
		t.Error(failure)
	}
	for _, unverified := range report.Unverified {
		t.Log("Unverified: " + unverified)
	}
	if dir := os.Getenv("AEGO_LIGHTING_REPORT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for name, pixels := range report.Images {
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, &image.RGBA{Pix: pixels, Stride: lightinggolden.Size * 4, Rect: image.Rect(0, 0, lightinggolden.Size, lightinggolden.Size)})
			closeErr := f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		data, err := json.MarshalIndent(struct {
			Renderer string                 `json:"renderer"`
			Report   *lightinggolden.Report `json:"report"`
		}{e.Caps().Renderer, report}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "results.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
