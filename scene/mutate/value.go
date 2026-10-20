package mutate

import (
	"math"
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene/sceneio"
	"aego/schema"
)

func ParseValue(text string) (aetext.Value, error) {
	doc, err := aetext.Parse([]byte("v {\n  x="+text+"\n}\n"), "value")
	if err != nil {
		return aetext.Value{}, err
	}
	v, ok := doc.Field("x")
	if !ok {
		return aetext.Value{}, errs.New(errs.SceneIOParse, "not a value", log.F("text", text))
	}
	return v, nil
}

func FormatValue(v aetext.Value) string {
	out := string(aetext.Write(&aetext.Node{Head: "v", Fields: []aetext.Field{{Name: "x", Value: v}}}))
	i := strings.Index(out, "x=")
	if i < 0 {
		return strings.TrimSpace(out)
	}
	s := out[i+2:]
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSpace(s)
}

func plain(k schema.Kind) *schema.FieldDescriptor {
	return &schema.FieldDescriptor{Kind: k, Flags: schema.FlagSerialize}
}

func elemOf(fd *schema.FieldDescriptor) *schema.FieldDescriptor {
	return &schema.FieldDescriptor{Kind: fd.Elem, Struct: fd.Struct, Enum: fd.Enum, EnumName: fd.EnumName, AssetKind: fd.AssetKind, Component: fd.Component, Flags: schema.FlagSerialize}
}

func numeric(v aetext.Value) bool {
	return v.Kind == aetext.VInt || v.Kind == aetext.VFloat
}

func numbers(v aetext.Value, n int) bool {
	if v.Kind != aetext.VList || len(v.List) != n {
		return false
	}
	for _, e := range v.List {
		if !numeric(e) {
			return false
		}
	}
	return true
}

