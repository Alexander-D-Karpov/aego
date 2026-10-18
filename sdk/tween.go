package sdk

import (
	"aego/core/curve"
	"aego/modules/tween"
)

type (
	Tweens       = tween.Service
	TweenHandle  = tween.Handle
	TweenOptions = tween.Options
	TimerOptions = tween.TimerOptions
	Easing       = curve.Easing
	Curve        = curve.Curve
	CurveKey     = curve.Key
)

func TweensOf(ctx *Context) *Tweens {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	return tween.Of(ctx.World)
}
