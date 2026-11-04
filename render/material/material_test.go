package material

import (
	"slices"
	"testing"

	"aego/gpu"
	"aego/gpu/null"
	"aego/render/shader"
)

type pspec struct {
	name string
	kind shader.Kind
	def  [4]float32
	inst bool
}

func testShader(name string, keywords []string, masks []uint32, specs ...pspec) *Shader {
	var params []shader.Param
	var kinds []shader.Kind
	var idx []int
	tex, lane, comp, lanes := 0, 0, 0, 0
	for i, s := range specs {
		p := shader.Param{Name: s.name, Kind: s.kind, Default: s.def, Instance: s.inst}
		switch {
		case s.kind == shader.KindTexture:
			p.Slot = shader.TexUser + tex
			tex++
		case s.inst:
			n := s.kind.Lanes()
			if comp+n > 4 {
				lane, comp = lane+1, 0
			}
			p.Lane, p.Comp = lane, comp
			comp += n
			lanes = lane + 1
		default:
			kinds = append(kinds, s.kind)
			idx = append(idx, i)
		}
		params = append(params, p)
	}
	offs := make([]int, len(kinds))
	size := shader.Std140(kinds, offs)
	for j, i := range idx {
		params[i].Offset = offs[j]
	}
	sh := &Shader{Name: name, Keywords: keywords, Schema: NewSchema(params, size, lanes)}
	for _, m := range masks {
		sh.Variants = append(sh.Variants, Variant{Mask: m, Sources: [4]string{"v", "f", "v", "f"}})
	}
	return sh
}

func allKinds() *Shader {
	return testShader("all", nil, []uint32{0},
		pspec{"F", shader.KindFloat, [4]float32{0.5}, false},
		pspec{"I", shader.KindInt, [4]float32{3}, false},
		pspec{"B", shader.KindBool, [4]float32{1}, false},
		pspec{"V2", shader.KindVec2, [4]float32{1, 2}, false},
		pspec{"V3", shader.KindVec3, [4]float32{1, 2, 3}, false},
		pspec{"V4", shader.KindVec4, [4]float32{1, 2, 3, 4}, false},
		pspec{"T", shader.KindTexture, [4]float32{}, false},
		pspec{"L", shader.KindVec2, [4]float32{7, 8}, true},
	)
}

func TestSetGetEveryKind(t *testing.T) {
	m, ok := New("m", allKinds(), 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("no variant 0")
	}
	defaults := map[string][4]float32{"F": {0.5}, "I": {3}, "B": {1}, "V2": {1, 2}, "V3": {1, 2, 3}, "V4": {1, 2, 3, 4}}
	for n, want := range defaults {
		if got, ok := m.Get(n); !ok || got != want {
			t.Errorf("default %s = %v %v, want %v", n, got, ok, want)
		}
	}
	set := map[string][4]float32{"F": {-1.25}, "I": {-7}, "B": {0}, "V2": {3, 4}, "V3": {5, 6, 7}, "V4": {8, 9, 10, 11}}
	for n, v := range set {
		if !m.Set(n, v) {
			t.Fatalf("Set %s refused", n)
		}
	}
	for n, want := range set {
		if got, _ := m.Get(n); got != want {
			t.Errorf("%s = %v, want %v", n, got, want)
		}
	}
	if m.Set("I", [4]float32{2.6}) {
		t.Fatal("fractional int was accepted")
	}
	if got, _ := m.GetInt("I"); got != -7 {
		t.Fatal("invalid Set changed the old integer")
	}
	if m.Set("T", [4]float32{1}) || m.Set("Nope", [4]float32{}) {
		t.Fatal("Set took a texture or unknown name")
	}
	if !m.SetTexture("T", Tex{}) || m.SetTexture("F", Tex{}) {
		t.Fatal("SetTexture took the wrong parameter")
	}
	if l := m.DefaultLanes(); l[0] != 7 || l[1] != 8 || m.Lanes() != 1 {
		t.Fatalf("default lanes %v of %d", l[:4], m.Lanes())
	}
}

