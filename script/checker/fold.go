package checker

import (
	"math"

	"aego/script/token"
	"aego/script/types"
)

const (
	whyOverflow = "overflows"
	whyDivZero  = "division by zero"
	whyInvalid  = "is not a valid constant operation"
)

func foldUnary(op token.Kind, x *types.Const) (*types.Const, string) {
	switch {
	case op == token.Sub && x.Kind == types.CInt:
		if x.Int == math.MinInt64 {
			return nil, whyOverflow
		}
		return types.IntConst(-x.Int), ""
	case op == token.Sub && x.Kind == types.CFloat:
		return types.FloatConst(-x.Float), ""
	case op == token.Not && x.Kind == types.CBool:
		return types.BoolConst(!x.Bool), ""
	}
	return nil, whyInvalid
}

func compare(op token.Kind, c int) *types.Const {
	switch op {
	case token.Eq:
		return types.BoolConst(c == 0)
	case token.Ne:
		return types.BoolConst(c != 0)
	case token.Lt:
		return types.BoolConst(c < 0)
	case token.Le:
		return types.BoolConst(c <= 0)
	case token.Gt:
		return types.BoolConst(c > 0)
	case token.Ge:
		return types.BoolConst(c >= 0)
	}
	return nil
}

func cmp[T int64 | float64 | string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func numeric(c *types.Const) bool {
	return c.Kind == types.CInt || c.Kind == types.CFloat
}

func foldBinary(op token.Kind, a, b *types.Const, single bool) (*types.Const, string) {
	switch {
	case a.Kind == types.CBool && b.Kind == types.CBool:
		switch op {
		case token.AndAnd:
			return types.BoolConst(a.Bool && b.Bool), ""
		case token.OrOr:
			return types.BoolConst(a.Bool || b.Bool), ""
		case token.Eq:
			return types.BoolConst(a.Bool == b.Bool), ""
		case token.Ne:
			return types.BoolConst(a.Bool != b.Bool), ""
		}
	case a.Kind == types.CString && b.Kind == types.CString:
		if op == token.Add {
			return types.StringConst(a.Str + b.Str), ""
		}
		if r := compare(op, cmp(a.Str, b.Str)); r != nil {
			return r, ""
		}
	case a.Kind == types.CInt && b.Kind == types.CInt:
		return foldInt(op, a.Int, b.Int)
	case numeric(a) && numeric(b):
		return foldFloat(op, a.Float64(), b.Float64(), single)
	}
	return nil, whyInvalid
}

func foldInt(op token.Kind, x, y int64) (*types.Const, string) {
	if r := compare(op, cmp(x, y)); r != nil {
		return r, ""
	}
	switch op {
	case token.Add:
		r := x + y
		if (x > 0 && y > 0 && r < 0) || (x < 0 && y < 0 && r >= 0) {
			return nil, whyOverflow
		}
		return types.IntConst(r), ""
	case token.Sub:
		r := x - y
		if (x >= 0 && y < 0 && r < 0) || (x < 0 && y > 0 && r >= 0) {
			return nil, whyOverflow
		}
		return types.IntConst(r), ""
	case token.Mul:
		if x == 0 || y == 0 {
			return types.IntConst(0), ""
		}
		r := x * y
		if r/y != x || (x == -1 && y == math.MinInt64) || (y == -1 && x == math.MinInt64) {
			return nil, whyOverflow
		}
		return types.IntConst(r), ""
	case token.Quo:
		if y == 0 {
			return nil, whyDivZero
		}
		if x == math.MinInt64 && y == -1 {
			return nil, whyOverflow
		}
		return types.IntConst(x / y), ""
	case token.Rem:
		if y == 0 {
			return nil, whyDivZero
		}
		if y == -1 {
			return types.IntConst(0), ""
		}
		return types.IntConst(x % y), ""
	}
	return nil, whyInvalid
}

func foldFloat(op token.Kind, x, y float64, single bool) (*types.Const, string) {
	if r := compare(op, cmp(x, y)); r != nil {
		return r, ""
	}
	var r float64
	switch op {
	case token.Add:
		r = x + y
	case token.Sub:
		r = x - y
	case token.Mul:
		r = x * y
	case token.Quo:
		if y == 0 {
			return nil, whyDivZero
		}
		r = x / y
	default:
		return nil, whyInvalid
	}
	if math.IsInf(r, 0) || (single && math.Abs(r) > math.MaxFloat32) {
		return nil, whyOverflow
	}
	if single {
		r = float64(float32(r))
	}
	return types.FloatConst(r), ""
}