func shape(fd *schema.FieldDescriptor, v aetext.Value) error {
	ok := true
	want := ""
	switch fd.Kind {
	case schema.KindBool:
		ok, want = v.Kind == aetext.VBool, "true or false"
	case schema.KindInt32, schema.KindInt64:
		ok, want = v.Kind == aetext.VInt || v.Kind == aetext.VFloat && v.Float == math.Trunc(v.Float), "a whole number"
	case schema.KindFloat32, schema.KindFloat64:
		ok, want = numeric(v), "a number"
	case schema.KindString:
		ok, want = v.Kind == aetext.VString, "a quoted string"
	case schema.KindVec2:
		ok, want = numbers(v, 2), "[x, y]"
	case schema.KindVec3:
		ok, want = numbers(v, 3), "[x, y, z]"
	case schema.KindVec4, schema.KindQuat:
		ok, want = numbers(v, 4), "[x, y, z, w]"
	case schema.KindRect:
		ok, want = numbers(v, 4), "[x, y, w, h]"
	case schema.KindColor:
		ok, want = v.Kind == aetext.VColor, "a colour like #rrggbbaa"
	case schema.KindEnum:
		ok, want = v.Kind == aetext.VIdent || v.Kind == aetext.VInt, "an enum name"
	case schema.KindAssetRef:
		ok, want = v.Kind == aetext.VAsset || v.Kind == aetext.VString, `asset("<id>") or a project path`
	case schema.KindNodeRef:
		ok, want = v.Kind == aetext.VNode || v.Kind == aetext.VString, `node("<id>")`
	case schema.KindCurve:
		ok, want = v.Kind == aetext.VStruct, "{ ease=… keys=[…] }"
	case schema.KindList:
		ok, want = v.Kind == aetext.VList, "a list [ … ]"
	}
	if !ok {
		return errs.New(errs.EditorProps, fd.Name+" takes "+want, log.F("value", FormatValue(v)))
	}
	if fd.Kind == schema.KindList {
		el := elemOf(fd)
		el.Name = fd.Name
		for _, e := range v.List {
			if err := shape(el, e); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Document) read(fd *schema.FieldDescriptor, v aetext.Value) (schema.Value, error) {
	if err := shape(fd, v); err != nil {
		return schema.Value{}, err
	}
	out, err := sceneio.ReadValue(d.reg, fd, v)
	if err == nil || fd.Kind != schema.KindAssetRef || d.env.Lookup == nil || v.Str == "" {
		return out, err
	}
	id, ok := d.env.Lookup(v.Str)
	if !ok {
		return out, errs.New(errs.AssetNotFound, "no asset at this path", log.F("path", v.Str))
	}
	return sceneio.ReadValue(d.reg, fd, aetext.AssetRef(id.String()))
}

func (d *Document) text(fd *schema.FieldDescriptor, v schema.Value) string {
	return FormatValue(sceneio.WriteValue(d.reg, fd, v))
}

func lane(v *schema.Value, name string) *float32 {
	switch v.Kind {
	case schema.KindVec2:
		switch name {
		case "x":
			return &v.V.X
		case "y":
			return &v.V.Y
		}
	case schema.KindVec3:
		switch name {
		case "x":
			return &v.V3.X
		case "y":
			return &v.V3.Y
		case "z":
			return &v.V3.Z
		}
	case schema.KindVec4:
		switch name {
		case "x":
			return &v.V4.X
		case "y":
			return &v.V4.Y
		case "z":
			return &v.V4.Z
		case "w":
			return &v.V4.W
		}
	case schema.KindQuat:
		switch name {
		case "x":
			return &v.Q.X
		case "y":
			return &v.Q.Y
		case "z":
			return &v.Q.Z
		case "w":
			return &v.Q.W
		}
	case schema.KindColor:
		switch name {
		case "r":
			return &v.C.R
		case "g":
			return &v.C.G
		case "b":
			return &v.C.B
		case "a":
			return &v.C.A
		}
	case schema.KindRect:
		switch name {
		case "x":
			return &v.R.X
		case "y":
			return &v.R.Y
		case "w":
			return &v.R.W
		case "h":
			return &v.R.H
		}
	}
	return nil
}

func setLane(cur schema.Value, name string, v aetext.Value) (schema.Value, error) {
	next := cur
	p := lane(&next, name)
	if p == nil {
		return cur, errs.New(errs.EditorProps, "the value has no part "+name)
	}
	f, err := strconv.ParseFloat(FormatValue(v), 32)
	if err != nil || !numeric(v) || math.IsInf(f, 0) {
		return cur, errs.New(errs.EditorProps, "a part takes a number", log.F("part", name))
	}
	*p = float32(f)
	return next, nil
}

func (d *Document) validate(fd *schema.FieldDescriptor, v schema.Value, what string, live bool) error {
	if fd.Flags.Has(schema.FlagReadOnly) {
		return errs.New(errs.EditorProps, what+" is read-only")
	}
	if !live && !fd.Flags.Has(schema.FlagSerialize) {
		return errs.New(errs.EditorProps, what+" is not saved with the scene")
	}
	return d.check(fd, v, what)
}

func errNoAsset(text string) error {
	return errs.New(errs.AssetNotFound, "no asset with this id or path", log.F("asset", text))
}

func (d *Document) check(fd *schema.FieldDescriptor, v schema.Value, what string) error {
	x, numeric := 0.0, false
	switch v.Kind {
	case schema.KindInt32, schema.KindInt64:
		x, numeric = float64(v.I), true
	case schema.KindFloat32, schema.KindFloat64:
		x, numeric = v.F, true
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return errs.New(errs.EditorProps, what+" must be a finite number")
		}
	case schema.KindAssetRef:
		if !v.Asset.IsZero() && d.env.Assets != nil && !d.env.Assets(v.Asset) {
			return errs.New(errs.AssetNotFound, what+" references an asset that is not in the project", log.F("asset", v.Asset.String()))
		}
	case schema.KindList:
		el := elemOf(fd)
		for i, e := range v.List {
			if err := d.check(el, e, what+"["+strconv.Itoa(i)+"]"); err != nil {
				return err
			}
		}
	}
	if numeric {
		if fd.HasMin && x < fd.Min {
			return errs.New(errs.EditorProps, what+" must be at least "+strconv.FormatFloat(fd.Min, 'g', -1, 64), log.F("value", x))
		}
		if fd.HasMax && x > fd.Max {
			return errs.New(errs.EditorProps, what+" must be at most "+strconv.FormatFloat(fd.Max, 'g', -1, 64), log.F("value", x))
		}
	}
	return nil
}