func TestPromoteKeepsValuesByName(t *testing.T) {
	old := testShader("v1", nil, []uint32{0},
		pspec{"Speed", shader.KindFloat, [4]float32{1}, false},
		pspec{"Tint", shader.KindVec4, [4]float32{1, 1, 1, 1}, false},
		pspec{"Kind", shader.KindFloat, [4]float32{}, false},
	)
	next := testShader("v2", nil, []uint32{0},
		pspec{"Glow", shader.KindVec3, [4]float32{0.5, 0.5, 0.5}, false},
		pspec{"Tint", shader.KindVec4, [4]float32{1, 1, 1, 1}, false},
		pspec{"Kind", shader.KindInt, [4]float32{2}, false},
		pspec{"Speed", shader.KindFloat, [4]float32{1}, false},
	)
	m, _ := New("m", old, 0, shader.BlendPremultiplied)
	m.Set("Speed", [4]float32{4})
	m.Set("Tint", [4]float32{0.1, 0.2, 0.3, 0.4})
	m.Set("Kind", [4]float32{9})
	if err := m.promote(next, 0); err == nil {
		t.Fatal("incompatible explicit type was silently reset")
	}
	if m.Shader() != old {
		t.Fatal("failed migration changed shader")
	}
	m.Reset("Kind")
	if err := m.promote(next, 0); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Get("Speed"); v[0] != 4 {
		t.Errorf("Speed lost: %v", v)
	}
	if v, _ := m.Get("Tint"); v != [4]float32{0.1, 0.2, 0.3, 0.4} {
		t.Errorf("Tint lost: %v", v)
	}
	if v, _ := m.Get("Glow"); v != [4]float32{0.5, 0.5, 0.5} {
		t.Errorf("new Glow is not its default: %v", v)
	}
	if v, _ := m.Get("Kind"); v[0] != 2 {
		t.Errorf("Kind changed type and kept the old value: %v", v)
	}
}

func TestValidate(t *testing.T) {
	sh := testShader("s", []string{"A", "B"}, []uint32{0, 1},
		pspec{"F", shader.KindFloat, [4]float32{}, false},
		pspec{"T", shader.KindTexture, [4]float32{}, false},
	)
	sh.Schema.Params[0].HasRange, sh.Schema.Params[0].Min, sh.Schema.Params[0].Max = true, 0, 1
	codes := func(ps []Problem) (out []string) {
		for _, p := range ps {
			out = append(out, p.Code)
		}
		return
	}
	cases := []struct {
		name string
		kw   []string
		vals []Value
		want string
		warn bool
	}{
		{"unknown param", nil, []Value{{Name: "X", N: 1}}, MAT001, true},
		{"texture as value", nil, []Value{{Name: "T", N: 1}}, MAT002, false},
		{"value as texture", nil, []Value{{Name: "F", Asset: true}}, MAT002, false},
		{"wrong count", nil, []Value{{Name: "F", N: 2}}, MAT002, false},
		{"out of range", nil, []Value{{Name: "F", N: 1, Lanes: [4]float32{2}}}, MAT002, true},
		{"unknown keyword", []string{"C"}, nil, MAT003, false},
		{"unlisted variant", []string{"B"}, nil, MAT003, false},
	}
	for _, c := range cases {
		ps := Validate(sh, c.kw, c.vals)
		if len(ps) != 1 || ps[0].Code != c.want || ps[0].Warn != c.warn {
			t.Errorf("%s: %v %+v", c.name, codes(ps), ps)
		}
	}
	if ps := Validate(sh, []string{"A"}, []Value{{Name: "F", N: 1, Lanes: [4]float32{0.5}}, {Name: "T", Asset: true}}); len(ps) != 0 {
		t.Errorf("a valid material: %+v", ps)
	}
	if _, ok := CheckDomain(sh, shader.DomainSprite); !ok {
		t.Error("sprite domain refused")
	}
	if p, ok := CheckDomain(sh, shader.DomainMesh2D); ok || p.Code != MAT004 {
		t.Errorf("mesh2d: %+v", p)
	}
}

