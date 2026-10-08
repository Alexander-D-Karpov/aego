package scene

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/core/strs"
)

type Transform2D struct {
	Position mathx.Vec2
	Rotation float32
	Scale    mathx.Vec2
}

func IdentityTransform() Transform2D {
	return Transform2D{Scale: mathx.Vec2{X: 1, Y: 1}}
}

func (t Transform2D) Matrix() mathx.Mat3 {
	s := t.Scale
	if s.X == 0 && s.Y == 0 {
		s = mathx.Vec2{X: 1, Y: 1}
	}
	return mathx.Mat3TRS(t.Position, t.Rotation, s)
}

const (
	flagAlive uint8 = 1 << iota
	flagEnabled
	flagEffective
	flagWorldDirty
	flagPendingDestroy
	flagUnder3D
)

type nodeData struct {
	id         ids.NodeID
	name       strs.StringID
	parent     NodeHandle
	firstChild NodeHandle
	lastChild  NodeHandle
	nextSib    NodeHandle
	prevSib    NodeHandle
	local      Transform2D
	world      mathx.Mat3
	layers     uint64
	flags      uint8
	space      Space
	childCount int32
	slot       int32
}

func (n *nodeData) has(f uint8) bool {
	return n.flags&f != 0
}

func (n *nodeData) set(f uint8, v bool) {
	if v {
		n.flags |= f
		return
	}
	n.flags &^= f
}
