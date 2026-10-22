package systems

import (
	"aego/contracts/physics2d"
	"aego/core/mathx"
	"aego/modules/physics/world"
	"aego/scene"
)

var _ physics2d.Queries = (*Sim)(nil)

func (m *Sim) BodyNode(h world.BodyHandle) scene.NodeHandle {
	v, _ := m.w.BodyUserData(h)
	return Unpack(v)
}

func (m *Sim) ShapeNode(h world.ShapeHandle) scene.NodeHandle {
	v, _ := m.w.ShapeUserData(h)
	return Unpack(v)
}

func (m *Sim) filter(f physics2d.QueryFilter) world.QueryFilter {
	wf := world.QueryFilter{LayerMask: f.LayerMask, IncludeTriggers: f.IncludeTriggers, SkipDynamic: f.SkipDynamic}
	ex := f.ExcludeBody
	if ex.IsZero() {
		ex = f.ExcludeNode
	}
	if !ex.IsZero() {
		if b := m.bodyAt(ex); b != nil {
			wf.ExcludeBody = b.h
		}
	}
	return wf
}

func (m *Sim) hit(h world.RayHit) physics2d.RayHit {
	return physics2d.RayHit{
		Point:        h.Point,
		Normal:       h.Normal,
		Distance:     h.Distance,
		Node:         m.BodyNode(h.Body),
		ColliderNode: m.ShapeNode(h.Shape),
	}
}

func (m *Sim) Raycast(origin, dir mathx.Vec2, maxDist float32, f physics2d.QueryFilter) (physics2d.RayHit, bool) {
	h, ok := m.w.Raycast(origin, dir, maxDist, m.filter(f))
	if !ok {
		return physics2d.RayHit{}, false
	}
	return m.hit(h), true
}

func (m *Sim) RaycastAll(origin, dir mathx.Vec2, maxDist float32, f physics2d.QueryFilter, out []physics2d.RayHit) int {
	if len(out) == 0 {
		return 0
	}
	if cap(m.qhits) < len(out) {
		m.qhits = make([]world.RayHit, len(out))
	}
	buf := m.qhits[:len(out)]
	n := m.w.RaycastAll(origin, dir, maxDist, m.filter(f), buf)
	for i := 0; i < n; i++ {
		out[i] = m.hit(buf[i])
	}
	return n
}

func (m *Sim) scratch(n int) []world.ShapeHandle {
	if cap(m.qshapes) < n {
		m.qshapes = make([]world.ShapeHandle, n)
	}
	return m.qshapes[:n]
}

// nodes maps shapes to their body nodes, dropping duplicates.
func (m *Sim) nodes(shapes []world.ShapeHandle, out []scene.NodeHandle) int {
	k := 0
	for _, sh := range shapes {
		bh, ok := m.w.ShapeBody(sh)
		if !ok {
			continue
		}
		node := m.BodyNode(bh)
		dup := false
		for _, x := range out[:k] {
			if x == node {
				dup = true
				break
			}
		}
		if !dup {
			out[k] = node
			k++
		}
	}
	return k
}

func (m *Sim) OverlapPoint(p mathx.Vec2, f physics2d.QueryFilter, out []scene.NodeHandle) int {
	buf := m.scratch(len(out))
	return m.nodes(buf[:m.w.OverlapPoint(p, m.filter(f), buf)], out)
}

func (m *Sim) OverlapCircle(c mathx.Vec2, r float32, f physics2d.QueryFilter, out []scene.NodeHandle) int {
	buf := m.scratch(len(out))
	return m.nodes(buf[:m.w.OverlapCircle(c, r, m.filter(f), buf)], out)
}

func (m *Sim) OverlapBox(c, half mathx.Vec2, angle float32, f physics2d.QueryFilter, out []scene.NodeHandle) int {
	buf := m.scratch(len(out))
	return m.nodes(buf[:m.w.OverlapBox(c, half, angle, m.filter(f), buf)], out)
}

func (m *Sim) BodyAt(p mathx.Vec2, f physics2d.QueryFilter) (scene.NodeHandle, bool) {
	sh, ok := m.w.ShapeAt(p, m.filter(f))
	if !ok {
		return scene.NodeHandle{}, false
	}
	bh, ok := m.w.ShapeBody(sh)
	if !ok {
		return scene.NodeHandle{}, false
	}
	return m.BodyNode(bh), true
}