func spritePipe(shader.Domain, int, shader.Blend) gpu.PipelineDesc {
	return gpu.PipelineDesc{Primitive: gpu.PrimTriangleStrip}
}

func TestBindDoesNotAllocateAfterTheFirstFrame(t *testing.T) {
	dev := null.New()
	c := NewCache(dev, false, spritePipe)
	white, _ := dev.CreateTexture(gpu.TextureDesc{Width: 1, Height: 1, Format: gpu.TexRGBA8})
	c.SetWhite(Tex{Texture: white})
	m, _ := New("m", allKinds(), 0, shader.BlendPremultiplied)
	frame := func() {
		c.Begin()
		f := dev.BeginFrame(8, 8)
		p := f.BeginPass(gpu.PassDesc{})
		m.Set("F", [4]float32{1})
		if !c.Bind(p, m) {
			t.Fatal("Bind failed")
		}
		p.End()
		dev.EndFrame()
		c.End()
	}
	frame()
	if n := testing.AllocsPerRun(50, frame); n != 0 {
		t.Fatalf("%v allocations per frame", n)
	}
}

func TestFailedReloadKeepsTheShader(t *testing.T) {
	dev := null.New()
	c := NewCache(dev, false, spritePipe)
	old := testShader("v1", []string{"A"}, []uint32{0, 1}, pspec{"F", shader.KindFloat, [4]float32{2}, false})
	next := testShader("v2", []string{"A"}, []uint32{0}, pspec{"F", shader.KindFloat, [4]float32{2}, false})
	m, _ := New("m", old, 1, shader.BlendPremultiplied)
	m.Set("F", [4]float32{5})
	m.Reload(next)
	f := dev.BeginFrame(8, 8)
	p := f.BeginPass(gpu.PassDesc{})
	if !c.Bind(p, m) {
		t.Fatal("the old shader stopped drawing")
	}
	p.End()
	dev.EndFrame()
	c.End()
	if m.Shader() != old || !m.Failed() {
		t.Fatalf("shader %s, failed %v", m.Shader().Name, m.Failed())
	}
	if v, _ := m.Get("F"); v[0] != 5 {
		t.Fatalf("value lost: %v", v)
	}
	fixed := testShader("v3", []string{"A"}, []uint32{0, 1}, pspec{"F", shader.KindFloat, [4]float32{2}, false})
	m.Reload(fixed)
	f = dev.BeginFrame(8, 8)
	c.Bind(f.BeginPass(gpu.PassDesc{}), m)
	dev.EndFrame()
	c.End()
	if m.Shader() != fixed || m.Failed() || !slices.Equal(m.Shader().Keywords, []string{"A"}) {
		t.Fatalf("the fixed shader was not taken: %s %v", m.Shader().Name, m.Failed())
	}
}

type lossyDevice struct {
	*null.Device
	restores int
	uploads  int
}

func (d *lossyDevice) Restores() int { return d.restores }

func (d *lossyDevice) UpdateBuffer(h gpu.BufferHandle, off int, data []byte) {
	d.uploads++
	d.Device.UpdateBuffer(h, off, data)
}

// A restored WebGL context brings uniform buffers back empty, so every material uploads its values again.
func TestContextRestoreReuploadsMaterials(t *testing.T) {
	dev := &lossyDevice{Device: null.New()}
	c := NewCache(dev, true, spritePipe)
	m, _ := New("m", allKinds(), 0, shader.BlendPremultiplied)
	frame := func() {
		c.Begin()
		f := dev.BeginFrame(8, 8)
		c.Bind(f.BeginPass(gpu.PassDesc{}), m)
		dev.EndFrame()
		c.End()
	}
	frame()
	frame()
	if dev.uploads != 1 {
		t.Fatalf("%d uploads of an unchanged material", dev.uploads)
	}
	dev.restores++
	frame()
	if dev.uploads != 2 {
		t.Fatalf("the restore did not upload the material again: %d uploads", dev.uploads)
	}
}
