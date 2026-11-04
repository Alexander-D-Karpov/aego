package render2d

import (
	"strings"

	"aego/asset"
	"aego/contracts/shapes2d"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/render/lighting"
	"aego/scene"
)

func lightDiagnosticCode(err error) string {
	if err != nil && strings.HasPrefix(err.Error(), "LGT002:") {
		return "LGT002"
	}
	return "LGT001"
}

func lightingIssue(code, message, help string) []problems.Diagnostic {
	return []problems.Diagnostic{{Code: code, Severity: problems.Error, Message: message, Help: help, BlocksPlay: true}}
}

func validateLight2D(ctx module.ValidateContext, l *Light2D) []problems.Diagnostic {
	if err := l.descriptor().Validate(); err != nil {
		return lightingIssue(lightDiagnosticCode(err), err.Error(), "Use finite nonnegative dimensions and energy, ordered Spot angles and a positive Directional elevation. Only local lights cast shadows.")
	}
	if !l.Cookie.IsZero() {
		resolved := false
		if source, ok := ctx.Scene.Resolver().(asset.TextureMetadataSource); ok {
			info, found := source.TextureMetadata(l.Cookie.ID)
			resolved = found
			if found && info.Semantic != asset.TextureData && info.Semantic != asset.TextureMask && info.Semantic != asset.TextureHeight && info.Semantic != asset.TextureCookieMask {
				return lightingIssue("LGT006", "Light cookie must be a linear cookie-mask, mask, data or height texture.", "Set the texture role to cookie-mask and choose its source channel. Color, normal and emission roles have different sampling contracts.")
			}
		} else {
			_, resolved = l.Cookie.Region(ctx.Scene.Resolver())
		}
		if !resolved {
			return []problems.Diagnostic{{Code: "LGT006", Severity: problems.Warning, Message: "Light cookie texture is unresolved; its AssetID is preserved.", Help: "Restore or replace the cookie texture."}}
		}
	}
	return nil
}

func validateLightOccluder2D(ctx module.ValidateContext, o *LightOccluder2D) []problems.Diagnostic {
	if o.Source > OccluderSpriteAlpha || o.CullMode > lighting.CullCounterClockwise {
		return lightingIssue("LGT004", "Unknown occluder source or culling mode.", "Choose a registered occluder source and winding policy.")
	}
	if o.Source == OccluderPolygon {
		if err := shapes2d.ValidateContour(o.Polygon, o.Closed); err != nil {
			return lightingIssue("LGT004", err.Error(), "Use a simple finite polygon without holes, or an open chain with distinct nonintersecting edges.")
		}
	} else if o.Source == OccluderSpriteAlpha {
		if !scene.Has[SpriteRenderer](ctx.Scene, ctx.Node) {
			return lightingIssue("LGT004", "Sprite alpha shadows require a sprite on this node.", "Add SpriteRenderer or select a polygon.")
		}
	} else if !o.ShapeNode.IsZero() {
		if _, ok := o.ShapeNode.Resolve(ctx.Scene); !ok {
			return []problems.Diagnostic{{Code: "LGT005", Severity: problems.Warning, Message: "Occluder shape-source node is unresolved; its NodeID is preserved.", Help: "Restore the node or choose an existing shape owner."}}
		}
	}
	return nil
}

func validateLightingEnvironment2D(ctx module.ValidateContext, e *LightingEnvironment2D) []problems.Diagnostic {
	if err := e.descriptor().Validate(); err != nil {
		return lightingIssue("LGT003", err.Error(), "Use nonnegative ambient, positive exposure and a supported shadow resolution/budget.")
	}
	if !e.Camera.IsZero() {
		h, ok := e.Camera.Resolve(ctx.Scene)
		if !ok || !scene.Has[Camera2D](ctx.Scene, h) {
			return lightingIssue("LGT003", "Lighting environment refers to a missing Camera2D; the NodeID is preserved.", "Assign an existing Camera2D or clear the reference to apply to the output.")
		}
	}
	if !ctx.Scene.EffectiveEnabled(ctx.Node) {
		return nil
	}
	conflict := false
	scene.Query1[LightingEnvironment2D](ctx.Scene).Each(func(h scene.NodeHandle, other *LightingEnvironment2D) {
		if h != ctx.Node && (e.Camera.IsZero() || other.Camera.IsZero() || e.Camera.ID == other.Camera.ID) {
			conflict = true
		}
	})
	if conflict {
		return lightingIssue("LGT003", "Several LightingEnvironment2D components apply to the same camera output.", "Keep one environment per camera output; a global environment also applies to every camera.")
	}
	return nil
}
