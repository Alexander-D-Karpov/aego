package schema

import (
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/log"
)

type tagInfo struct {
	skip      bool
	name      string
	former    []string
	flags     FieldFlags
	assetKind string
	min, max  float64
	hasMin    bool
	hasMax    bool
	doc       string
	hints     EditorHints
}

func parseTag(typeName, goName, tag string) (tagInfo, error) {
	info := tagInfo{
		name:  lowerFirst(goName),
		flags: FlagSerialize | FlagEditor | FlagScript,
	}
	if tag == "" {
		return info, nil
	}
	if tag == "-" {
		info.skip = true
		return info, nil
	}
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		info.name = parts[0]
	}
	for _, p := range parts[1:] {
		key, value, hasValue := strings.Cut(p, "=")
		switch key {
		case "":
		case "transient":
			info.flags &^= FlagSerialize
			info.flags |= FlagTransient
		case "readonly":
			info.flags |= FlagReadOnly
		case "noeditor":
			info.flags &^= FlagEditor
		case "noscript":
			info.flags &^= FlagScript
		case "replicated":
			info.flags |= FlagReplicated
		case "asset":
			info.assetKind = value
		case "former":
			info.former = append(info.former, strings.Split(value, "|")...)
		case "doc":
			info.doc = value
		case "display":
			info.hints.DisplayName = value
		case "tip":
			info.hints.Tooltip = value
		case "category":
			info.hints.Category = value
		case "unit":
			info.hints.Unit = value
		case "hidden":
			info.hints.Hidden = true
		case "flags":
			info.hints.Flags = strings.Split(value, "|")
		case "gizmo":
			info.hints.Gizmo = value
		case "gizmospace":
			info.hints.GizmoSpace = value
		case "step":
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return info, tagError(typeName, goName, "option step is not a number")
			}
			info.hints.Step = f
		case "min", "max":
			if !hasValue {
				return info, tagError(typeName, goName, "option "+key+" needs a value")
			}
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return info, tagError(typeName, goName, "option "+key+" is not a number")
			}
			if key == "min" {
				info.min, info.hasMin = f, true
			} else {
				info.max, info.hasMax = f, true
			}
		default:
			return info, tagError(typeName, goName, "unknown tag option "+key)
		}
	}
	if info.hints.Tooltip == "" {
		info.hints.Tooltip = info.doc
	}
	return info, nil
}

func ValidGizmo(k Kind, gizmo, space string) error {
	switch gizmo {
	case "":
		if space != "" {
			return errs.New(errs.SchemaBadField, "gizmo space needs a gizmo")
		}
		return nil
	case GizmoVector, GizmoPoint:
		if k != KindVec2 {
			return errs.New(errs.SchemaBadField, "gizmo "+gizmo+" needs a Vec2 field")
		}
	case GizmoRadius:
		if k != KindFloat32 && k != KindFloat64 {
			return errs.New(errs.SchemaBadField, "gizmo radius needs a float field")
		}
	default:
		return errs.New(errs.SchemaBadField, "unknown gizmo "+gizmo+", use vector, point or radius")
	}
	switch space {
	case "", GizmoLocal, GizmoWorld:
		return nil
	}
	return errs.New(errs.SchemaBadField, "gizmo space must be local or world")
}

func tagError(typeName, field, msg string) error {
	return errs.New(errs.SchemaBadField, msg,
		log.F("type", typeName), log.F("field", field))
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	n := 0
	for n < len(s) && s[n] >= 'A' && s[n] <= 'Z' {
		n++
	}
	if n == 0 {
		return s
	}
	if n > 1 && n < len(s) {
		n--
	}
	return strings.ToLower(s[:n]) + s[n:]
}
