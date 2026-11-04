package material

import (
	"errors"
	"testing"

	"aego/gpu"
	"aego/gpu/null"
	"aego/render/shader"
)

type rejectAuxDevice struct {
	*null.Device
	reject bool
}

func (d *rejectAuxDevice) CreatePipeline(desc gpu.PipelineDesc) (gpu.PipelineHandle, error) {
	if d.reject {
		for _, attr := range desc.Layout.Attrs {
			if attr.Location == 9 {
				return gpu.PipelineHandle{}, errors.New("injected affine layout failure")
			}
		}
	}
	return d.Device.CreatePipeline(desc)
}

func TestReloadPreparesUsedAffineLayoutBeforeCommit(t *testing.T) {
	dev := &rejectAuxDevice{Device: null.New()}
	t.Cleanup(dev.Close)
	c := NewCache(dev, false, spritePipe)
	t.Cleanup(c.Close)
	old := testShader("old-affine", nil, []uint32{0})
	old.Features = shader.FeatureAffine
	next := testShader("next-affine", nil, []uint32{0})
	next.Features = shader.FeatureAffine
	m, _ := New("shared", old, 0, shader.BlendPremultiplied)
	c.Track(m)
	if _, ok := c.pipelineLayout(old, 0, m.blend, false, true); !ok {
		t.Fatal("old layout")
	}
	dev.reject = true
	committed := false
	c.QueueReload(old, next, []*Material{m}, func() { committed = true })
	c.Begin()
	c.End()
	if committed || m.Shader() != old || !m.Failed() {
		t.Fatal("layout failure did not preserve last-good")
	}
	dev.reject = false
	c.QueueReload(old, next, []*Material{m}, func() { committed = true })
	c.Begin()
	c.End()
	if !committed || m.Shader() != next {
		t.Fatal("fixed candidate did not commit")
	}
	if _, ok := c.pipes[pipeKey{sh: next, variant: 0, blend: m.blend, aux: true}]; !ok {
		t.Fatal("active affine layout was not prepared")
	}
	if _, ok := c.pipes[pipeKey{sh: next, variant: 0, blend: m.blend}]; ok {
		t.Fatal("reload allocated an unused base pipeline")
	}
}
