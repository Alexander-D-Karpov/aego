package schema

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

func (b PropertyBag) Clone() PropertyBag {
	return PropertyBag{Props: append([]Property(nil), b.Props...)}
}
