package physics

import (
	"aego/core/mathx"
	"aego/engine/module"
	"aego/modules/physics/components"
)

func center(r mathx.Rect) mathx.Vec2 {
	return mathx.Vec2{X: r.X + r.W/2, Y: r.Y + r.H/2}
}

func fitBox(ctx module.AddContext, c *components.BoxCollider2D) {
	if r, ok := ctx.VisualRect(); ok {
		c.Offset, c.Size = center(r), mathx.Vec2{X: r.W, Y: r.H}
	}
}

func fitCircle(ctx module.AddContext, c *components.CircleCollider2D) {
	if r, ok := ctx.VisualRect(); ok {
		c.Offset, c.Radius = center(r), min(r.W, r.H)/2
	}
}

func fitCapsule(ctx module.AddContext, c *components.CapsuleCollider2D) {
	if r, ok := ctx.VisualRect(); ok {
		c.Offset, c.Length, c.Radius = center(r), r.W, min(r.W, r.H)/2
	}
}
