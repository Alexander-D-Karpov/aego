//go:build js

package webgl2

import (
	"aego/core/errs"
	"aego/core/handle"
	"aego/core/log"
	"aego/gpu"
)

type fboKey struct {
	color              handle.Handle[glTexture]
	depth              handle.Handle[glTexture]
	colorSub, depthSub gpu.TextureSubresource
	generation         uint64
}

func (d *Device) attachTexture(attachment uint32, t *glTexture, sub gpu.TextureSubresource) {
	if t.desc.Dimension == gpu.Texture2DArray {
		d.call(d.f.FramebufferTextureLayer, glFramebuffer, attachment, d.obj.get(t.id), sub.Mip, sub.Layer)
	} else {
		d.call(d.f.FramebufferTexture2D, glFramebuffer, attachment, glTexture2D, d.obj.get(t.id), sub.Mip)
	}
}

func (d *Device) framebufferSub(color handle.Handle[glTexture], colorSub gpu.TextureSubresource, depth handle.Handle[glTexture], depthSub gpu.TextureSubresource) (uint32, int, int, bool, error) {
	if d.lost {
		return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer: context is lost")
	}
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
	key := fboKey{color: color, depth: depth, colorSub: colorSub, depthSub: depthSub, generation: d.contextGen}
	if fbo, ok := d.fbos[key]; ok {
		return fbo, cw, ch, hasDepth, nil
	}
	for num(d.call(d.f.GetError)) != glNoError {
	}
	fbo := d.obj.put(d.call(d.f.CreateFramebuffer))
	d.cache.BindFramebuffer(fbo)
	if ct.info.depth {
		d.attachTexture(ct.info.attach, ct, colorSub)
		d.call(d.f.DrawBuffers, d.drawNone)
		d.call(d.f.ReadBuffer, glNone)
	} else {
		d.attachTexture(glColorAttachment0, ct, colorSub)
		d.call(d.f.DrawBuffers, d.drawColor)
		d.call(d.f.ReadBuffer, glColorAttachment0)
	}
	if dt != nil {
		d.attachTexture(dt.info.attach, dt, depthSub)
	}
	status, glErr := uint32(num(d.call(d.f.CheckFramebufferStatus, glFramebuffer))), uint32(num(d.call(d.f.GetError)))
	if status != glFramebufferComplete || glErr != glNoError || d.lost {
		d.cache.BindFramebuffer(0)
		d.cache.ForgetFramebuffer(fbo)
		d.call(d.f.DeleteFramebuffer, d.obj.drop(fbo))
		return 0, 0, 0, false, errs.New(errs.GPUResource, "framebuffer is incomplete", log.F("status", fbStatusName(status)), log.F("gl", glErrorName(glErr)), log.F("format", ct.info.name), log.F("layer", colorSub.Layer), log.F("mip", colorSub.Mip))
	}
	d.fbos[key] = fbo
	return fbo, cw, ch, hasDepth, nil
}

func (d *Device) ValidateRenderTarget(desc gpu.PassDesc) error {
	if d.lost {
		return errs.New(errs.GPUResource, "render target: context is lost")
	}
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
