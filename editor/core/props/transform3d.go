package props

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/scene"
)

type Transform3DEdit struct {
	Node   ids.NodeID
	Before scene.Transform3D
	After  scene.Transform3D
}

const (
	T3PosX = iota
	T3PosY
	T3PosZ
	T3RotX
	T3RotY
	T3RotZ
	T3ScaleX
	T3ScaleY
	T3ScaleZ
)

func (a *Access) ApplyTransforms3D(tx *history.Transaction, label string, edits []Transform3DEdit) error {
	return a.within(tx, label, func(tx *history.Transaction) error {
		for _, e := range edits {
			if e.Before == e.After {
				continue
			}
			if err := tx.Exec(&commands.SetTransform3D{Node: e.Node, Before: e.Before, After: e.After}); err != nil {
				return err
			}
		}
		return nil
	})
}

func component(v mathx.Vec3, i int) float32 {
	switch i {
	case 0:
		return v.X
	case 1:
		return v.Y
	}
	return v.Z
}

func setComponent(v *mathx.Vec3, i int, f float32) {
	switch i {
	case 0:
		v.X = f
	case 1:
		v.Y = f
	default:
		v.Z = f
	}
}

func (a *Access) SetTransform3DComponent(tx *history.Transaction, nodes []ids.NodeID, euler *EulerCache, comp int, v float32) error {
	if euler == nil {
		euler = NewEulerCache()
	}
	return a.within(tx, i18n.Tr("editor.history.transform", "Transform"), func(tx *history.Transaction) error {
		for _, n := range nodes {
			h, err := a.handle(n)
			if err != nil {
				return err
			}
			if a.s.Space(h) != scene.Space3D {
				continue
			}
			before := a.s.Local3D(h)
			after := before
			switch {
			case comp <= T3PosZ:
				setComponent(&after.Position, comp, v)
			case comp <= T3RotZ:
				e := euler.Get(n, before.Rotation)
				if component(e, comp-T3RotX) == v {
					continue
				}
				setComponent(&e, comp-T3RotX, v)
				after.Rotation = euler.Set(n, e)
			default:
				setComponent(&after.Scale, comp-T3ScaleX, v)
			}
			if after == before {
				continue
			}
			if err := tx.Exec(&commands.SetTransform3D{Node: n, Before: before, After: after}); err != nil {
				return err
			}
		}
		return nil
	})
}
