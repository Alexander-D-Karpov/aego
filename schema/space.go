package schema

type SpaceKind uint8

const (
	SpaceAny SpaceKind = iota
	Space2D
	Space3D
)

func (k SpaceKind) String() string {
	switch k {
	case Space2D:
		return "2d"
	case Space3D:
		return "3d"
	}
	return "any"
}

func (k SpaceKind) Allows(node SpaceKind) bool {
	return k == SpaceAny || node == SpaceAny || k == node
}
