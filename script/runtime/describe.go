package runtime

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"aego/schema"
	"aego/script/abi"
	"aego/script/bytecode"
)

func fieldKind(t abi.TypeRef) (schema.Kind, bool) {
	switch t.Kind {
	case abi.Bool:
		return schema.KindBool, true
	case abi.Int:
		return schema.KindInt64, true
	case abi.Float:
		return schema.KindFloat32, true
	case abi.String:
		return schema.KindString, true
	case abi.Vec2:
		return schema.KindVec2, true
	case abi.Vec3:
		return schema.KindVec3, true
	case abi.Color:
		return schema.KindColor, true
	case abi.Quat:
		return schema.KindQuat, true
	case abi.Rect:
		return schema.KindRect, true
	case abi.Enum:
		return schema.KindEnum, true
	case abi.NodeRef:
		return schema.KindNodeRef, true
	case abi.AssetRef:
		return schema.KindAssetRef, true
	}
	return schema.KindInvalid, false
}

func defaultValue(k schema.Kind, c bytecode.Const) schema.Value {
	switch k {
	case schema.KindBool:
		return schema.Bool(c.A != 0)
	case schema.KindInt64:
		return schema.Int64(int64(c.A))
	case schema.KindEnum:
		return schema.Enum(int64(c.A))
	case schema.KindFloat32:
		return schema.Float32(math.Float32frombits(uint32(c.A)))
	case schema.KindString:
		return schema.String(c.Str)
	}
	return schema.Value{Kind: k}
}

func enumValues(name string, local map[string][]schema.EnumValue, s abi.Surface) []schema.EnumValue {
	if v, ok := local[name]; ok {
		return v
	}
	i := strings.LastIndexByte(name, '.')
	if s == nil || i <= 0 {
		return nil
	}
	e, ok := s.Enum(name[:i], name[i+1:])
	if !ok {
		return nil
	}
	out := make([]schema.EnumValue, len(e.Members))
	for j, m := range e.Members {
		out[j] = schema.EnumValue{Name: m.Name, Value: m.Value}
	}
	return out
}

func Describe(mods []*bytecode.Module, s abi.Surface) ([]*schema.TypeDescriptor, error) {
	local := map[string][]schema.EnumValue{}
	for _, m := range mods {
		for _, e := range m.Enums {
			vals := make([]schema.EnumValue, len(e.Members))
			for i, name := range e.Members {
				vals[i] = schema.EnumValue{Name: name, Value: int64(i)}
			}
			local[e.Name] = vals
		}
	}
	var out []*schema.TypeDescriptor
	for _, m := range mods {
		for i := range m.Scripts {
			def := &m.Scripts[i]
			var fields []schema.FieldDescriptor
			for _, f := range def.Fields {
				if !f.Export {
					continue
				}
				t, ok := abi.Parse(f.Type)
				kind, ok2 := fieldKind(t)
				if !ok || !ok2 {
					return nil, fmt.Errorf("script %s: field %s has type %s that cannot be exported", def.FullName, f.Name, f.Type)
				}
				h := f.Hints
				fd := schema.FieldDescriptor{
					Name:   f.Name,
					Former: f.Former,
					Kind:   kind,
					Flags:  schema.FlagSerialize | schema.FlagEditor | schema.FlagScript,
					Min:    h.Min, Max: h.Max, HasMin: h.HasMin, HasMax: h.HasMax,
					Hints: schema.EditorHints{
						DisplayName: h.Display, Tooltip: h.Tip, Category: h.Category, Step: h.Step, Unit: h.Unit,
					},
				}
				switch kind {
				case schema.KindAssetRef:
					fd.AssetKind = t.Name
				case schema.KindEnum:
					fd.EnumName = t.Name
					fd.Enum = enumValues(t.Name, local, s)
				}
				if f.HasDefault {
					fd.Default, fd.HasDefault = defaultValue(kind, f.Default), true
				}
				fields = append(fields, fd)
			}
			td, err := schema.NewScriptType(def.FullName, def.Former, fields)
			if err != nil {
				return nil, err
			}
			td.Hidden = def.Hidden
			out = append(out, td)
		}
	}
	return out, nil
}

func Publish(reg *schema.Registry, mods []*bytecode.Module, s abi.Surface) error {
	types, err := Describe(mods, s)
	if err != nil {
		return err
	}
	return reg.SetDynamic(types)
}

func DecodeAll(blobs map[string][]byte) ([]*bytecode.Module, error) {
	names := make([]string, 0, len(blobs))
	for n := range blobs {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*bytecode.Module, 0, len(names))
	for _, n := range names {
		m, err := bytecode.Decode(blobs[n])
		if err != nil {
			return nil, fmt.Errorf("script package %s: %w", n, err)
		}
		out = append(out, m)
	}
	return out, nil
}
