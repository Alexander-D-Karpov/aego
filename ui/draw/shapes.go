package draw

import (
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/texture"
)

const (
	quarter = 16
	whiteX  = 4*quarter + 4
	shapeW  = 128
	shapeH  = 2*quarter + 2*32 + 2*64
)

var discSizes = [...]int{16, 32, 64}

var discY = [...]int{0, 2 * quarter, 2*quarter + 2*32}

type Corners uint8

const (
	CornerTL Corners = 1 << iota
	CornerTR
	CornerBL
	CornerBR
	CornersTop  = CornerTL | CornerTR
	CornersLeft = CornerTL | CornerBL
	CornersAll  = CornerTL | CornerTR | CornerBL | CornerBR
)

type Shapes struct {
	White texture.Region
	disc  [len(discSizes)][4]texture.Region
	blur  [4]texture.Region
	edge  [4]texture.Region
}

func NewShapes(dev gpu.Device, up *texture.Uploader) (*Shapes, error) {
	data := texture.New(shapeW, shapeH, gpu.TexSRGBA8)
	data.Premultiplied = true
	tex, err := up.Upload(data, texture.UploadOptions{Label: "ui.shapes"})
	if err != nil {
		return nil, err
	}
	dev.UpdateTexture(tex, 0, 0, shapeW, shapeH, shapePixels())
	w := texture.Whole(tex, shapeW, shapeH)
	q := quarter
	s := &Shapes{White: w.Sub(whiteX+1, 1, 2, 2)}
	for i, o := range [4][2]int{{0, 0}, {q, 0}, {0, q}, {q, q}} {
		s.blur[i] = w.Sub(2*q+o[0], o[1], q, q)
		for k, n := range discSizes {
			s.disc[k][i] = w.Sub(o[0]*n/q, discY[k]+o[1]*n/q, n, n)
		}
	}
	s.edge = [4]texture.Region{w.Sub(3*q, 0, 1, q), w.Sub(3*q, q, 1, q), w.Sub(2*q, q, q, 1), w.Sub(3*q, q, q, 1)}
	return s, nil
}

func (s *Shapes) discs(rad float32) *[4]texture.Region {
	k := 0
	for k+1 < len(discSizes) && rad > float32(discSizes[k])*1.4 {
		k++
	}
	return &s.disc[k]
}

func shapePixels() []byte {
	buf := make([]byte, shapeW*shapeH*4)
	put := func(x, y int, a float32) {
		a = mathx.Clamp(a, 0, 1)
		v := byte(mathx.LinearToSRGB(a)*255 + 0.5)
		i := (y*shapeW + x) * 4
		buf[i], buf[i+1], buf[i+2], buf[i+3] = v, v, v, byte(a*255+0.5)
	}
	q := float32(quarter)
	for y := 0; y < 2*quarter; y++ {
		for x := 0; x < 2*quarter; x++ {
			d := mathx.Vec2{X: float32(x) + 0.5 - q, Y: float32(y) + 0.5 - q}.Len()
			t := mathx.Clamp(d/q, 0, 1)
			put(2*quarter+x, y, 1-t*t*(3-2*t))
		}
	}
	for k, n := range discSizes {
		r := float32(n)
		for y := 0; y < 2*n; y++ {
			for x := 0; x < 2*n; x++ {
				d := mathx.Vec2{X: float32(x) + 0.5 - r, Y: float32(y) + 0.5 - r}.Len()
				put(x, discY[k]+y, r-d+0.5)
			}
		}
	}
	for y := 0; y < 4; y++ {
		for x := whiteX; x < shapeW; x++ {
			put(x, y, 1)
		}
	}
	return buf
}
