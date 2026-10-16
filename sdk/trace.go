package sdk

import "aego/core/trace"

type (
	Tracer     = trace.Tracer
	TraceZone  = trace.ZoneID
	TraceScope = trace.Scope
)

func TracerOf(ctx *Context) *Tracer {
	if ctx == nil || ctx.Engine == nil {
		return nil
	}
	return ctx.Engine.Trace()
}
