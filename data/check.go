package data

import (
	"strconv"
	"unsafe"

	"aego/core/ids"
	"aego/schema"
)

const (
	CodeRange   = "DAT002"
	CodeMissing = "DAT003"
)

type Issue struct {
	Field   string
	Code    string
	Message string
}

func Validate(td *schema.TypeDescriptor, p unsafe.Pointer, exists func(ids.AssetID) bool) []Issue {
	var out []Issue
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !fd.Flags.Has(schema.FlagSerialize) {
			continue
		}
		var v schema.Value
		fd.Get(p, &v)
		out = checkValue(out, fd, fd.Name, v, exists)
	}
	return out
}

func num(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func checkValue(out []Issue, fd *schema.FieldDescriptor, name string, v schema.Value, exists func(ids.AssetID) bool) []Issue {
	switch v.Kind {
	case schema.KindInt32, schema.KindInt64, schema.KindFloat32, schema.KindFloat64:
		f := v.AsFloat()
		if fd.HasMin && f < fd.Min {
			out = append(out, Issue{Field: name, Code: CodeRange, Message: name + " is " + num(f) + ", below its minimum " + num(fd.Min)})
		}
		if fd.HasMax && f > fd.Max {
			out = append(out, Issue{Field: name, Code: CodeRange, Message: name + " is " + num(f) + ", above its maximum " + num(fd.Max)})
		}
	case schema.KindAssetRef:
		if exists != nil && !v.Asset.IsZero() && !exists(v.Asset) {
			out = append(out, Issue{Field: name, Code: CodeMissing, Message: name + " refers to a missing asset " + v.Asset.Short()})
		}
	case schema.KindList:
		elem := *fd
		elem.Kind = fd.Elem
		for i, e := range v.List {
			out = checkValue(out, &elem, name+"["+strconv.Itoa(i)+"]", e, exists)
		}
	}
	return out
}
