package sceneio

import (
	"aego/core/curve"
	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
)

func CurveValue(c curve.Curve) aetext.Value {
	var fs []aetext.Field
	if c.Ease != curve.None {
		fs = append(fs, aetext.Field{Name: "ease", Value: aetext.Ident(c.Ease.String())})
	}
	if c.Pre != curve.WrapClamp {
		fs = append(fs, aetext.Field{Name: "pre", Value: aetext.Ident(c.Pre.String())})
	}
	if c.Post != curve.WrapClamp {
		fs = append(fs, aetext.Field{Name: "post", Value: aetext.Ident(c.Post.String())})
	}
	if len(c.Keys) > 0 {
		keys := make([]aetext.Value, len(c.Keys))
		for i, k := range c.Keys {
			keys[i] = aetext.List(
				aetext.Float32(k.T), aetext.Float32(k.V), aetext.Float32(k.In), aetext.Float32(k.Out),
				aetext.Ident(k.Interp.String()), aetext.Ident(k.Tangent.String()),
			)
		}
		fs = append(fs, aetext.Field{Name: "keys", Value: aetext.List(keys...)})
	}
	return aetext.Struct(fs...)
}

func ReadCurve(v aetext.Value) (curve.Curve, error) {
	var c curve.Curve
	if v.Kind != aetext.VStruct {
		return c, errs.New(errs.SceneIOParse, "a curve is written as { ease=… keys=[…] }")
	}
	for _, f := range v.Obj {
		ok := false
		switch f.Name {
		case "ease":
			c.Ease, ok = curve.ParseEasing(f.Value.Ident)
		case "pre":
			c.Pre, ok = curve.ParseWrap(f.Value.Ident)
		case "post":
			c.Post, ok = curve.ParseWrap(f.Value.Ident)
		case "keys":
			ok = f.Value.Kind == aetext.VList
			for _, kv := range f.Value.List {
				k, kok := readKey(kv)
				if !kok {
					ok = false
					break
				}
				c.Keys = append(c.Keys, k)
			}
		}
		if !ok {
			return curve.Curve{}, errs.New(errs.SceneIOParse, "curve field "+f.Name+" is malformed", log.F("field", f.Name))
		}
	}
	c.Fix()
	return c, nil
}

func readKey(v aetext.Value) (curve.Key, bool) {
	l := v.List
	if v.Kind != aetext.VList || len(l) < 2 || len(l) > 6 {
		return curve.Key{}, false
	}
	k := curve.Key{T: float32(l[0].AsFloat()), V: float32(l[1].AsFloat())}
	if len(l) > 2 {
		k.In = float32(l[2].AsFloat())
	}
	if len(l) > 3 {
		k.Out = float32(l[3].AsFloat())
	}
	ok := true
	if len(l) > 4 {
		k.Interp, ok = curve.ParseInterp(l[4].Ident)
	}
	if ok && len(l) > 5 {
		k.Tangent, ok = curve.ParseTangent(l[5].Ident)
	}
	return k, ok
}
