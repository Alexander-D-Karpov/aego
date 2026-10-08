package sdk

import "aego/platform/input"

func InputOf(ctx *Context) *input.State {
	return ctx.Input()
}
