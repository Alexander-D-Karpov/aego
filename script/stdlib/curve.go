package stdlib

import (
	"unicode"
	"unicode/utf8"

	"aego/core/curve"
	"aego/script/abi"
	"aego/script/bind"
)

func easingName(e curve.Easing) string {
	s := e.String()
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func resolveCurve(c *bind.Ctx, cv bind.Curve) *curve.Curve {
	p, err := cv.Resolve()
	if err != nil {
		c.FailErr(err)
		return nil
	}
	return p
}

func installCurve(r *bind.Registry) {
	m := r.Module(ModCurve)
	members := make([]abi.EnumMember, 0, int(curve.EasingCount))
	for e := curve.None; e < curve.EasingCount; e++ {
		members = append(members, bind.Member(easingName(e), e))
	}
	bind.Enum[curve.Easing](m, "Easing", members...)
	bind.Describe(m, "Easing", bind.Doc("Easing names a preset curve from 0 to 1. None and Linear move at a constant speed."))
	for e := curve.Linear; e < curve.EasingCount; e++ {
		n := easingName(e)
		bind.Const(m, n, e, bind.Doc(n+" is the "+e.String()+" easing preset."))
	}
	bind.Func2(m, "Eval", func(c *bind.Ctx, cv bind.Curve, t float32) float32 {
		if p := resolveCurve(c, cv); p != nil {
			return p.Eval(t)
		}
		return 0
	}, bind.Doc("Eval returns the value of a curve at time t; outside its keys the curve repeats as its Before and After settings say."),
		bind.Params("curve", "t"), bind.Example("speed := Speed * curve.Eval(node.Get<Mover>().Accel, elapsed)"))
	bind.Func2(m, "EvalClamped", func(c *bind.Ctx, cv bind.Curve, t float32) float32 {
		if p := resolveCurve(c, cv); p != nil {
			return p.EvalClamped(t)
		}
		return 0
	}, bind.Doc("EvalClamped returns the value at t clamped to the first and last key."),
		bind.Params("curve", "t"), bind.Example("k := curve.EvalClamped(fade, age)"))
	bind.Func1(m, "Duration", func(c *bind.Ctx, cv bind.Curve) float32 {
		if p := resolveCurve(c, cv); p != nil {
			return p.Duration()
		}
		return 0
	}, bind.Doc("Duration returns the time between the first and the last key, 1 for a preset."),
		bind.Params("curve"), bind.Example("t := age / curve.Duration(fade)"))
	bind.Func2(m, "Ease", func(c *bind.Ctx, e curve.Easing, t float32) float32 {
		return curve.Ease(e, t)
	}, bind.Doc("Ease returns a preset easing at t, clamped to 0..1."),
		bind.Params("easing", "t"), bind.Example("x := math.Lerp(a, b, curve.Ease(curve.OutCubic, k))"))
}
