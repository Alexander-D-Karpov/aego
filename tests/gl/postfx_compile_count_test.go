//go:build aego_gltest

package gl

import (
	"strings"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/gl33"
	"aego/render"
	"aego/render/postfx"
	"aego/render/sprite"
)

// Embed the real GL33 device so optional interfaces and all rendering still
// reach the real driver; only count calls to its shader creation entry point.
type postFXCompileCounter struct {
	*gl33.Device
	calls map[string]int
}

func (d *postFXCompileCounter) CreateShader(desc gpu.ShaderDesc) (gpu.ShaderHandle, error) {
	d.calls[desc.Label]++
	return d.Device.CreateShader(desc)
}

func TestBloomIntensityDoesNotRecompileGL33(t *testing.T) {
	e := newGLEngine(t)
	defer e.Close()
	actual, ok := e.GPU().(*gl33.Device)
	if !ok {
		t.Fatalf("expected actual GL33 backend, got %T", e.GPU())
	}
	dev := &postFXCompileCounter{Device: actual, calls: make(map[string]int)}
	r, err := render.New(dev, e.Caps(), e.VFS(), log.Nop(), render.Options{MaxSprites: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	target, err := dev.CreateTexture(gpu.TextureDesc{Width: 32, Height: 32, Format: gpu.TexRGBA8, Label: "bloom-compile-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer dev.DestroyTexture(target)
	reg := postfx.NewRegistry()
	instance, err := reg.NewInstance(postfx.BloomID, "lamp")
	if err != nil {
		t.Fatal(err)
	}
	if len(instance.Parameters) < 2 || instance.Parameters[1].Name != "Intensity" {
		t.Fatal("Bloom schema lost its Intensity lane")
	}
	var plan postfx.Plan
	build := func() {
		if err := reg.Build(&plan, true, []postfx.Instance{instance}, postfx.Profile{}); err != nil {
			t.Fatal(err)
		}
	}
	build()
	shot := func() {
		drawMaterialFrame(t, r, dev, target, func(f *render.ViewFrame) {
			f.View().Clear = mathx.Color{A: 1}
			f.PostProcess(&plan)
			f.Sprite(sprite.Draw{Region: r.White(), Position: mathx.V2(16, 16), Size: mathx.V2(24, 24), Origin: mathx.V2(.5, .5), Tint: mathx.White})
		})
		if err := r.PostProcessError(); err != nil {
			t.Fatal(err)
		}
	}
	compileCount := func() int {
		n := 0
		for label, count := range dev.calls {
			if strings.HasPrefix(label, postfx.BloomID) {
				n += count
			}
		}
		return n
	}
	shot()
	warmed := compileCount()
	if warmed != 4 {
		t.Fatalf("warm Bloom: expected four actual GL shader creations, got %d: %v", warmed, dev.calls)
	}
	desc, _ := reg.Lookup(postfx.BloomID)
	for i := 0; i < 30; i++ {
		instance.Parameters[1].Value.X = .1 + float32(i%2)*.8
		if !plan.UpdateParameters(0, desc, &instance) {
			t.Fatal("active numeric update unexpectedly required a structural rebuild")
		}
		shot()
		if n := compileCount(); n != warmed {
			t.Fatalf("shader compiled during numeric edit %d: %d -> %d", i, warmed, n)
		}
	}
	instance.Parameters[1].Value.X = 0
	if plan.UpdateParameters(0, desc, &instance) {
		t.Fatal("crossing zero must rebuild the structural plan")
	}
	build()
	if len(plan.Passes) != 0 {
		t.Fatal("neutral Bloom still has a pass")
	}
	shot()
	instance.Parameters[1].Value.X = .22
	if plan.UpdateParameters(0, desc, &instance) {
		t.Fatal("leaving neutral must rebuild the structural plan")
	}
	build()
	shot()
	if n := compileCount(); n != warmed {
		t.Fatalf("restoring Bloom recompiled cached GL shaders: %d -> %d", warmed, n)
	}
	t.Logf("actual GL33 Bloom shader creations: %d at warmup, %d after 30 edits and zero crossings", warmed, compileCount())
}
