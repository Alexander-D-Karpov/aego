package tween

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/scene"
	"aego/scene/propref"
)

const ownerTransform = propref.OwnerTransform

type target = propref.Ref

type shape = propref.Shape

const (
	shFloat = propref.Float
	shInt   = propref.Int
	shVec2  = propref.Vec2
	shVec3  = propref.Vec3
	shVec4  = propref.Vec4
	shQuat  = propref.Quat
)

func fits(t *target, want shape) bool {
	return t.Fits(want)
}

func badPath(path, msg string) error {
	return errs.New(errs.EngineModule, "tween: "+path+": "+msg, log.F("path", path))
}

func (s *Service) resolve(h scene.NodeHandle, path string) (target, error) {
	t, err := s.props.Resolve(h, path)
	if pe, ok := err.(*propref.PathError); ok {
		return t, badPath(path, pe.Reason)
	}
	return t, err
}

func (s *Service) read(t *target, h scene.NodeHandle) ([4]float32, bool) {
	return s.props.Read(t, h)
}

func (s *Service) write(t *target, h scene.NodeHandle, l [4]float32) bool {
	return s.props.Write(t, h, l)
}
