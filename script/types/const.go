package types

import (
	"math"
	"strconv"
)

type ConstKind uint8

const (
	CInt ConstKind = iota
	CFloat
	CBool
	CString
)

type Const struct {
	Kind  ConstKind
	Int   int64
	Float float64
	Bool  bool
	Str   string
}

func IntConst(v int64) *Const {
	return &Const{Kind: CInt, Int: v}
}

func FloatConst(v float64) *Const {
	return &Const{Kind: CFloat, Float: v}
}

func BoolConst(v bool) *Const {
	return &Const{Kind: CBool, Bool: v}
}

func StringConst(v string) *Const {
	return &Const{Kind: CString, Str: v}
}

func (c *Const) Float64() float64 {
	if c.Kind == CInt {
		return float64(c.Int)
	}
	return c.Float
}

func (c *Const) String() string {
	switch c.Kind {
	case CInt:
		return strconv.FormatInt(c.Int, 10)
	case CFloat:
		return strconv.FormatFloat(c.Float, 'g', -1, 64)
	case CBool:
		return strconv.FormatBool(c.Bool)
	}
	return strconv.Quote(c.Str)
}

func (c *Const) Key() string {
	return strconv.Itoa(int(c.Kind)) + ":" + c.String()
}

func Represent(c *Const, t *Type) (*Const, string) {
	switch t.Kind {
	case KInt, KUntypedInt:
		switch c.Kind {
		case CInt:
			return c, ""
		case CFloat:
			if c.Float != math.Trunc(c.Float) {
				return nil, "would be truncated"
			}
			if c.Float < -9.223372036854776e18 || c.Float >= 9.223372036854776e18 {
				return nil, "overflows int"
			}
			return IntConst(int64(c.Float)), ""
		}
	case KFloat:
		if c.Kind == CInt || c.Kind == CFloat {
			v := c.Float64()
			if math.Abs(v) > math.MaxFloat32 {
				return nil, "overflows float"
			}
			return FloatConst(float64(float32(v))), ""
		}
	case KUntypedFloat:
		if c.Kind == CInt || c.Kind == CFloat {
			return FloatConst(c.Float64()), ""
		}
	case KBool:
		if c.Kind == CBool {
			return c, ""
		}
	case KString:
		if c.Kind == CString {
			return c, ""
		}
	}
	return nil, "mismatched types"
}
