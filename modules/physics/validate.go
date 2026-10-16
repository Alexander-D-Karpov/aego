package physics

import (
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/physics/components"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
)

func colliderIssues(ctx module.ValidateContext, k components.Kind) []problems.Diagnostic {
	s, h := ctx.Scene, ctx.Node
	if s.Under3D(h) {
		return nil
	}
	var c components.Collider
	if !components.Read(s, h, k, &c) {
		return nil
	}
	def, _, code, err := systems.ShapeDef(&c, systems.Relative(s, h), h, nil)
	if err == nil {
		err = world.CheckShape(def, 0)
	}
	if err == nil {
		return nil
	}
	return []problems.Diagnostic{{
		Code:       code,
		Severity:   problems.Error,
		Message:    components.Names[k] + ": " + err.Error(),
		BlocksPlay: code == systems.CodeGeometry,
	}}
}

func validator[T any](k components.Kind) func(module.ValidateContext, *T) []problems.Diagnostic {
	return func(ctx module.ValidateContext, _ *T) []problems.Diagnostic {
		return colliderIssues(ctx, k)
	}
}
