package document

import "aego/core/mathx"

type ViewMode uint8

const (
	ViewAuto ViewMode = iota
	View2D
	View3D
)

type Orbit struct {
	Target   mathx.Vec3
	Distance float32
	Yaw      float32
	Pitch    float32
	FOV      float32
}
