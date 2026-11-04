//go:build js

package webgl2

import (
	"bytes"
	"syscall/js"

	"aego/core/mathx"
	"aego/gpu/internal/glstate"
)

// The caller redraws this 8×8 tile with the reference resolve if the clear differs.
func (d *Device) MatchWindowClear(x, y int, color mathx.Color) bool {
	b := &d.bb
	if d.lost || b.fbo == 0 || x < 0 || y < 0 || x+8 > b.w || y+8 > b.h {
		return false
	}
	y = b.h - y - 8
	var before, after [8 * 8 * 4]byte
	d.cache.BindFramebuffer(b.fbo)
	d.call(d.f.ReadBuffer, glColorAttachment0)
	d.call(d.f.PixelStorei, glPackAlignment, 1)
	read := func(dst []byte) bool {
		jsBuf := d.scratch(len(dst))
		d.call(d.f.ReadPixels, x, y, 8, 8, glRGBA, glUnsignedByte, jsBuf, 0)
		return num(d.call(d.f.GetError)) == glNoError && js.CopyBytesToGo(dst, jsBuf) == len(dst)
	}
	if !read(before[:]) {
		return false
	}
	d.cache.SetScissor(true, glstate.Rect{X: int32(x), Y: int32(y), W: 8, H: 8})
	d.call(d.f.ClearColor, color.R, color.G, color.B, color.A)
	d.call(d.f.Clear, glColorBufferBit)
	d.cache.SetScissor(false, glstate.Rect{})
	return read(after[:]) && bytes.Equal(before[:], after[:])
}
