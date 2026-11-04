//go:build js

package webgl2

import (
	"syscall/js"

	"aego/core/handle"
	"aego/core/log"
)

func (d *Device) restore() {
	d.lost = false
	d.contextGen++
	d.targetErrors = [16]string{}
	d.targetErrorCount = 0
	// Extension objects and optional capabilities belong to the context generation.
	d.queryCaps()
	d.cache.Invalidate()
	d.initState()
	for k, fbo := range d.fbos {
		d.obj.drop(fbo)
		delete(d.fbos, k)
	}
	d.obj.drop(d.bb.fbo)
	d.obj.drop(d.bb.color)
	d.obj.drop(d.bb.depth)
	d.bb = backbuffer{}
	d.buffers.Each(func(_ handle.Handle[glBuffer], b *glBuffer) {
		d.obj.reuse(b.id, d.call(d.f.CreateBuffer))
		b.last.Reset()
		d.uploadBuffer(b, b.shadow)
	})
	d.textures.Each(func(_ handle.Handle[glTexture], t *glTexture) {
		d.obj.reuse(t.id, d.call(d.f.CreateTexture))
		d.uploadTexture(t)
	})
	d.samplers.Each(func(_ handle.Handle[glSampler], s *glSampler) {
		d.obj.reuse(s.id, d.call(d.f.CreateSampler))
		d.applySampler(s.id, s.desc)
	})
	failed := 0
	d.shaders.Each(func(_ handle.Handle[glShader], s *glShader) {
		prog, err := d.link(s.desc)
		if err != nil {
			failed++
			d.log.Error("shader cannot be rebuilt after the context loss", log.F("label", s.desc.Label), log.F("err", err.Error()))
			d.obj.reuse(s.program, js.Null())
			return
		}
		d.obj.reuse(s.program, prog)
		d.bindProgramSlots(prog, s.program, s.desc.Label)
	})
	d.pipelines.Each(func(_ handle.Handle[glPipeline], p *glPipeline) {
		d.obj.reuse(p.vao, d.call(d.f.CreateVertexArray))
		p.bind.Invalidate()
		d.setupVAO(p)
	})
	d.queries.Each(func(_ handle.Handle[glQuery], q *glQuery) {
		d.obj.reuse(q.id, d.call(d.f.CreateQuery))
		q.active, q.issued, q.canceled = false, false, true
	})
	d.activeQuery = 0
	if err := d.initOutput(); err != nil {
		d.log.Error("the output pass cannot be rebuilt", log.F("err", err.Error()))
	}
	d.cache.Invalidate()
	d.restores++
	d.log.Info("WebGL context restored", log.F("buffers", d.buffers.Len()), log.F("textures", d.textures.Len()), log.F("shaders_failed", failed))
}
