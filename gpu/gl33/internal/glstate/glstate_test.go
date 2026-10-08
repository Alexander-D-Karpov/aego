package glstate

import "testing"

type mockAPI struct {
	calls map[string]int
}

func newMock() *mockAPI {
	return &mockAPI{calls: map[string]int{}}
}

func (m *mockAPI) UseProgram(uint32)             { m.calls["program"]++ }
func (m *mockAPI) BindVertexArray(uint32)        { m.calls["vao"]++ }
func (m *mockAPI) BindUniformBuffer(int, uint32) { m.calls["ubo"]++ }
func (m *mockAPI) BindTexture(int, uint32)       { m.calls["tex"]++ }
func (m *mockAPI) BindSampler(int, uint32)       { m.calls["smp"]++ }
func (m *mockAPI) BindFramebuffer(uint32)        { m.calls["fbo"]++ }
func (m *mockAPI) SetBlend(Blend)                { m.calls["blend"]++ }
func (m *mockAPI) SetDepth(Depth)                { m.calls["depth"]++ }
func (m *mockAPI) SetCull(Cull)                  { m.calls["cull"]++ }
func (m *mockAPI) FrontFace(bool)                { m.calls["front"]++ }
func (m *mockAPI) Viewport(Rect)                 { m.calls["viewport"]++ }
func (m *mockAPI) Scissor(bool, Rect)            { m.calls["scissor"]++ }

func TestRedundantCallsSkipped(t *testing.T) {
	m := newMock()
	c := New(m)
	for i := 0; i < 5; i++ {
		c.UseProgram(3)
		c.BindVertexArray(4)
		c.BindUniformBuffer(0, 7)
		c.BindTexture(0, 9)
		c.BindSampler(0, 2)
		c.BindFramebuffer(0)
		c.SetBlend(Blend{Enabled: true, SrcRGB: 1, DstRGB: 2, SrcA: 1, DstA: 2})
		c.SetDepth(Depth{Test: false, Write: true, Func: 1})
		c.SetCull(Cull{})
		c.SetFrontFace(true)
		c.SetViewport(Rect{W: 800, H: 600})
		c.SetScissor(false, Rect{X: 1})
	}
	for name, n := range m.calls {
		if n != 1 {
			t.Fatalf("%s called %d times", name, n)
		}
	}
	if len(m.calls) != 12 {
		t.Fatalf("expected 12 distinct calls, got %v", m.calls)
	}
}

func TestFrontFaceTracksChanges(t *testing.T) {
	m := newMock()
	c := New(m)
	if !c.SetFrontFace(false) || c.SetFrontFace(false) || !c.SetFrontFace(true) {
		t.Fatal("front face change tracking wrong")
	}
	c.Invalidate()
	if !c.SetFrontFace(true) {
		t.Fatal("invalidate did not force front face")
	}
}

func TestChangesAndInvalidate(t *testing.T) {
	m := newMock()
	c := New(m)
	if !c.BindTexture(1, 5) || c.BindTexture(1, 5) || !c.BindTexture(1, 6) {
		t.Fatal("texture change tracking wrong")
	}
	if !c.BindTexture(2, 6) {
		t.Fatal("units must be independent")
	}
	c.Invalidate()
	if !c.BindTexture(1, 6) {
		t.Fatal("invalidate did not force rebind")
	}
	if m.calls["tex"] != 4 {
		t.Fatalf("tex calls = %d", m.calls["tex"])
	}
}

func TestDisabledStatesNormalised(t *testing.T) {
	m := newMock()
	c := New(m)
	c.SetBlend(Blend{})
	c.SetBlend(Blend{SrcRGB: 5})
	c.SetScissor(false, Rect{X: 1})
	c.SetScissor(false, Rect{X: 2})
	if m.calls["blend"] != 1 || m.calls["scissor"] != 1 {
		t.Fatalf("disabled state caused extra calls: %v", m.calls)
	}
}

func TestForget(t *testing.T) {
	m := newMock()
	c := New(m)
	c.BindTexture(0, 9)
	c.BindTexture(3, 9)
	c.ForgetTexture(9)
	c.BindTexture(0, 9)
	c.BindTexture(3, 9)
	if m.calls["tex"] != 4 {
		t.Fatalf("forget did not invalidate all units: %d", m.calls["tex"])
	}
	c.UseProgram(1)
	c.ForgetProgram(2)
	if c.UseProgram(1) {
		t.Fatal("forgetting other program invalidated current")
	}
	if c.BindUniformBuffer(MaxUniformBuffers, 1) || c.BindTexture(-1, 1) {
		t.Fatal("out of range slots accepted")
	}
}
