package material

import (
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/shader"
	"errors"
	"strings"
	"testing"
)

type typedDevice struct {
	*null.Device
	restores        int
	typed, ordinary int
	failTyped       bool
}

func (d *typedDevice) Restores() int { return d.restores }
func (d *typedDevice) CreateShader(desc gpu.ShaderDesc) (gpu.ShaderHandle, error) {
	for _, s := range desc.Sources {
		if strings.Contains(s.Source, "#define AEGO_TYPED_LIGHT_CALLS ") {
			d.typed++
			if d.failTyped {
				return gpu.ShaderHandle{}, errors.New("optional compile rejected")
			}
			return d.Device.CreateShader(desc)
		}
	}
	d.ordinary++
	return d.Device.CreateShader(desc)
}
func typedShader() *Shader {
	sh := testShader("lit", nil, []uint32{0})
	sh.Features = shader.FeatureLighting | shader.FeatureAffine
	sh.OpaqueWithOpaqueAlbedo = true
	sh.Variants[0].Sources[SourceIndex(true, true)] = "#version 300 es\n#ifdef AEGO_TYPED_LIGHT_CALLS\n#endif\n"
	return sh
}
func typedTestPass(d *typedDevice, c *Cache, run func(gpu.Pass)) {
	c.Begin()
	f := d.BeginFrame(32, 32)
	p := f.BeginPass(gpu.PassDesc{})
	run(p)
	p.End()
	d.EndFrame()
	c.End()
}
func TestTypedLightCallsPreserveIndicesTypesAndShadows(t *testing.T) {
	sh := typedShader()
	before := sh.Variants[0]
	sig := LightSignature{Count: 3, Lights: [8]uint8{4, 2, 5}}
	typed := specializeLightShader(sh, 0, sig)
	want := "aego_apply_light(0,0,true,normal,receiveShadows,result);aego_apply_light(1,2,false,normal,receiveShadows,result);aego_apply_light(2,1,true,normal,receiveShadows,result);"
	if typed == nil || typed.lightParent != sh || !strings.Contains(typed.Variants[0].Sources[3], want) || before != sh.Variants[0] {
		t.Fatal("typed program changed its parent or lost the ordered light operations")
	}
	if typed.Variants[0].Sources[0] != before.Sources[0] || typed.Variants[0].Sources[1] != before.Sources[1] || typed.Variants[0].Sources[2] != before.Sources[2] {
		t.Fatal("specialization changed an unrelated shader stage")
	}
	for _, bad := range []LightSignature{{Count: -1}, {Count: 9}, {Count: 1, Lights: [8]uint8{8}}, {Count: 0, Lights: [8]uint8{1}}} {
		if bad.valid() {
			t.Fatalf("invalid signature accepted: %+v", bad)
		}
	}
	old := testShader("legacy", nil, []uint32{0})
	if specializeLightShader(old, 0, sig) != nil {
		t.Fatal("legacy shader did not select its ordinary program")
	}
}
func TestTypedLightNoLateProgramsAndRetiredGPUResources(t *testing.T) {
	d := &typedDevice{Device: null.New()}
	defer d.Close()
	c := NewCache(d, true, spritePipe)
	defer c.Close()
	sh := typedShader()
	m, _ := New("lit", sh, 0, shader.BlendPremultiplied)
	sig := LightSignature{Count: 1, Lights: [8]uint8{4}}
	start := d.ResourceStats()
	typedTestPass(d, c, func(p gpu.Pass) {
		if !c.BindSpriteLightSet(p, m, false, false, true, sig) {
			t.Fatal("initial typed draw failed")
		}
		if c.BindSpriteLightSet(p, nil, false, false, true, sig) || c.BindSpriteLightSet(p, &Material{}, false, false, true, sig) {
			t.Fatal("empty material reported a draw")
		}
	})
	if d.typed != 1 || d.ordinary != 1 {
		t.Fatalf("cold typed=%d ordinary=%d", d.typed, d.ordinary)
	}
	warm := d.ResourceStats()
	typedTestPass(d, c, func(p gpu.Pass) {
		for _, s := range []LightSignature{sig, {Count: 2, Lights: [8]uint8{4, 1}}, {Count: -1}} {
			if !c.BindSpriteLightSet(p, m, false, false, true, s) {
				t.Fatal("warm fallback failed")
			}
		}
	})
	typedTestPass(d, c, func(p gpu.Pass) {
		if got := testing.AllocsPerRun(64, func() {
			if !c.BindSpriteLightSet(p, m, false, false, true, LightSignature{Count: 2, Lights: [8]uint8{4, 1}}) {
				panic("warm fallback failed")
			}
		}); got != 0 {
			t.Fatalf("warm fallback allocated %.1f objects", got)
		}
	})
	if d.typed != 1 || d.ordinary != 1 || d.ResourceStats() != warm {
		t.Fatal("new optional program or pipeline during warm camera change")
	}
	d.restores++
	typedTestPass(d, c, func(p gpu.Pass) {
		if !c.BindSpriteLightSet(p, m, false, false, true, sig) {
			t.Fatal("restore lost typed draw")
		}
	})
	next := typedShader()
	var committed bool
	c.QueueReload(sh, next, []*Material{m}, func() { committed = true })
	c.Begin()
	c.End()
	if !committed || m.Shader() != next || len(c.spec) != 0 {
		t.Fatal("replaced light program retained its shader or failed to commit")
	}
	typedTestPass(d, c, func(p gpu.Pass) {
		if !c.BindSpriteLightSet(p, m, false, false, true, sig) {
			t.Fatal("replacement could not specialize")
		}
	})
	if d.typed != 2 {
		t.Fatal("replacement reused a previous material generation")
	}
	c.Release(m)
	c.Retire(next)
	c.Begin()
	c.End()
	if len(c.lightFirstUse) != 0 || len(c.spec) != 0 || d.ResourceStats() != start {
		t.Fatalf("retirement retained resources %+v vs %+v", d.ResourceStats(), start)
	}
}
func TestTypedLightCompileFailureAndCapacityUseOrdinaryProgram(t *testing.T) {
	for _, reject := range []bool{false, true} {
		d := &typedDevice{Device: null.New(), failTyped: reject}
		c := NewCache(d, true, spritePipe)
		sh := typedShader()
		m, _ := New("lit", sh, 0, shader.BlendPremultiplied)
		typedTestPass(d, c, func(p gpu.Pass) {
			for i := 0; i < 40; i++ {
				sig := LightSignature{Count: 2, Lights: [8]uint8{uint8(i & 7), uint8(i / 8)}}
				if !c.BindSpriteLightSet(p, m, false, false, true, sig) {
					t.Fatal("capacity or rejected optional compile lost ordinary draw")
				}
			}
		})
		if len(c.spec) > 24 || d.typed != 24 || d.ordinary != 1 || c.Stats().CompileFailures != 0 {
			t.Fatalf("program cap or fallback failed: %v %+v", d.typed, c.Stats())
		}
		c.Close()
		d.Close()
	}
}
