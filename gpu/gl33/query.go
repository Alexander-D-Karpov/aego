package gl33

import (
	"aego/core/errs"
	"aego/core/handle"
	"aego/gpu"
	"aego/gpu/gl33/gl"
)

type glQuery struct {
	id     uint32
	active bool
}

func (d *Device) CreateQuery() (gpu.QueryHandle, error) {
	if !d.caps.TimerQueries {
		return gpu.QueryHandle{}, errs.New(errs.GPUResource, "timer queries are not supported")
	}
	var id uint32
	gl.GenQueries(1, &id)
	if err := d.checkError("create query", ""); err != nil {
		gl.DeleteQueries(1, &id)
		return gpu.QueryHandle{}, err
	}
	h, q := d.queries.Alloc()
	q.id = id
	return gpu.QueryHandle(h), nil
}

func (d *Device) DestroyQuery(h gpu.QueryHandle) {
	d.destroy(kindQuery, h.Index, h.Gen)
}

func (f *frame) QueryResult(h gpu.QueryHandle) (uint64, bool) {
	q, ok := f.d.queries.Get(handle.Cast[glQuery](h))
	if !ok || q.active {
		return 0, false
	}
	f.qAvail = 0
	gl.GetQueryObjectiv(q.id, gl.QUERY_RESULT_AVAILABLE, &f.qAvail)
	if f.qAvail == 0 {
		return 0, false
	}
	f.qNS = 0
	gl.GetQueryObjectui64v(q.id, gl.QUERY_RESULT, &f.qNS)
	return f.qNS, true
}

func (p *pass) BeginQuery(h gpu.QueryHandle) {
	if p.d.activeQuery != 0 {
		p.d.log.Warn("nested gpu query ignored")
		return
	}
	q, ok := p.d.queries.Get(handle.Cast[glQuery](h))
	if !ok {
		return
	}
	gl.BeginQuery(gl.TIME_ELAPSED, q.id)
	q.active = true
	p.d.activeQuery = q.id
	p.query = h
}

func (p *pass) EndQuery() {
	if p.d.activeQuery == 0 {
		return
	}
	gl.EndQuery(gl.TIME_ELAPSED)
	if q, ok := p.d.queries.Get(handle.Cast[glQuery](p.query)); ok {
		q.active = false
	}
	p.d.activeQuery = 0
	p.query = gpu.QueryHandle{}
}
