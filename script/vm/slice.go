package vm

func (m *Machine) SliceData(v Value) ([]Value, int, bool) {
	if v.A == 0 {
		return nil, 0, true
	}
	o := m.heap.get(v.A)
	if o == nil || o.str {
		return nil, 0, false
	}
	n := min(int(v.B)*o.width, len(o.elems))
	return o.elems[:n], o.width, true
}

func (m *Machine) MakeSlice(width int, mask uint32, elems []Value) Value {
	width = max(width, 1)
	n := len(elems) / width
	v := m.heap.newSlice(width, mask, max(n, 1))
	o := m.heap.get(v.A)
	o.elems = append(o.elems, elems[:n*width]...)
	v.B = uint64(n)
	return v
}
