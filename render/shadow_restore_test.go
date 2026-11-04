package render

import (
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/null"
	"errors"
	"slices"
	"testing"
)

type shadowGenerationDevice struct {
	*null.Device
	generation uint64
	validated  []int
	reject     int
}

func (d *shadowGenerationDevice) ContextLost() bool         { return false }
func (d *shadowGenerationDevice) ContextGeneration() uint64 { return d.generation }
func (d *shadowGenerationDevice) ValidateRenderTarget(p gpu.PassDesc) error {
	d.validated = append(d.validated, p.TargetSubresource.Layer)
	if p.TargetSubresource.Layer == d.reject {
		return errors.New("injected restored layer validation failure")
	}
	return d.Device.ValidateRenderTarget(p)
}

func TestShadowArrayRewarmsEveryLayerOncePerContextGeneration(t *testing.T) {
	d := &shadowGenerationDevice{Device: null.New(), generation: 1, reject: -1}
	defer d.Close()
	r := &Renderer{dev: d, log: log.Nop().With(log.Channel("shadow-restore-test"))}
	var v lightingView
	r.beginLightingFrame()
	if !r.prepareShadowArray(&v, 64, 3) || !slices.Equal(d.validated, []int{0, 1, 2}) {
		t.Fatalf("initial prewarm: %v", d.validated)
	}
	original := v.shadows
	before := d.ResourceStats()
	d.validated = d.validated[:0]
	if !r.prepareShadowArray(&v, 64, 3) || len(d.validated) != 0 {
		t.Fatal("steady frame recreated shadow attachments")
	}
	// The GPU recreates the underlying texture but preserves its public handle.
	// A failure on an idle layer must not publish a partially ready generation.
	d.generation++
	d.reject = 1
	r.beginLightingFrame()
	if r.prepareShadowArray(&v, 64, 3) || !slices.Equal(d.validated, []int{0, 1}) || v.shadows != original || v.shadowFBOGeneration == d.generation {
		t.Fatalf("partial restore was published: %+v %v", v, d.validated)
	}
	d.reject = -1
	d.validated = d.validated[:0]
	if !r.prepareShadowArray(&v, 64, 3) || !slices.Equal(d.validated, []int{0, 1, 2}) || v.shadows != original || d.ResourceStats() != before {
		t.Fatalf("restore did not warm identical capacity without reallocating: %v", d.validated)
	}
	d.validated = d.validated[:0]
	if !r.prepareShadowArray(&v, 64, 3) || len(d.validated) != 0 {
		t.Fatal("restored steady frame recreated shadow attachments")
	}
	r.dropLightingView(&v)
}
