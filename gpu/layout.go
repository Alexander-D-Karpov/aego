package gpu

import "fmt"

// ValidateVertexLayout checks the complete layout, including engine attributes
// and material lanes, before any backend creates a VAO.
func ValidateVertexLayout(l VertexLayout, caps Capabilities) error {
	limit := caps.MaxVertexAttributes
	if limit <= 0 {
		limit = 16
	} // minimum of the supported GL33/WebGL2 profiles
	if len(l.Attrs) > limit {
		return fmt.Errorf("SHD004: %d vertex attributes exceed device limit %d", len(l.Attrs), limit)
	}
	for i, a := range l.Attrs {
		if int(a.Location) >= limit {
			return fmt.Errorf("SHD004: vertex attribute location %d exceeds device limit %d", a.Location, limit)
		}
		if int(a.Slot) >= MaxVertexSlots {
			return fmt.Errorf("vertex buffer slot %d is out of range", a.Slot)
		}
		if a.Format > VFUInt4 {
			return fmt.Errorf("unknown vertex attribute format %d", a.Format)
		}
		if l.Strides[a.Slot] == 0 || int(a.Offset)+a.Format.Size() > int(l.Strides[a.Slot]) {
			return fmt.Errorf("vertex attribute %d exceeds its stride", a.Location)
		}
		for j := 0; j < i; j++ {
			if l.Attrs[j].Location == a.Location {
				return fmt.Errorf("duplicate vertex attribute location %d", a.Location)
			}
		}
	}
	return nil
}
