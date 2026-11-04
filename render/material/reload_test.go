package material

import (
	"errors"
	"math"
	"testing"

	"aego/gpu"
	"aego/gpu/null"
	"aego/render/shader"
)

func TestExactIntegerAndBoolInstanceABI(t *testing.T) {
	sh := testShader("typed", nil, []uint32{0}, pspec{"I", shader.KindInt, [4]float32{}, true}, pspec{"B", shader.KindBool, [4]float32{}, true})
	m, _ := New("m", sh, 0, shader.BlendPremultiplied)
	for _, n := range []int32{math.MinInt32, -16777217, 16777217, math.MaxInt32} {
		if !m.SetInt("I", n) {
			t.Fatal("set int")
		}
		got, ok := m.GetInt("I")
		if !ok || got != n || math.Float32bits(m.DefaultLanes()[0]) != uint32(n) {
			t.Fatalf("integer bits lost: %d -> %d", n, got)
		}
	}
	if !m.Set("B", [4]float32{1}) || math.Float32bits(m.DefaultLanes()[1]) != 1 {
		t.Fatal("bool lane is not typed uint word")
	}
	if m.Set("B", [4]float32{2}) || m.Set("I", [4]float32{1.25}) || m.Set("I", [4]float32{math.MaxFloat32}) {
		t.Fatal("invalid typed value accepted")
	}
	lanes := *m.DefaultLanes()
	p, _ := sh.Schema.Param("I")
	if !PutLaneInt(&lanes, p, -16777217) || math.Float32bits(lanes[0]) != uint32(0xfeffffff) {
		t.Fatalf("override integer narrowed: %08x", math.Float32bits(lanes[0]))
	}
	if got, _ := m.GetInt("I"); got != math.MaxInt32 {
		t.Fatal("node override mutated shared material")
	}
}

func TestExplicitInheritanceUnknownAndAliasMigration(t *testing.T) {
	old := testShader("old", nil, []uint32{0}, pspec{"Inherited", shader.KindFloat, [4]float32{1}, false}, pspec{"Explicit", shader.KindFloat, [4]float32{1}, true}, pspec{"Removed", shader.KindFloat, [4]float32{}, false})
	m, _ := New("m", old, 0, shader.BlendPremultiplied)
	m.Set("Explicit", [4]float32{1})
	m.Set("Removed", [4]float32{7})
	next := testShader("next", nil, []uint32{0}, pspec{"Inherited", shader.KindFloat, [4]float32{3}, false}, pspec{"Renamed", shader.KindFloat, [4]float32{4}, true})
	next.Schema.Params[1].Former = []string{"Explicit"}
	if err := m.promote(next, 0); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Get("Inherited"); v[0] != 3 {
		t.Fatal("inherited default did not update")
	}
	if v, _ := m.Get("Renamed"); v[0] != 1 || !m.Explicit("Renamed") {
		t.Fatal("explicit equal-default/alias lost")
	}
	back := testShader("back", nil, []uint32{0}, pspec{"Removed", shader.KindFloat, [4]float32{}, false})
	if err := m.promote(back, 0); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Get("Removed"); v[0] != 7 {
		t.Fatal("unknown removed value lost")
	}
}

func TestReloadVariantsMatchKeywordsNotBitPositions(t *testing.T) {
	old := testShader("old", []string{"A", "B"}, []uint32{0, 1, 2}, pspec{"F", shader.KindFloat, [4]float32{}, false})
	next := testShader("next", []string{"B", "A"}, []uint32{0, 1, 2}, pspec{"F", shader.KindFloat, [4]float32{}, false})
	m, _ := New("m", old, 1, shader.BlendPremultiplied)
	if err := m.promote(next, 0); err != nil || m.Mask() != 2 {
		t.Fatalf("wrong variant: %d %v", m.Mask(), err)
	}
}

func TestGroupedReloadIsAtomicAndRetainsOffscreenLastGood(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	cache := NewCache(dev, false, spritePipe)
	defer cache.Close()
	old := testShader("old", nil, []uint32{0}, pspec{"Value", shader.KindFloat, [4]float32{1}, false})
	next := testShader("next", nil, []uint32{0}, pspec{"Value", shader.KindInt, [4]float32{2}, false})
	a, _ := New("a", old, 0, shader.BlendPremultiplied)
	b, _ := New("b", old, 0, shader.BlendPremultiplied)
	b.Set("Value", [4]float32{7})
	cache.Track(a)
	cache.Track(b)
	committed := false
	cache.QueueReload(old, next, []*Material{a, b}, func() { committed = true })
	cache.Begin()
	cache.End()
	if committed || a.Shader() != old || b.Shader() != old || !a.Failed() || !b.Failed() {
		t.Fatal("partial group commit")
	}
	// A material stays last-good even across frames in which it was not drawn.
	cache.Retire(old)
	for range 3 {
		cache.Begin()
		cache.End()
	}
	if len(cache.retired) != 1 {
		t.Fatal("offscreen reference retired prematurely")
	}
	b.Reset("Value")
	cache.QueueReload(old, next, []*Material{a, b}, func() { committed = true })
	cache.Begin()
	cache.End()
	if !committed || a.Shader() != next || b.Shader() != next {
		t.Fatal("fixed group did not commit")
	}
	cache.Begin()
	cache.End()
	if len(cache.retired) != 0 {
		t.Fatal("old generation was not released")
	}
}

