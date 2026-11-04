package texture

import (
	"aego/core/mathx"
	"aego/gpu"
)

type Region struct {
	SourceW, SourceH uint16
	TrimX, TrimY     uint16
	Texture          gpu.TextureHandle
	UVMin            mathx.Vec2
	UVMax            mathx.Vec2
	W, H             uint16
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
	if r.Trimmed() {
		return r.trimmedSub(x, y, w, h)
	}
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
	size := r.Size()
	cw := int(size.X) / cols
	ch := int(size.Y) / rows
	return r.Sub((index%cols)*cw, (index/cols)*ch, cw, ch)
}

func (r Region) Size() mathx.Vec2 {
	if r.SourceW > 0 && r.SourceH > 0 {
		return mathx.Vec2{X: float32(r.SourceW), Y: float32(r.SourceH)}
	}
	return mathx.Vec2{X: float32(r.W), Y: float32(r.H)}
}

func (r Region) IsZero() bool {
	return r.Texture.IsZero()
}

func (r Region) FlippedX() Region {
	r.UVMin.X, r.UVMax.X = r.UVMax.X, r.UVMin.X
	if r.Trimmed() {
		r.TrimX = r.SourceW - r.TrimX - r.W
	}
	return r
}

func (r Region) FlippedY() Region {
	r.UVMin.Y, r.UVMax.Y = r.UVMax.Y, r.UVMin.Y
	if r.Trimmed() {
		r.TrimY = r.SourceH - r.TrimY - r.H
	}
	return r
}

// Trimmed reports a rectangular crop of the original sprite canvas.
func (r Region) Trimmed() bool {
	return r.SourceW > 0 && r.SourceH > 0 && (r.SourceW != r.W || r.SourceH != r.H || r.TrimX != 0 || r.TrimY != 0)
}

// LocalRect gives source-original normalized UV endpoints for the packed quad.
// Reversed endpoints preserve FlipX/Y while atlas coordinates remain separate.
func (r Region) LocalRect() [4]float32 {
	out := [4]float32{0, 0, 1, 1}
	if r.Trimmed() {
		out = [4]float32{float32(r.TrimX) / float32(r.SourceW), float32(r.TrimY) / float32(r.SourceH), float32(r.TrimX+r.W) / float32(r.SourceW), float32(r.TrimY+r.H) / float32(r.SourceH)}
	}
	if r.UVMin.X > r.UVMax.X {
		out[0], out[2] = 1-out[0], 1-out[2]
	}
	if r.UVMin.Y > r.UVMax.Y {
		out[1], out[3] = 1-out[1], 1-out[3]
	}
	return out
}

func (r Region) trimmedSub(x, y, w, h int) Region {
	if w <= 0 || h <= 0 {
		return Region{}
	}
	x0, y0 := max(x, int(r.TrimX)), max(y, int(r.TrimY))
	x1, y1 := min(x+w, int(r.TrimX+r.W)), min(y+h, int(r.TrimY+r.H))
	if x1 <= x0 || y1 <= y0 {
		return Region{}
	}
	du, dv := (r.UVMax.X-r.UVMin.X)/float32(r.W), (r.UVMax.Y-r.UVMin.Y)/float32(r.H)
	return Region{Texture: r.Texture, UVMin: mathx.Vec2{X: r.UVMin.X + du*float32(x0-int(r.TrimX)), Y: r.UVMin.Y + dv*float32(y0-int(r.TrimY))}, UVMax: mathx.Vec2{X: r.UVMin.X + du*float32(x1-int(r.TrimX)), Y: r.UVMin.Y + dv*float32(y1-int(r.TrimY))}, W: uint16(x1 - x0), H: uint16(y1 - y0), SourceW: uint16(w), SourceH: uint16(h), TrimX: uint16(x0 - x), TrimY: uint16(y0 - y)}
}
