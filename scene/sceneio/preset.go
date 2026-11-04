package sceneio

import (
	"aego/format/aetext"
	"aego/schema"
	"fmt"
	"math"
)

// Presets are reusable typed values. Unlike legacy scene loading, accepting a
// malformed literal as zero would permanently change the next applied effect.
func ValidatePresetFields(reg *schema.Registry, td *schema.TypeDescriptor, component *aetext.Node) error {
	if len(component.Blocks) != 0 {
		return fmt.Errorf("unexpected child block in preset component %s", td.FullName)
	}
	return presetFields(reg, td, component.Fields)
}

func presetFields(reg *schema.Registry, td *schema.TypeDescriptor, fields []aetext.Field) error {
	seen := make(map[schema.FieldID]bool, len(fields))
	for _, field := range fields {
		fd, ok := td.Field(field.Name)
		if !ok {
			return fmt.Errorf("unknown preset field %s.%s", td.FullName, field.Name)
		}
		if seen[fd.ID] {
			return fmt.Errorf("duplicate preset field %s.%s", td.FullName, field.Name)
		}
		seen[fd.ID] = true
		if !presetValue(reg, fd, field.Value) {
			return fmt.Errorf("invalid %s literal for %s.%s", fd.Kind, td.FullName, fd.Name)
		}
	}
	return nil
}

func presetNumber(v aetext.Value) bool {
	return v.Kind == aetext.VInt || v.Kind == aetext.VFloat && !math.IsNaN(v.Float) && !math.IsInf(v.Float, 0)
}
func presetVector(v aetext.Value, n int) bool {
	if v.Kind != aetext.VList || len(v.List) != n {
		return false
	}
	for _, x := range v.List {
		if !presetNumber(x) || math.Abs(x.AsFloat()) > math.MaxFloat32 {
			return false
		}
	}
	return true
}
func presetValue(reg *schema.Registry, fd *schema.FieldDescriptor, v aetext.Value) bool {
	switch fd.Kind {
	case schema.KindBool:
		return v.Kind == aetext.VBool
	case schema.KindInt32, schema.KindInt64:
		if v.Kind != aetext.VInt && (v.Kind != aetext.VFloat || v.Float != math.Trunc(v.Float) || v.Float < -0x1p63 || v.Float >= 0x1p63) {
			return false
		}
		return fd.Kind == schema.KindInt64 || v.AsInt() >= math.MinInt32 && v.AsInt() <= math.MaxInt32
	case schema.KindFloat32:
		return presetNumber(v) && math.Abs(v.AsFloat()) <= math.MaxFloat32
	case schema.KindFloat64:
		return presetNumber(v)
	case schema.KindVec2:
		return presetVector(v, 2)
	case schema.KindVec3:
		return presetVector(v, 3)
	case schema.KindVec4, schema.KindRect, schema.KindQuat:
		return presetVector(v, 4)
	case schema.KindColor:
		return v.Kind == aetext.VColor
	case schema.KindString:
		return v.Kind == aetext.VString
	case schema.KindEnum:
		if v.Kind == aetext.VIdent {
			_, ok := fd.EnumValueOf(v.Ident)
			return ok
		}
		_, ok := fd.EnumNameOf(v.Int)
		return v.Kind == aetext.VInt && ok
	case schema.KindAssetRef:
		return v.Kind == aetext.VAsset || v.Kind == aetext.VString
	case schema.KindNodeRef:
		return v.Kind == aetext.VNode || v.Kind == aetext.VString
	case schema.KindList:
		if v.Kind != aetext.VList {
			return false
		}
		el := *fd
		el.Kind = fd.Elem
		for _, x := range v.List {
			if !presetValue(reg, &el, x) {
				return false
			}
		}
		return true
	case schema.KindStruct:
		td, ok := reg.ByID(fd.Struct)
		return v.Kind == aetext.VStruct && ok && presetFields(reg, td, v.Obj) == nil
	case schema.KindCurve:
		if v.Kind != aetext.VStruct {
			return false
		}
		for _, field := range v.Obj {
			if field.Name == "keys" {
				for _, k := range field.Value.List {
					for _, x := range k.List[:min(4, len(k.List))] {
						if !presetNumber(x) || math.Abs(x.AsFloat()) > math.MaxFloat32 {
							return false
						}
					}
				}
			}
		}
		_, err := ReadCurve(v)
		return err == nil
	}
	return false
}
