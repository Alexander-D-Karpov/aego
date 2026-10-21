package camera2d

import (
	"aego/core/mathx"
	"aego/render/view"
	"aego/scene"
)

const (
	ServiceID         = "camera2d.adjust"
	AccessibilityKind = "camera2d.accessibility"
)

type Adjuster interface {
	Adjust(node scene.NodeHandle, v view.View) view.View
}

type Viewer interface {
	ViewCenter() (mathx.Vec2, bool)
}

type Accessibility struct {
	ScreenShake   string
	ReducedMotion string
}
