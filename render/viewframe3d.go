package render

import (
	"aego/core/mathx"
	"aego/render/mesh"
	"aego/render/shader"
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
	f.trackResolveBounds = false
	r := f.r
	if d.Mesh == nil {
		return
	}
	if (d.Material == nil || !d.Material.Shader().Vertex) && !f.frustum.ContainsAABB(d.Mesh.Bounds.Transformed(d.World)) {
		r.stats.MeshCulled++
		return
	}
	if d.Is2D && d.Material != nil && d.Material.Shader().Features&shader.FeatureLighting != 0 {
		copyDraw := *d
		d = &copyDraw
		b := d.Mesh.Bounds.Transformed(d.World)
		d.LightSet = f.lightSet(mathx.AABB2{Min: mathx.Vec2{X: b.Min.X, Y: b.Min.Y}, Max: mathx.Vec2{X: b.Max.X, Y: b.Max.Y}}, d.VisibilityLayers)
		if normal, explicit, _ := d.Material.Normal(); explicit {
			d.Normal = normal
		}
		if emission, explicit, _ := d.Material.Emission(); explicit {
			d.Emission = emission
		}
	}
	if d.Blend == sprite.BlendAdditive || d.Material != nil && d.Material.Blend() == shader.BlendAdditive {
		f.hasAdditive = true
	}
	if !r.meshes.PushOrdered(d, f.cam, r.White(), r.defSampler, f.submission, f.view.ZMin, f.view.ZMax) {
		r.stats.Dropped++
		f.warnBudget()
		return
	}
	f.submission++
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
	f.trackResolveBounds = false
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
