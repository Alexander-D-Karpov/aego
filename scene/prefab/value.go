package prefab

import (
	"slices"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/schema"
)

func ColorKey(c mathx.Color) uint32 {
	enc := func(f float32) uint32 {
		return uint32(mathx.Clamp(mathx.LinearToSRGB(mathx.Clamp(f, 0, 1)), 0, 1)*255 + 0.5)
	}
	return enc(c.R)<<24 | enc(c.G)<<16 | enc(c.B)<<8 | uint32(mathx.Clamp(c.A, 0, 1)*255+0.5)
}

func Equal(a, b schema.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case schema.KindBool, schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return a.I == b.I
	case schema.KindFloat32:
		return float32(a.F) == float32(b.F)
	case schema.KindFloat64:
		return a.F == b.F
	case schema.KindVec2:
		return a.V == b.V
	case schema.KindVec3:
		return a.V3 == b.V3
	case schema.KindVec4:
		return a.V4 == b.V4
	case schema.KindQuat:
		return a.Q == b.Q
	case schema.KindRect:
		return a.R == b.R
	case schema.KindColor:
		return ColorKey(a.C) == ColorKey(b.C)
	case schema.KindString:
		return a.S == b.S
	case schema.KindAssetRef:
		return a.Asset == b.Asset
	case schema.KindNodeRef:
		return a.Node == b.Node
	case schema.KindList:
		return slices.EqualFunc(a.List, b.List, Equal)
	case schema.KindStruct:
		if a.Obj == nil || b.Obj == nil {
			return a.Obj == b.Obj
		}
		return a.Obj.Type == b.Obj.Type && slices.EqualFunc(a.Obj.Fields, b.Obj.Fields, Equal)
	}
	return true
}

func MapValue(v schema.Value, m map[ids.NodeID]ids.NodeID) (schema.Value, bool) {
	switch v.Kind {
	case schema.KindNodeRef:
		if to, ok := m[v.Node]; ok && to != v.Node {
			v.Node = to
			return v, true
		}
	case schema.KindList:
		var out []schema.Value
		for i, e := range v.List {
			if ne, changed := MapValue(e, m); changed {
				if out == nil {
					out = slices.Clone(v.List)
				}
				out[i] = ne
			}
		}
		if out != nil {
			v.List = out
			return v, true
		}
	case schema.KindStruct:
		if v.Obj == nil {
			return v, false
		}
		var out []schema.Value
		for i, e := range v.Obj.Fields {
			if ne, changed := MapValue(e, m); changed {
				if out == nil {
					out = slices.Clone(v.Obj.Fields)
				}
				out[i] = ne
			}
		}
		if out != nil {
			v.Obj = &schema.Object{Type: v.Obj.Type, Fields: out}
			return v, true
		}
	}
	return v, false
}

func (n *Node) MapRefs(m map[ids.NodeID]ids.NodeID) {
	n.Walk(func(c, _ *Node) {
		for i := range c.Comps {
			fields := c.Comps[i].Fields
			for j := range fields {
				if v, changed := MapValue(fields[j].Value, m); changed {
					fields[j].Value = v
				}
			}
		}
	})
}
