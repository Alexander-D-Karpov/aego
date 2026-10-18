package schema

import (
	"unsafe"

	"aego/core/curve"
	"aego/core/rewind"
)

func HashFields(h *rewind.Hasher, td *TypeDescriptor, p unsafe.Pointer) {
	var v Value
	for i := range td.Fields {
		fd := &td.Fields[i]
		fd.Get(p, &v)
		h.String(fd.Name)
		HashValue(h, v)
	}
}

func HashValue(h *rewind.Hasher, v Value) {
	h.U64(uint64(v.Kind))
	switch v.Kind {
	case KindBool:
		h.I64(v.I)
	case KindInt32, KindInt64, KindEnum:
		h.I64(v.AsInt())
	case KindFloat32, KindFloat64:
		h.F64(v.AsFloat())
	case KindString:
		h.String(v.S)
	case KindVec2:
		h.F32(v.V.X)
		h.F32(v.V.Y)
	case KindVec3:
		h.F32(v.V3.X)
		h.F32(v.V3.Y)
		h.F32(v.V3.Z)
	case KindVec4:
		h.F32(v.V4.X)
		h.F32(v.V4.Y)
		h.F32(v.V4.Z)
		h.F32(v.V4.W)
	case KindQuat:
		h.F32(v.Q.X)
		h.F32(v.Q.Y)
		h.F32(v.Q.Z)
		h.F32(v.Q.W)
	case KindColor:
		h.F32(v.C.R)
		h.F32(v.C.G)
		h.F32(v.C.B)
		h.F32(v.C.A)
	case KindRect:
		h.F32(v.R.X)
		h.F32(v.R.Y)
		h.F32(v.R.W)
		h.F32(v.R.H)
	case KindCurve:
		hashCurve(h, &v.Cv)
	case KindAssetRef:
		h.String(v.Asset.String())
	case KindNodeRef:
		h.U64(v.Node.Hi)
		h.U64(v.Node.Lo)
	case KindList:
		h.Int(len(v.List))
		for _, e := range v.List {
			HashValue(h, e)
		}
	case KindStruct:
		if v.Obj == nil {
			h.Int(-1)
			return
		}
		h.Int(len(v.Obj.Fields))
		for _, f := range v.Obj.Fields {
			HashValue(h, f)
		}
	}
}

func hashCurve(h *rewind.Hasher, c *curve.Curve) {
	h.U64(uint64(c.Ease))
	h.U64(uint64(c.Pre))
	h.U64(uint64(c.Post))
	h.Int(len(c.Keys))
	for i := range c.Keys {
		k := &c.Keys[i]
		h.F32(k.T)
		h.F32(k.V)
		h.F32(k.In)
		h.F32(k.Out)
		h.U64(uint64(k.Interp))
		h.U64(uint64(k.Tangent))
	}
}
