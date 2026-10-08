package mathx

type Rect struct{ X, Y, W, H float32 }

type AABB2 struct{ Min, Max Vec2 }

func (r Rect) Contains(p Vec2) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.X+r.W && p.Y < r.Y+r.H
}

func (r Rect) ToAABB() AABB2 {
	return AABB2{
		Min: Vec2{X: r.X, Y: r.Y},
		Max: Vec2{X: r.X + r.W, Y: r.Y + r.H},
	}
}

func (r Rect) Center() Vec2 {
	return Vec2{X: r.X + r.W/2, Y: r.Y + r.H/2}
}

func (a AABB2) Intersects(b AABB2) bool {
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X &&
		a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y
}

func (a AABB2) Contains(p Vec2) bool {
	return p.X >= a.Min.X && p.Y >= a.Min.Y && p.X <= a.Max.X && p.Y <= a.Max.Y
}

func (a AABB2) Union(b AABB2) AABB2 {
	return AABB2{
		Min: Vec2{X: min(a.Min.X, b.Min.X), Y: min(a.Min.Y, b.Min.Y)},
		Max: Vec2{X: max(a.Max.X, b.Max.X), Y: max(a.Max.Y, b.Max.Y)},
	}
}

func (a AABB2) Size() Vec2 {
	return a.Max.Sub(a.Min)
}

func (a AABB2) Center() Vec2 {
	return a.Min.Add(a.Max).Scale(0.5)
}
