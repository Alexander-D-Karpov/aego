package gl33

import (
	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu/gl33/gl"
)

type fboKey struct {
	color handle.Handle[glTexture]
	depth handle.Handle[glTexture]
}

func fbStatusName(s uint32) string {
	switch s {
	case gl.FRAMEBUFFER_UNDEFINED:
		return "UNDEFINED"
	case gl.FRAMEBUFFER_INCOMPLETE_ATTACHMENT:
		return "INCOMPLETE_ATTACHMENT"
	case gl.FRAMEBUFFER_INCOMPLETE_MISSING_ATTACHMENT:
		return "INCOMPLETE_MISSING_ATTACHMENT"
	case gl.FRAMEBUFFER_INCOMPLETE_DRAW_BUFFER:
		return "INCOMPLETE_DRAW_BUFFER"
	case gl.FRAMEBUFFER_INCOMPLETE_READ_BUFFER:
		return "INCOMPLETE_READ_BUFFER"
	case gl.FRAMEBUFFER_UNSUPPORTED:
		return "UNSUPPORTED"
	case gl.FRAMEBUFFER_INCOMPLETE_MULTISAMPLE:
		return "INCOMPLETE_MULTISAMPLE"
	}
	return "UNKNOWN"
}

func (d *Device) validDepth(colorDepth bool, cw, ch int, depth handle.Handle[glTexture]) handle.Handle[glTexture] {
	if depth.IsZero() {
		return depth
	}
	dt, ok := d.textures.Get(depth)
	switch {
	case !ok:
		d.log.Warn("depth target handle is invalid, rendering without depth")
	case colorDepth || !dt.info.depth:
		d.log.Warn("depth target must be a depth texture paired with a color texture")
	case dt.w != cw || dt.h != ch:
		d.log.Warn("depth target size differs from the color target, rendering without depth",
			log.F("color_w", cw), log.F("color_h", ch), log.F("depth_w", dt.w), log.F("depth_h", dt.h))
	default:
		return depth
	}
	return handle.Handle[glTexture]{}
}

func fbStatus() (uint32, uint32) {
	glErr := gl.GetError()
	return gl.CheckFramebufferStatus(gl.FRAMEBUFFER), glErr
}

func (d *Device) framebuffer(color, depth handle.Handle[glTexture]) (uint32, int, int, bool, error) {
	ct, ok := d.textures.Get(color)
	if !ok {
		return 0, 0, 0, false, errs.New(errs.GPUInvalidHandle, "framebuffer: color texture handle is invalid")
	}
	colorID, cw, ch, colorInfo := ct.id, ct.w, ct.h, ct.info
	colorDepth := colorInfo.depth
	depth = d.validDepth(colorDepth, cw, ch, depth)
	hasDepth := colorDepth || !depth.IsZero()
	key := fboKey{color: color, depth: depth}
	if fbo, ok := d.fbos[key]; ok {
		return fbo, cw, ch, hasDepth, nil
	}
	depthInfo := texInfo{name: "none"}
	var depthID uint32
	if !depth.IsZero() {
		dt, _ := d.textures.Get(depth)
		depthID, depthInfo = dt.id, dt.info
	}
	for gl.GetError() != gl.NO_ERROR {
	}
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	d.cache.Invalidate()
	stage := "color"
	if colorDepth {
		stage = "depth-only"
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, colorInfo.attach, gl.TEXTURE_2D, colorID, 0)
		gl.DrawBuffer(gl.NONE)
		gl.ReadBuffer(gl.NONE)
	} else {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, colorID, 0)
		gl.DrawBuffer(gl.COLOR_ATTACHMENT0)
		gl.ReadBuffer(gl.COLOR_ATTACHMENT0)
	}
	status, glErr := fbStatus()
	if status == gl.FRAMEBUFFER_COMPLETE && glErr == gl.NO_ERROR && depthID != 0 {
		stage = "depth"
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, depthInfo.attach, gl.TEXTURE_2D, depthID, 0)
		status, glErr = fbStatus()
	}
	if status != gl.FRAMEBUFFER_COMPLETE || glErr != gl.NO_ERROR {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.DeleteFramebuffers(1, &fbo)
		d.cache.Invalidate()
		return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer is incomplete",
			log.F("stage", stage), log.F("status", fbStatusName(status)), log.F("status_code", status),
			log.F("gl", glErrorName(glErr)), log.F("color_format", colorInfo.name), log.F("depth_format", depthInfo.name),
			log.F("w", cw), log.F("h", ch), log.F("renderer", d.caps.Renderer))
	}
	d.fbos[key] = fbo
	return fbo, cw, ch, hasDepth, nil
}

func (d *Device) forgetFramebuffers(h handle.Handle[glTexture]) {
	for k, fbo := range d.fbos {
		if k.color != h && k.depth != h {
			continue
		}
		id := fbo
		d.cache.ForgetFramebuffer(id)
		gl.DeleteFramebuffers(1, &id)
		delete(d.fbos, k)
	}
}

func (d *Device) deleteFramebuffers() {
	for k, fbo := range d.fbos {
		id := fbo
		gl.DeleteFramebuffers(1, &id)
		delete(d.fbos, k)
	}
}
