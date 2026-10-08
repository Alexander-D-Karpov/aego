package render

import (
	"aego/core/mathx"
	"aego/render/mesh"
	"aego/render/sprite"
)

func (f *ViewFrame) Mesh(d mesh.Draw) {
	f.submitMesh(&d)
}

func (f *ViewFrame) Meshes(ds []mesh.Draw) {
	for i := range ds {
		f.submitMesh(&ds[i])
	}
}

func (f *ViewFrame) submitMesh(d *mesh.Draw) {
	r := f.r
	if d.Mesh == nil {
		return
	}
	if !f.frustum.ContainsAABB(d.Mesh.Bounds.Transformed(d.World)) {
		r.stats.MeshCulled++
		return
	}
	if !r.meshes.Push(d, f.cam, r.White(), r.defSampler) {
		r.stats.Dropped++
		f.warnBudget()
	}
}

func (f *ViewFrame) Sprite3D(d sprite.Draw3D) {
	f.submit3D(&d)
}

func (f *ViewFrame) Sprites3D(ds []sprite.Draw3D) {
	for i := range ds {
		f.submit3D(&ds[i])
	}
}

func (f *ViewFrame) submit3D(d *sprite.Draw3D) {
	r := f.r
	if d.Region.IsZero() {
		return
	}
	inst := d.Instance()
	if !f.frustum.ContainsAABB(inst.Bounds()) {
		r.stats.Sprite3DCulled++
		return
	}
	if !r.sprites3d.Push(inst, d, r.defSampler, f.cam) {
		r.stats.Dropped++
		f.warnBudget()
		return
	}
	r.stats.Sprites3D++
}

func (f *ViewFrame) warnBudget() {
	r := f.r
	if r.warned {
		return
	}
	r.warned = true
	r.log.Warn("render budget exceeded, extra draws are dropped")
}

func (f *ViewFrame) Frustum() mathx.Frustum {
	return f.frustum
}
