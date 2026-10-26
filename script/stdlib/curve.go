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
		bind.Const(m, easingName(e), e, bind.Doc(easingDocs[e]))
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

// easingDocs say how each preset moves and what it suits.
var easingDocs = [curve.EasingCount]string{
	curve.Linear:       "Linear moves at a constant speed from start to end.",
	curve.InQuad:       "InQuad starts slowly and speeds up, gently: for things that set off, such as a door starting to open.",
	curve.OutQuad:      "OutQuad starts fast and slows down gently to the end: the usual choice for UI that slides in.",
	curve.InOutQuad:    "InOutQuad is slow at both ends and fastest in the middle, gently: for moves between two rest positions.",
	curve.InCubic:      "InCubic starts slowly and speeds up, more strongly than InQuad.",
	curve.OutCubic:     "OutCubic starts fast and slows down to the end, more strongly than OutQuad.",
	curve.InOutCubic:   "InOutCubic is slow at both ends and fast in the middle, more strongly than InOutQuad.",
	curve.InSine:       "InSine starts slowly and speeds up, the softest of the curves.",
	curve.OutSine:      "OutSine starts fast and slows down to the end, the softest of the curves.",
	curve.InOutSine:    "InOutSine is slow at both ends, the softest of the curves: for breathing, bobbing and fades.",
	curve.InExpo:       "InExpo barely moves at first, then covers nearly all the way at the end.",
	curve.OutExpo:      "OutExpo covers nearly all the way at once, then creeps to the end: for snappy UI.",
	curve.InOutExpo:    "InOutExpo waits at the start, rushes through the middle and creeps to the end.",
	curve.InBack:       "InBack pulls back past the start before it moves, like a wind-up.",
	curve.OutBack:      "OutBack goes past the end and settles back: for a pop or a bounce-in.",
	curve.InOutBack:    "InOutBack pulls back past the start and goes past the end before it settles.",
	curve.InElastic:    "InElastic wobbles around the start with growing swings, then springs to the end.",
	curve.OutElastic:   "OutElastic springs past the end and wobbles around it as it settles, like a spring.",
	curve.InOutElastic: "InOutElastic wobbles around both the start and the end.",
	curve.InBounce:     "InBounce bounces off the start with growing hops before it goes.",
	curve.OutBounce:    "OutBounce reaches the end and bounces on it like a dropped ball.",
	curve.InOutBounce:  "InOutBounce bounces off the start and bounces on the end.",
}
