package render2d

import (
	"aego/engine"
	"aego/engine/module"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene"
	"aego/scene/frame"
	"fmt"
)

func worldMaterials(ctx *module.AttachContext) *material.Cache {
	if ctx.World == nil {
		return nil
	}
	runner, ok := engine.ServiceOf[*frame.Runner](ctx.World, frame.ServiceID)
	if !ok || runner == nil || runner.Renderer() == nil {
		return nil
	}
	return runner.Renderer().Materials()
}

func checkMaterialSwap(sc *scene.Scene, next *material.Shader, match func(*material.Material) bool) error {
	var failed error
	check := func(h scene.NodeHandle, domain shader.Domain) {
		if failed != nil || !sc.IsAlive(h) {
			return
		}
		m, err := NodeMaterial(sc, h)
		if err != nil || !match(m) {
			return
		}
		if next.Domain != domain {
			failed = fmt.Errorf("node %s material domain changed from %s to %s", sc.Name(h), domain, next.Domain)
			return
		}
		o, ok := scene.Get[MaterialOverride](sc, h)
		if !ok {
			return
		}
		for _, value := range o.Values {
			p, ok := next.Schema.Param(value.Name)
			if !ok {
				continue
			}
			if p.Kind != value.Kind || !p.Instance {
				failed = fmt.Errorf("node %s explicit material parameter %s is incompatible; keeping last-good", sc.Name(h), value.Name)
				return
			}
		}
	}
	scene.Store[SpriteRenderer](sc).EachAll(func(h scene.NodeHandle, _ *SpriteRenderer) { check(h, shader.DomainSprite) })
	scene.Store[Mesh2D](sc).EachAll(func(h scene.NodeHandle, _ *Mesh2D) { check(h, shader.DomainMesh2D) })
	return failed
}

func (m Module) AttachWorld(ctx *module.AttachContext) error {
	m.attachLighting(ctx)
	cache := worldMaterials(ctx)
	if cache == nil || ctx.Scene == nil {
		return nil
	}
	sc := ctx.Scene
	key := fmt.Sprintf("render2d:%p", sc)
	cache.SetReloadCheck(key, func(old, next *material.Shader) error {
		return checkMaterialSwap(sc, next, func(m *material.Material) bool { return m.Shader() == old })
	})
	cache.SetMaterialReloadCheck(key, func(current *material.Material, next *material.Shader) error {
		return checkMaterialSwap(sc, next, func(m *material.Material) bool { return m == current })
	})
	return nil
}
func (Module) DetachWorld(ctx *module.AttachContext) {
	if cache := worldMaterials(ctx); cache != nil && ctx.Scene != nil {
		key := fmt.Sprintf("render2d:%p", ctx.Scene)
		cache.SetReloadCheck(key, nil)
		cache.SetMaterialReloadCheck(key, nil)
	}
	detachLighting(ctx)
}
