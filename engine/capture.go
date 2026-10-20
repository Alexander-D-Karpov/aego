package engine

import (
	"image"

	"aego/core/errs"
	"aego/gpu"
)

// CaptureFrame calls fn once with the next presented frame, top row first.
func (e *Engine) CaptureFrame(fn func(*image.RGBA, error)) {
	e.capture = fn
}

func (e *Engine) grab() {
	fn := e.capture
	e.capture = nil
	r, ok := e.dev.(gpu.BackbufferReader)
	if !ok {
		fn(nil, errs.New(errs.GPUResource, "this device cannot read the backbuffer"))
		return
	}
	w, h := e.metrics.PixelW, e.metrics.PixelH
	buf := make([]byte, w*h*4)
	if err := r.ReadBackbuffer(w, h, buf); err != nil {
		fn(nil, err)
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	row := w * 4
	for y := 0; y < h; y++ {
		copy(img.Pix[y*row:(y+1)*row], buf[(h-1-y)*row:(h-y)*row])
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	fn(img, nil)
}
