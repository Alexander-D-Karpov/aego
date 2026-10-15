package props

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
)

func TestEulerDegRoundTrip(t *testing.T) {
	e := mathx.Vec3{X: 10, Y: 20, Z: 30}
	if got := EulerDeg(QuatFromEulerDeg(e)); !got.NearlyEqual(e, 1e-3) {
		t.Fatalf("round trip = %v", got)
	}
}

func TestEulerCacheKeepsTypedAngles(t *testing.T) {
	c := NewEulerCache()
	id := ids.NewNodeID()
	typed := mathx.Vec3{X: 180, Y: 30}
	q := c.Set(id, typed)
	if got := c.Get(id, q); got != typed {
		t.Fatalf("cached = %v", got)
	}
	other := mathx.QuatAxisAngle(mathx.Vec3{Y: 1}, mathx.Deg2Rad(45))
	if got := c.Get(id, other); !got.NearlyEqual(mathx.Vec3{Y: 45}, 1e-3) {
		t.Fatalf("external change = %v", got)
	}
	c.Retain(func(ids.NodeID) bool { return false })
	if len(c.entries) != 0 {
		t.Fatal("retain did not drop entries")
	}
}
