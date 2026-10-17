package sdk

import (
	"aego/contracts/debugdraw"
	"aego/engine"
)

type (
	DebugDraw     = debugdraw.Draw
	DebugStyle    = debugdraw.Style
	DebugLifetime = debugdraw.Lifetime
)

func Debug(ctx *Context) (DebugDraw, bool) {
	if ctx == nil || ctx.World == nil {
		return nil, false
	}
	return engine.ServiceOf[debugdraw.Draw](ctx.World, debugdraw.ServiceID)
}
