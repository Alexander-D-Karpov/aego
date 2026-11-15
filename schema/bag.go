package schema

import "slices"

type Property struct {
	Name  string
	Value Value
}

type PropertyBag struct {
	Props []Property
}

func (b *PropertyBag) Get(name string) (Value, bool) {
	for i := range b.Props {
		if b.Props[i].Name == name {
			return b.Props[i].Value, true
		}
	}
	return Value{}, false
}

func (b *PropertyBag) Set(name string, v Value) {
	for i := range b.Props {
		if b.Props[i].Name == name {
			b.Props[i].Value = v
			return
		}
	}
	b.Props = append(b.Props, Property{Name: name, Value: v})
}

func (b *PropertyBag) Delete(name string) bool {
	for i := range b.Props {
		if b.Props[i].Name == name {
			b.Props = append(b.Props[:i], b.Props[i+1:]...)
			return true
		}
	}
	return false
}

// Clone detaches nested lists, structs and curves from the original value.
func (v Value) Clone() Value {
	v.cloneReferences()
	return v
}

func (b PropertyBag) Clone() PropertyBag {
	out := PropertyBag{Props: slices.Clone(b.Props)}
	for i := range out.Props {
		out.Props[i].Value.cloneReferences()
	}
	return out
}

// The outer values have already been copied. Only nested mutable payloads
// need new storage. Scalars, strings and asset/node IDs are values.
func (v *Value) cloneReferences() {
	if v.List != nil {
		v.List = cloneValues(v.List)
	}
	if v.Obj != nil {
		v.Obj = &Object{Type: v.Obj.Type, Fields: cloneValues(v.Obj.Fields)}
	}
	if v.Cv.Keys != nil {
		v.Cv = v.Cv.Clone()
	}
}

func cloneValues(values []Value) []Value {
	out := slices.Clone(values)
	for i := range out {
		out[i].cloneReferences()
	}
	return out
}
