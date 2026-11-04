package material

import (
	"errors"
	"testing"

	"aego/gpu/null"
	"aego/render/shader"
)

func TestMaterialSwitchWaitsForTargetShaderTransaction(t *testing.T) {
	dev := &deferredDevice{Device: null.New()}
	defer dev.Close()
	c := NewCache(dev, true, spritePipe)
	defer c.Close()
	a0 := testShader("A", nil, []uint32{0})
	b0 := testShader("B0", nil, []uint32{0})
	b1 := testShader("B1", nil, []uint32{0})
	a, _ := New("a", a0, 0, shader.BlendPremultiplied)
	b, _ := New("b", b0, 0, shader.BlendPremultiplied)
	next, _ := New("a-to-b", b1, 0, shader.BlendPremultiplied)
	c.Track(b)
	group, switched := false, false
	c.QueueReload(b0, b1, nil, func() { group = true })
	c.QueueMaterial(a, next, func() {
		if !group {
			t.Error("material committed before its target shader group")
		}
		switched = true
	})
	c.Begin()
	c.End()
	if group || switched || a.Shader() != a0 || b.Shader() != b0 || len(dev.jobs) != 1 {
		t.Fatal("pending target was independently compiled or committed")
	}
	dev.jobs[0].ready = true
	c.Begin()
	c.End()
	if !group || !switched || a.Shader() != b1 || b.Shader() != b1 || c.Stats().Pending != 0 {
		t.Fatal("ready target did not commit both definitions in order")
	}
}

func TestRejectedTargetShaderCannotEscapeThroughMaterialSwitch(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	c := NewCache(dev, false, spritePipe)
	defer c.Close()
	a0 := testShader("A", nil, []uint32{0})
	b0 := testShader("B0", nil, []uint32{0})
	b1 := testShader("B1", nil, []uint32{0})
	a, _ := New("a", a0, 0, shader.BlendPremultiplied)
	b, _ := New("b", b0, 0, shader.BlendPremultiplied)
	next, _ := New("a-to-b", b1, 0, shader.BlendPremultiplied)
	c.Track(b)
	c.SetReloadCheck("active-node", func(_, _ *Shader) error { return errors.New("incompatible explicit node override") })
	group, switched := false, false
	c.QueueReload(b0, b1, nil, func() { group = true })
	c.QueueMaterial(a, next, func() { switched = true })
	c.Begin()
	c.End()
	if group || switched || a.Shader() != a0 || b.Shader() != b0 || !a.Failed() || !b.Failed() || c.Pending(a) {
		t.Fatal("failed target transaction became visible through a material switch")
	}
	if c.Stats().Programs != 0 {
		t.Fatal("rejected target retained its newly compiled program")
	}
}

func TestCanceledTargetRejectsStaleSwitchAndNewRevisionCanCommit(t *testing.T) {
	dev := &deferredDevice{Device: null.New()}
	defer dev.Close()
	c := NewCache(dev, true, spritePipe)
	defer c.Close()
	a0 := testShader("A", nil, []uint32{0})
	b0 := testShader("B0", nil, []uint32{0})
	b1 := testShader("B1", nil, []uint32{0})
	b2 := testShader("B2", nil, []uint32{0})
	a, _ := New("a", a0, 0, shader.BlendPremultiplied)
	b, _ := New("b", b0, 0, shader.BlendPremultiplied)
	first, _ := New("old-source", b1, 0, shader.BlendPremultiplied)
	second, _ := New("new-source", b2, 0, shader.BlendPremultiplied)
	c.Track(b)
	stale, latest := false, false
	c.QueueReload(b0, b1, nil, nil)
	c.QueueMaterial(a, first, func() { stale = true })
	c.Begin()
	c.End()
	slow := dev.jobs[0]
	c.QueueReload(b0, b2, nil, nil)
	slow.ready = true
	c.Begin()
	c.End()
	if !slow.cancelled || stale || a.Shader() != a0 || c.Pending(a) {
		t.Fatal("canceled dependency allowed its older source to commit")
	}
	c.QueueMaterial(a, second, func() { latest = true })
	dev.jobs[len(dev.jobs)-1].ready = true
	c.Begin()
	c.End()
	if stale || !latest || a.Shader() != b2 || b.Shader() != b2 || a.Failed() {
		t.Fatal("new source did not recover after canceled target")
	}
}

func TestCommitCallbackCanQueueNewMaterialDefinition(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	c := NewCache(dev, false, spritePipe)
	defer c.Close()
	sh := testShader("shared", nil, []uint32{0}, pspec{"F", shader.KindFloat, [4]float32{}, false})
	m, _ := New("m", sh, 0, shader.BlendPremultiplied)
	first, _ := New("first", sh, 0, shader.BlendPremultiplied)
	second, _ := New("second", sh, 0, shader.BlendPremultiplied)
	first.Set("F", [4]float32{1})
	second.Set("F", [4]float32{2})
	c.QueueMaterial(m, first, func() { c.QueueMaterial(m, second, nil) })
	c.Begin()
	c.End()
	if v, _ := m.Get("F"); v[0] != 1 || !c.Pending(m) {
		t.Fatal("new callback request was deleted with its completed predecessor")
	}
	c.Begin()
	c.End()
	if v, _ := m.Get("F"); v[0] != 2 || c.Pending(m) {
		t.Fatal("callback request did not commit on the following boundary")
	}
}
