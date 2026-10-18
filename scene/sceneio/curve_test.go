package sceneio

import (
	"testing"

	"aego/core/curve"
	"aego/format/aetext"
)

func TestCurveRoundTrip(t *testing.T) {
	preset := curve.FromEasing(curve.OutBack)
	preset.Post = curve.WrapPingPong
	keyed := curve.Keyed(
		curve.Key{T: 0, V: 0},
		curve.Key{T: 0.5, V: 2, Interp: curve.InterpStep},
		curve.Key{T: 1, V: 1, Tangent: curve.TangentFree, In: 3, Out: -1},
	)
	keyed.Pre = curve.WrapLoop
	for _, c := range []curve.Curve{preset, keyed, curve.Line(0, 0, 2, 5)} {
		n := &aetext.Node{Head: "t", Fields: []aetext.Field{{Name: "c", Value: CurveValue(c)}}}
		doc, err := aetext.Parse(aetext.Write(n), "t")
		if err != nil {
			t.Fatal(err)
		}
		v, _ := doc.Field("c")
		got, err := ReadCurve(v)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(c) {
			t.Fatalf("round trip changed the curve:\n%+v\n%+v", c, got)
		}
	}
}

func TestCurveRejectsGarbage(t *testing.T) {
	if _, err := ReadCurve(aetext.Int(1)); err == nil {
		t.Fatal("a number is not a curve")
	}
	bad := aetext.Struct(aetext.Field{Name: "ease", Value: aetext.Ident("nope")})
	if _, err := ReadCurve(bad); err == nil {
		t.Fatal("unknown easing accepted")
	}
	short := aetext.Struct(aetext.Field{Name: "keys", Value: aetext.List(aetext.List(aetext.Int(1)))})
	if _, err := ReadCurve(short); err == nil {
		t.Fatal("a key needs time and value")
	}
}
