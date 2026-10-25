package sdk

import "aego/modules/animation"

type (
	Animation       = animation.Service
	AnimationPlayer = animation.AnimationPlayer
	Animator        = animation.Animator
	AnimClip        = animation.ClipRef
	AnimController  = animation.ControllerRef
)

func AnimationOf(ctx *Context) *Animation {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	return animation.Of(ctx.World)
}
