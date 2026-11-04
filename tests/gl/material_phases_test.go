//go:build aego_gltest

package gl

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"aego/asset"
	shaderimport "aego/asset/importers/shader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/render/sprite"
)

// TestMaterialPhaseMeasurements separates development import/reference work
// from player decoding, driver linking, first draw and frame-end publication.
// Durations are observations, not machine-independent CI thresholds. The gate
// checks real publication, preserved explicit values and bounded GPU resources.
func TestMaterialPhaseMeasurements(t *testing.T) {
	r, dev, target := materialRig(t)
	const count = 5
	type sample struct {
		ImportReferenceNS int64 `json:"importReferenceNS"`
		DecodeNS          int64 `json:"decodeNS"`
		DriverLinkNS      int64 `json:"driverLinkNS"`
		QueueNS           int64 `json:"queueNS"`
		CommitLatencyNS   int64 `json:"commitLatencyNS"`
		FrameBoundaryNS   int64 `json:"frameBoundaryNS"`
	}
	var samples [count]sample
	var candidates [count]*material.Shader
	imp := shaderimport.New()
	for i := range candidates {
		source := fmt.Sprintf(`#pragma aego abi 1
#pragma material
uniform float Amount; // @default(0.5)
uniform float Scale; // @instance @default(1.0)
#pragma end
void fragment(){
    float value=Amount*Scale+float(%d)*0.000001;
    aego_out=vec4(value,0.0,0.0,value);
}
`, i)
		started := time.Now()
		result, err := imp.Import(&pipeline.ImportContext{Path: "assets/phase-measure.aeshader", Source: []byte(source)})
		samples[i].ImportReferenceNS = time.Since(started).Nanoseconds()
		if err != nil {
			t.Fatal(err)
		}
		started = time.Now()
		artifact, err := asset.DecodeShader(result.Artifacts[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		if artifact.Validation&shader.ReferenceChecked == 0 {
			t.Fatal("phase measurement requires the pinned glslang validator")
		}
		candidates[i] = artifact.Runtime(ids.AssetID{}, "phase-measure")
		samples[i].DecodeNS = time.Since(started).Nanoseconds()
		stages := candidates[i].Variants[0].Sources
		started = time.Now()
		h, err := dev.CreateShader(gpu.ShaderDesc{Label: "phase-link", Sources: []gpu.ShaderSource{
			{Stage: gpu.StageVertex, Source: stages[0]},
			{Stage: gpu.StageFragment, Source: stages[1]},
		}})
		samples[i].DriverLinkNS = time.Since(started).Nanoseconds()
		if err != nil {
			t.Fatal(err)
		}
		dev.DestroyShader(h)
	}
	m, ok := material.New("measured", candidates[0], 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("cannot create measured material")
	}
	m.Set("Amount", [4]float32{0.25})
	draw := func(v *render.ViewFrame) {
		v.Sprite(sprite.Draw{Region: r.White(), Position: mathx.Vec2{X: 16, Y: 16},
			Size: mathx.Vec2{X: 24, Y: 24}, Origin: mathx.Vec2{X: .5, Y: .5}, Tint: mathx.White, Material: m})
	}
	started := time.Now()
	drawMaterialFrame(t, r, dev, target, draw)
	prewarmNS := time.Since(started).Nanoseconds()
	reporter := dev.(gpu.ResourceReporter)
	baseline := reporter.ResourceStats()
	for i := range samples {
		// Import has finished. This next CPU definition has no dependency on
		// the importer or reference validator during driver publication.
		next := *candidates[(i+1)%count]
		r.Materials().Begin()
		var boundaryStart time.Time
		started = time.Now()
		r.Materials().QueueReload(m.Shader(), &next, []*material.Material{m}, func() {
			samples[i].CommitLatencyNS = time.Since(boundaryStart).Nanoseconds()
		})
		samples[i].QueueNS = time.Since(started).Nanoseconds()
		if m.Shader() == &next {
			t.Fatal("candidate published before the frame boundary")
		}
		boundaryStart = time.Now()
		r.Materials().End()
		samples[i].FrameBoundaryNS = time.Since(boundaryStart).Nanoseconds()
		if m.Shader() != &next || m.Failed() || samples[i].CommitLatencyNS == 0 {
			t.Fatal("GL33 candidate was not published at the frame boundary")
		}
		value, found := m.Get("Amount")
		if !found || value[0] != .25 {
			t.Fatalf("reload lost the explicit material value: %v", value)
		}
		drawMaterialFrame(t, r, dev, target, draw)
		if got := reporter.ResourceStats(); got != baseline {
			t.Fatalf("phase %d leaks resources: before=%+v after=%+v", i, baseline, got)
		}
	}
	data, err := json.MarshalIndent(struct {
		Method            string            `json:"method"`
		Renderer          string            `json:"renderer"`
		Samples           [count]sample     `json:"samples"`
		PrewarmReadbackNS int64             `json:"prewarmReadbackNS"`
		ResourcesBefore   gpu.ResourceStats `json:"resourcesBefore"`
		ResourcesAfter    gpu.ResourceStats `json:"resourcesAfter"`
		ReloadGenerations uint64            `json:"reloadGenerations"`
	}{"wall time; import includes both reference targets; driver cache is not flushed; prewarm includes draw/readback; frame boundary includes driver/layout/migration/commit", dev.Caps().Renderer, samples, prewarmNS, baseline, reporter.ResourceStats(), r.Materials().Stats().ReloadGenerations}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	if out := os.Getenv("AEGO_MATERIAL_PHASE_REPORT"); out != "" {
		if err := os.WriteFile(out, append(data, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
