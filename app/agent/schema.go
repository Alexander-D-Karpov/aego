package agent

import (
	"errors"
	"strings"

	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

type FieldView struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Elem     string   `json:"elem,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	Asset    string   `json:"asset,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	Default  string   `json:"default,omitempty"`
	ReadOnly bool     `json:"readOnly,omitempty"`
	Saved    bool     `json:"saved"`
	Live     bool     `json:"live,omitempty"`
	Doc      string   `json:"doc,omitempty"`
}

type TypeView struct {
	Name     string      `json:"name"`
	Kind     string      `json:"kind"`
	Display  string      `json:"display,omitempty"`
	Category string      `json:"category,omitempty"`
	Owner    string      `json:"owner,omitempty"`
	Requires []string    `json:"requires,omitempty"`
	Doc      string      `json:"doc,omitempty"`
	Fields   []FieldView `json:"fields,omitempty"`
}

func schemaCommands() []*Command {
	return []*Command{
		{Name: "schema.list", Doc: "component and data types", Args: []Arg{{Name: "kind", Type: "string", Doc: "component, data or all (default)"}}, Run: (*Session).schemaList},
		{Name: "schema.show", Doc: "fields of a type: kind, range, unit, default, flags and docs", Args: []Arg{{Name: "type", Type: "string", Doc: "full name or a unique suffix", Required: true, Pos: true}}, Run: (*Session).schemaShow},
	}
}

func (s *Session) types(kind string) []*schema.TypeDescriptor {
	reg := s.Types.Schema()
	var out []*schema.TypeDescriptor
	if kind == "" || kind == "all" || kind == "component" {
		out = append(out, reg.Components()...)
	}
	if kind == "" || kind == "all" || kind == "data" {
		out = append(out, reg.DataTypes()...)
	}
	return out
}

func kindName(td *schema.TypeDescriptor) string {
	if td.Kind == schema.TypeAsset {
		return "data"
	}
	return "component"
}

func summary(doc string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(doc), "\n")
	return line
}

func (s *Session) schemaList(a Args) (*Output, error) {
	var list []TypeView
	var b strings.Builder
	for _, td := range s.types(a.Str("kind")) {
		if td.Hidden {
			continue
		}
		list = append(list, TypeView{Name: td.FullName, Kind: kindName(td), Display: td.DisplayName, Category: td.Category, Owner: td.Owner, Doc: summary(td.Doc)})
		b.WriteString(td.FullName + "  " + kindName(td) + "  " + summary(td.Doc) + "\n")
	}
	return &Output{Value: list, Text: b.String()}, nil
}

func (s *Session) lookupType(name string) (*schema.TypeDescriptor, error) {
	if td, ok := s.Types.Schema().ByName(name); ok {
		return td, nil
	}
	var found []*schema.TypeDescriptor
	for _, td := range s.types("") {
		if strings.HasSuffix(td.FullName, "."+name) || strings.EqualFold(td.DisplayName, name) {
			found = append(found, td)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, errors.New("unknown type " + name)
	}
	var names []string
	for _, td := range found {
		names = append(names, td.FullName)
	}
	return nil, errors.New("ambiguous type " + name + ": " + strings.Join(names, ", "))
}

func defaultText(reg *schema.Registry, td *schema.TypeDescriptor, fd *schema.FieldDescriptor) string {
	var v schema.Value
	switch {
	case td.Default != nil:
		fd.Get(td.Default(), &v)
	case td.New != nil:
		fd.Get(td.New(), &v)
	case fd.HasDefault:
		v = fd.Default
	default:
		return ""
	}
	return mutate.FormatValue(sceneio.WriteValue(reg, fd, v))
}

func (s *Session) schemaShow(a Args) (*Output, error) {
	td, err := s.lookupType(a.Str("type"))
	if err != nil {
		return nil, err
	}
	reg := s.Types.Schema()
	v := TypeView{Name: td.FullName, Kind: kindName(td), Display: td.DisplayName, Category: td.Category, Owner: td.Owner, Requires: td.Requires, Doc: td.Doc}
	var b strings.Builder
	b.WriteString(td.FullName + " (" + kindName(td) + ")\n")
	if d := summary(td.Doc); d != "" {
		b.WriteString(d + "\n")
	}
	for i := range td.Fields {
		fd := &td.Fields[i]
		if fd.Hints.Hidden {
			continue
		}
		f := FieldView{Name: fd.Name, Kind: fd.Kind.String(), Asset: fd.AssetKind, Unit: fd.Hints.Unit, Doc: summary(fd.Doc),
			ReadOnly: fd.Flags.Has(schema.FlagReadOnly), Saved: fd.Flags.Has(schema.FlagSerialize), Live: fd.Flags.Has(schema.FlagLive),
			Default: defaultText(reg, td, fd)}
		if fd.Kind == schema.KindList {
			f.Elem = fd.Elem.String()
		}
		for _, e := range fd.Enum {
			f.Enum = append(f.Enum, e.Name)
		}
		if fd.HasMin {
			m := fd.Min
			f.Min = &m
		}
		if fd.HasMax {
			m := fd.Max
			f.Max = &m
		}
		v.Fields = append(v.Fields, f)
		b.WriteString("  " + f.Name + "  " + f.Kind)
		if f.Default != "" {
			b.WriteString(" = " + f.Default)
		}
		if f.Doc != "" {
			b.WriteString("  — " + f.Doc)
		}
		b.WriteString("\n")
	}
	return &Output{Value: v, Text: b.String()}, nil
}
