//go:build js

package webgl2

import (
	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
)

type fboKey struct {
	color handle.Handle[glTexture]
	depth handle.Handle[glTexture]
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

func (d *Device) framebuffer(color, depth handle.Handle[glTexture]) (uint32, int, int, bool, error) {
	ct, ok := d.textures.Get(color)
	if !ok {
		return 0, 0, 0, false, errs.New(errs.GPUInvalidHandle, "framebuffer: color texture handle is invalid")
	}
	cw, ch, cinfo, cid := ct.w, ct.h, ct.info, ct.id
	depth = d.validDepth(cinfo.depth, cw, ch, depth)
	hasDepth := cinfo.depth || !depth.IsZero()
	key := fboKey{color: color, depth: depth}
	if fbo, ok := d.fbos[key]; ok {
		return fbo, cw, ch, hasDepth, nil
	}
	dinfo := texInfo{name: "none"}
	var did uint32
	if !depth.IsZero() {
		dt, _ := d.textures.Get(depth)
		did, dinfo = dt.id, dt.info
	}
	fbo := d.obj.put(d.call(d.f.CreateFramebuffer))
	d.cache.BindFramebuffer(fbo)
	if cinfo.depth {
		d.call(d.f.FramebufferTexture2D, glFramebuffer, cinfo.attach, glTexture2D, d.obj.get(cid), 0)
		d.call(d.f.DrawBuffers, d.drawNone)
		d.call(d.f.ReadBuffer, glNone)
	} else {
		d.call(d.f.FramebufferTexture2D, glFramebuffer, glColorAttachment0, glTexture2D, d.obj.get(cid), 0)
		d.call(d.f.DrawBuffers, d.drawColor)
		d.call(d.f.ReadBuffer, glColorAttachment0)
	}
	if did != 0 {
		d.call(d.f.FramebufferTexture2D, glFramebuffer, dinfo.attach, glTexture2D, d.obj.get(did), 0)
	}
	status := uint32(num(d.call(d.f.CheckFramebufferStatus, glFramebuffer)))
	if status != glFramebufferComplete && !d.lost {
		d.cache.BindFramebuffer(0)
		d.cache.ForgetFramebuffer(fbo)
		d.call(d.f.DeleteFramebuffer, d.obj.drop(fbo))
		return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer is incomplete",
			log.F("status", fbStatusName(status)), log.F("status_code", status),
			log.F("color_format", cinfo.name), log.F("depth_format", dinfo.name),
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
		d.cache.ForgetFramebuffer(fbo)
		d.call(d.f.DeleteFramebuffer, d.obj.drop(fbo))
		delete(d.fbos, k)
	}
}

func (d *Device) deleteFramebuffers() {
	for k, fbo := range d.fbos {
		d.cache.ForgetFramebuffer(fbo)
		d.call(d.f.DeleteFramebuffer, d.obj.drop(fbo))
		delete(d.fbos, k)
	}
}
