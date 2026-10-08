package mathx

import "testing"

func TestQuatLookDir(t *testing.T) {
	for _, d := range []Vec3{
		{Z: -1}, {X: 1}, {X: -2, Y: 1, Z: 3}, {Y: -2, Z: -6}, {X: 0.3, Y: 0.9, Z: -0.1},
	} {
		got := QuatLookDir(d).Rotate(Vec3{Z: -1})
		if !got.NearlyEqual(d.Normalized(), 1e-5) {
			t.Fatalf("look %v -> forward %v", d, got)
		}
	}
	if QuatLookDir(Vec3{}) != QuatIdentity() {
		t.Fatal("zero direction must give identity")
	}
}
