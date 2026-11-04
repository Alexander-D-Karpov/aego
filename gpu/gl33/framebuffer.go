package gl33

import (
	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/gl33/gl"
)

type fboKey struct {
	color              handle.Handle[glTexture]
	depth              handle.Handle[glTexture]
	colorSub, depthSub gpu.TextureSubresource
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

func (d *Device) attachTexture(attachment uint32, t *glTexture, sub gpu.TextureSubresource) {
	if t.desc.Dimension == gpu.Texture2DArray {
		gl.FramebufferTextureLayer(gl.FRAMEBUFFER, attachment, t.id, int32(sub.Mip), int32(sub.Layer))
	} else {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, attachment, gl.TEXTURE_2D, t.id, int32(sub.Mip))
	}
}

func (d *Device) framebufferSub(color handle.Handle[glTexture], colorSub gpu.TextureSubresource, depth handle.Handle[glTexture], depthSub gpu.TextureSubresource) (uint32, int, int, bool, error) {
	ct, ok := d.textures.Get(color)
	if !ok {
		return 0, 0, 0, false, errs.New(errs.GPUInvalidHandle, "framebuffer: color texture handle is invalid")
	}
	cw, ch, ok := ct.desc.SubresourceSize(colorSub)
	if !ok {
		return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer: color layer or mip is invalid")
	}
	if ct.desc.Format == gpu.TexRGBA16F && !d.caps.RGBA16FRenderable {
		return 0, 0, 0, false, errs.New(errs.GPUResource, "RGBA16F color target is unavailable on this device")
	}
	var dt *glTexture
	if !depth.IsZero() {
		dt, ok = d.textures.Get(depth)
		if !ok || !dt.info.depth || ct.info.depth {
			return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer: invalid depth target")
		}
		dw, dh, valid := dt.desc.SubresourceSize(depthSub)
		if !valid || dw != cw || dh != ch {
			return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer: depth subresource dimensions mismatch")
		}
	}
	hasDepth := ct.info.depth || dt != nil
	key := fboKey{color: color, depth: depth, colorSub: colorSub, depthSub: depthSub}
	if fbo, ok := d.fbos[key]; ok {
		return fbo, cw, ch, hasDepth, nil
	}
	for gl.GetError() != gl.NO_ERROR {
	}
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	d.cache.BindFramebuffer(fbo)
	if ct.info.depth {
		d.attachTexture(ct.info.attach, ct, colorSub)
		gl.DrawBuffer(gl.NONE)
		gl.ReadBuffer(gl.NONE)
	} else {
		d.attachTexture(gl.COLOR_ATTACHMENT0, ct, colorSub)
		gl.DrawBuffer(gl.COLOR_ATTACHMENT0)
		gl.ReadBuffer(gl.COLOR_ATTACHMENT0)
	}
	if dt != nil {
		d.attachTexture(dt.info.attach, dt, depthSub)
	}
	status, glErr := gl.CheckFramebufferStatus(gl.FRAMEBUFFER), gl.GetError()
	if status != gl.FRAMEBUFFER_COMPLETE || glErr != gl.NO_ERROR {
		d.cache.BindFramebuffer(0)
		d.cache.ForgetFramebuffer(fbo)
		gl.DeleteFramebuffers(1, &fbo)
		return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer is incomplete", log.F("status", fbStatusName(status)), log.F("gl", glErrorName(glErr)), log.F("format", ct.info.name), log.F("layer", colorSub.Layer), log.F("mip", colorSub.Mip))
	}
	d.fbos[key] = fbo
	return fbo, cw, ch, hasDepth, nil
}

func (d *Device) ValidateRenderTarget(desc gpu.PassDesc) error {
	if desc.Target.IsZero() {
		return nil
	}
	_, _, _, _, err := d.framebufferSub(handle.Cast[glTexture](desc.Target), desc.TargetSubresource, handle.Cast[glTexture](desc.DepthTarget), desc.DepthSubresource)
	return err
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
