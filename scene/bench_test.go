package scene

import (
	"testing"

	"aego/core/mathx"
)

func buildScene(b *testing.B, parents, children int) (*Scene, []NodeHandle) {
	b.Helper()
	s := New(newTypes(b), nil, Options{InitialNodes: parents * (children + 1)})
	var leaves []NodeHandle
	for i := 0; i < parents; i++ {
		p := s.Create(s.Root(), "p")
		for j := 0; j < children; j++ {
			c := s.Create(p, "c")
			sr := Add[Velocity](s, c)
			sr.Value = mathx.Vec2{X: float32(i), Y: float32(j)}
			leaves = append(leaves, c)
		}
	}
	s.Flush()
	s.UpdateTransforms()
	return s, leaves
}

func BenchmarkTransformAllDirty(b *testing.B) {
	s, _ := buildScene(b, 5000, 9)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.markWorldDirty(s.Root())
		s.UpdateTransforms()
	}
}

func BenchmarkTransformTenPercentDirty(b *testing.B) {
	s, leaves := buildScene(b, 5000, 9)
	step := len(leaves) / 10
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(leaves); j += step {
			s.SetPosition(leaves[j], mathx.Vec2{X: float32(i)})
		}
		s.UpdateTransforms()
	}
}

func BenchmarkQuery1(b *testing.B) {
	s, _ := buildScene(b, 5000, 9)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Query1[Velocity](s).Each(func(NodeHandle, *Velocity) {})
	}
}

func BenchmarkCreateDestroy(b *testing.B) {
	s, _ := buildScene(b, 100, 4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := s.Create(s.Root(), "tmp")
		Add[Velocity](s, h)
		s.Destroy(h)
		s.Flush()
	}
}
