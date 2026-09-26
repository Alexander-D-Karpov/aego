package texture

import (
	"aego/core/mathx"
	"aego/gpu"
)

type Region struct {
	Texture gpu.TextureHandle
	UVMin   mathx.Vec2
	UVMax   mathx.Vec2
	W, H    uint16
}

func Whole(t gpu.TextureHandle, w, h int) Region {
	return Region{
		Texture: t,
		UVMin:   mathx.Vec2{},
		UVMax:   mathx.Vec2{X: 1, Y: 1},
		W:       uint16(w),
		H:       uint16(h),
	}
}

func (r Region) Sub(x, y, w, h int) Region {
	if r.W == 0 || r.H == 0 {
		return r
	}
	du := (r.UVMax.X - r.UVMin.X) / float32(r.W)
	dv := (r.UVMax.Y - r.UVMin.Y) / float32(r.H)
	return Region{
		Texture: r.Texture,
		UVMin:   mathx.Vec2{X: r.UVMin.X + du*float32(x), Y: r.UVMin.Y + dv*float32(y)},
		UVMax:   mathx.Vec2{X: r.UVMin.X + du*float32(x+w), Y: r.UVMin.Y + dv*float32(y+h)},
		W:       uint16(w),
		H:       uint16(h),
	}
}

func (r Region) Grid(cols, rows, index int) Region {
	if cols < 1 || rows < 1 {
		return r
	}
	cw := int(r.W) / cols
	ch := int(r.H) / rows
	return r.Sub((index%cols)*cw, (index/cols)*ch, cw, ch)
}

func (r Region) Size() mathx.Vec2 {
	return mathx.Vec2{X: float32(r.W), Y: float32(r.H)}
}

func (r Region) IsZero() bool {
	return r.Texture.IsZero()
}

func (r Region) FlippedX() Region {
	r.UVMin.X, r.UVMax.X = r.UVMax.X, r.UVMin.X
	return r
}

func (r Region) FlippedY() Region {
	r.UVMin.Y, r.UVMax.Y = r.UVMax.Y, r.UVMin.Y
	return r
}
