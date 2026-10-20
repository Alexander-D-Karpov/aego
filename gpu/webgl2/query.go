//go:build js

package webgl2

import (
	"syscall/js"

	"aego/core/errs"
	"aego/core/handle"
	"aego/gpu"
)

type glQuery struct {
	id     uint32
	active bool
}

func (d *Device) CreateQuery() (gpu.QueryHandle, error) {
	if !d.caps.TimerQueries {
		return gpu.QueryHandle{}, errs.New(errs.GPUResource, "timer queries are not supported")
	}
	id := d.obj.put(d.call(d.f.CreateQuery))
	h, q := d.queries.Alloc()
	q.id = id
	return gpu.QueryHandle(h), nil
}

func (d *Device) DestroyQuery(h gpu.QueryHandle) {
	d.destroy(kindQuery, h.Index, h.Gen)
}

func (f *frame) QueryResult(h gpu.QueryHandle) (uint64, bool) {
	d := f.d
	q, ok := d.queries.Get(handle.Cast[glQuery](h))
	if !ok || q.active || d.lost {
		return 0, false
	}
	o := d.obj.get(q.id)
	if !d.call(d.f.GetQueryParameter, o, glQueryResultAvailable).Truthy() {
		return 0, false
	}
	v := d.call(d.f.GetQueryParameter, o, glQueryResult)
	if d.param(glGPUDisjoint).Truthy() || v.Type() != js.TypeNumber {
		return 0, true
	}
	return uint64(v.Float()), true
}

func (p *pass) BeginQuery(h gpu.QueryHandle) {
	d := p.d
	if d.activeQuery != 0 {
		d.log.Warn("nested gpu query ignored")
		return
	}
	q, ok := d.queries.Get(handle.Cast[glQuery](h))
	if !ok {
		return
	}
	d.call(d.f.BeginQuery, glTimeElapsed, d.obj.get(q.id))
	q.active = true
	d.activeQuery = q.id
	p.query = h
}

func (p *pass) EndQuery() {
	d := p.d
	if d.activeQuery == 0 {
		return
	}
	d.call(d.f.EndQuery, glTimeElapsed)
	if q, ok := d.queries.Get(handle.Cast[glQuery](p.query)); ok {
		q.active = false
	}
	d.activeQuery = 0
	p.query = gpu.QueryHandle{}
}