type deferredJob struct {
	d                *deferredDevice
	desc             gpu.ShaderDesc
	ready, cancelled bool
}

func (j *deferredJob) Poll() (gpu.ShaderHandle, bool, error) {
	if j.cancelled {
		return gpu.ShaderHandle{}, true, errors.New("cancelled")
	}
	if !j.ready {
		return gpu.ShaderHandle{}, false, nil
	}
	h, e := j.d.Device.CreateShader(j.desc)
	return h, true, e
}
func (j *deferredJob) Cancel() { j.cancelled = true }

type deferredDevice struct {
	*null.Device
	jobs       []*deferredJob
	generation uint64
	lost       bool
}

func (d *deferredDevice) BeginShaderCompile(desc gpu.ShaderDesc) (gpu.ShaderCompilation, error) {
	j := &deferredJob{d: d, desc: desc}
	d.jobs = append(d.jobs, j)
	return j, nil
}
func (d *deferredDevice) ContextGeneration() uint64 { return d.generation }
func (d *deferredDevice) ContextLost() bool         { return d.lost }

func TestStaleCompilationAndContextLossCannotCommit(t *testing.T) {
	dev := &deferredDevice{Device: null.New()}
	defer dev.Close()
	cache := NewCache(dev, true, spritePipe)
	defer cache.Close()
	old := testShader("old", nil, []uint32{0})
	a := testShader("slow", nil, []uint32{0})
	b := testShader("new", nil, []uint32{0})
	m, _ := New("m", old, 0, shader.BlendPremultiplied)
	cache.Track(m)
	cache.QueueReload(old, a, []*Material{m}, nil)
	cache.Begin()
	cache.End()
	slow := dev.jobs[0]
	cache.QueueReload(old, b, []*Material{m}, nil)
	if !slow.cancelled {
		t.Fatal("superseded compile not cancelled")
	}
	slow.ready = true
	cache.Begin()
	cache.End()
	latest := dev.jobs[len(dev.jobs)-1]
	dev.lost = true
	dev.generation++
	cache.Begin()
	cache.End()
	latest.ready = true
	if !latest.cancelled || m.Shader() != old {
		t.Fatal("old context candidate committed")
	}
	dev.lost = false
	dev.generation++
	cache.Begin()
	cache.End()
	dev.jobs[len(dev.jobs)-1].ready = true
	cache.Begin()
	cache.End()
	if m.Shader() != b {
		t.Fatal("latest source did not compile in restored context")
	}
}

func TestMaterialDefinitionReloadPreservesIdentity(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	c := NewCache(dev, false, spritePipe)
	defer c.Close()
	sh := testShader("s", nil, []uint32{0}, pspec{"F", shader.KindFloat, [4]float32{1}, false})
	m, _ := New("m", sh, 0, shader.BlendPremultiplied)
	candidate, _ := New("new", sh, 0, shader.BlendPremultiplied)
	candidate.Set("F", [4]float32{8})
	c.Track(m)
	c.QueueMaterial(m, candidate, nil)
	if v, _ := m.Get("F"); v[0] != 1 {
		t.Fatal("definition changed before frame boundary")
	}
	c.Begin()
	c.End()
	if v, _ := m.Get("F"); v[0] != 8 {
		t.Fatal("definition was not committed")
	}
}

func TestIndependentMaterialSwitchesAndCancellation(t *testing.T) {
	dev := &deferredDevice{Device: null.New()}
	defer dev.Close()
	c := NewCache(dev, true, spritePipe)
	defer c.Close()
	old := testShader("shared", nil, []uint32{0})
	a, _ := New("a", old, 0, shader.BlendPremultiplied)
	b, _ := New("b", old, 0, shader.BlendPremultiplied)
	first := testShader("first", nil, []uint32{0})
	second := testShader("second", nil, []uint32{0})
	ca, _ := New("a-new", first, 0, shader.BlendPremultiplied)
	cb, _ := New("b-new", second, 0, shader.BlendPremultiplied)
	ac, bc := false, false
	c.QueueMaterial(a, ca, func() { ac = true })
	c.QueueMaterial(b, cb, func() { bc = true })
	c.Begin()
	c.End()
	if c.Stats().Pending != 2 || len(dev.jobs) != 2 {
		t.Fatal("independent definitions overwrote a shared-shader request")
	}
	c.CancelMaterial(a)
	for _, job := range dev.jobs {
		job.ready = true
	}
	c.Begin()
	c.End()
	if ac || !bc || a.Shader() != old || b.Shader() != second {
		t.Fatal("cancelling one definition affected another")
	}
	c.QueueMaterial(a, ca, func() { ac = true })
	c.Begin()
	c.End()
	job := dev.jobs[len(dev.jobs)-1]
	c.Release(a)
	job.ready = true
	c.Begin()
	c.End()
	if !job.cancelled || ac || a.Shader() != old {
		t.Fatal("released preview accepted pending shader state")
	}
}
