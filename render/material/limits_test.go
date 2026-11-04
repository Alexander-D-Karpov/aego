package material

import (
	"strings"
	"testing"

	"aego/gpu"
	"aego/gpu/null"
	"aego/render/shader"
)

type limitedDevice struct {
	*null.Device
	caps gpu.Capabilities
}

func (d *limitedDevice) Caps() gpu.Capabilities { return d.caps }

func TestDeviceResourceLimitRejectsWholeCandidateAndPreservesLastGood(t *testing.T) {
	for _, name := range []string{"attributes", "instance-lanes", "uniform-block"} {
		t.Run(name, func(t *testing.T) {
			base := null.New()
			dev := &limitedDevice{Device: base, caps: base.Caps()}
			defer dev.Close()
			old := testShader("last-good", nil, []uint32{0})
			next := testShader("over-budget", nil, []uint32{0})
			template := spritePipe
			switch name {
			case "attributes":
				dev.caps.MaxVertexAttributes = 7
				old.Domain, next.Domain = shader.DomainMesh2D, shader.DomainMesh2D
				next.Schema.Lanes = 4
				template = func(_ shader.Domain, lanes int, _ shader.Blend) gpu.PipelineDesc {
					layout := gpu.VertexLayout{Strides: [gpu.MaxVertexSlots]uint16{64, 64}}
					for i := 0; i < 4; i++ {
						layout.Attrs = append(layout.Attrs, gpu.VertexAttr{Location: uint8(i), Format: gpu.VFFloat4, Offset: uint16(i * 16)})
					}
					for i := 0; i < lanes; i++ {
						layout.Attrs = append(layout.Attrs, gpu.VertexAttr{Location: uint8(4 + i), Slot: 1, Format: gpu.VFUInt4, Offset: uint16(i * 16)})
					}
					return gpu.PipelineDesc{Layout: layout, Primitive: gpu.PrimTriangles}
				}
			case "instance-lanes":
				next.Schema.Lanes = shader.MaxLanes + 1
			case "uniform-block":
				dev.caps.MaxUniformBlockSize = 16
				next.Schema.Size = 32
			}
			c := NewCache(dev, false, template)
			defer c.Close()
			var diagnostic error
			c.OnError = func(_ *Shader, _ int, err error) { diagnostic = err }
			m, _ := New("m", old, 0, shader.BlendPremultiplied)
			c.Begin()
			f := dev.BeginFrame(8, 8)
			p := f.BeginPass(gpu.PassDesc{})
			if !c.Bind(p, m) {
				t.Fatal("last-good did not bind")
			}
			p.End()
			dev.EndFrame()
			c.End()
			before := dev.ResourceStats()
			committed := false
			c.QueueReload(old, next, []*Material{m}, func() { committed = true })
			c.Begin()
			c.End()
			if diagnostic == nil || !strings.Contains(diagnostic.Error(), "SHD004") {
				t.Fatalf("missing resource diagnostic: %v", diagnostic)
			}
			if committed || m.Shader() != old || !m.Failed() || !c.Ready(m) {
				t.Fatal("resource overflow partially committed or lost old draw state")
			}
			if after := dev.ResourceStats(); after != before {
				t.Fatalf("rejected candidate leaked GPU resources: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestRepeatedMaterialUnloadReleasesGPUResources(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	c := NewCache(dev, false, spritePipe)
	defer c.Close()
	baseline := dev.ResourceStats()
	for i := 0; i < 32; i++ {
		sh := testShader("temporary-preview", nil, []uint32{0}, pspec{"Value", shader.KindFloat, [4]float32{.5}, false})
		m, _ := New("preview", sh, 0, shader.BlendPremultiplied)
		c.Begin()
		f := dev.BeginFrame(8, 8)
		p := f.BeginPass(gpu.PassDesc{})
		if !c.Bind(p, m) {
			t.Fatal("preview did not bind")
		}
		p.DrawInstanced(4, 1)
		p.End()
		dev.EndFrame()
		c.End()
		c.Release(m)
		c.Retire(sh)
		c.Begin()
		c.End()
		if got := dev.ResourceStats(); got != baseline || c.Stats().Materials != 0 {
			t.Fatalf("unload cycle %d retained resources: before=%+v after=%+v cache=%+v", i, baseline, got, c.Stats())
		}
	}
}
