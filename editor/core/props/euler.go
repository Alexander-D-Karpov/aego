package props

import (
	"aego/core/ids"
	"aego/core/mathx"
)

func EulerDeg(q mathx.Quat) mathx.Vec3 {
	if q.IsZero() {
		return mathx.Vec3{}
	}
	x, y, z := q.Euler()
	k := 180 / mathx.Pi
	return mathx.Vec3{X: x * k, Y: y * k, Z: z * k}
}

func QuatFromEulerDeg(e mathx.Vec3) mathx.Quat {
	return mathx.QuatEuler(mathx.Deg2Rad(e.X), mathx.Deg2Rad(e.Y), mathx.Deg2Rad(e.Z)).Normalized()
}

type eulerEntry struct {
	q mathx.Quat
	e mathx.Vec3
}

type EulerCache struct {
	entries map[ids.NodeID]eulerEntry
}

func NewEulerCache() *EulerCache {
	return &EulerCache{entries: map[ids.NodeID]eulerEntry{}}
}

func (c *EulerCache) Get(id ids.NodeID, q mathx.Quat) mathx.Vec3 {
	if e, ok := c.entries[id]; ok && e.q == q {
		return e.e
	}
	e := EulerDeg(q)
	c.entries[id] = eulerEntry{q: q, e: e}
	return e
}

func (c *EulerCache) Set(id ids.NodeID, e mathx.Vec3) mathx.Quat {
	q := QuatFromEulerDeg(e)
	c.entries[id] = eulerEntry{q: q, e: e}
	return q
}

func (c *EulerCache) Retain(keep func(ids.NodeID) bool) {
	for id := range c.entries {
		if !keep(id) {
			delete(c.entries, id)
		}
	}
}
