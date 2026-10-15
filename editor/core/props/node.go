package props

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/scene"
)

func (a *Access) Rename(tx *history.Transaction, node ids.NodeID, name string) error {
	h, err := a.handle(node)
	if err != nil {
		return err
	}
	before := a.s.Name(h)
	if before == name || name == "" {
		return nil
	}
	return a.within(tx, "Rename", func(tx *history.Transaction) error {
		return tx.Exec(&commands.Rename{Node: node, Before: before, After: name})
	})
}

func (a *Access) SetEnabled(tx *history.Transaction, nodes []ids.NodeID, v bool) error {
	return a.within(tx, "Toggle Enabled", func(tx *history.Transaction) error {
		for _, n := range nodes {
			h, err := a.handle(n)
			if err != nil {
				return err
			}
			before := a.s.Enabled(h)
			if before == v {
				continue
			}
			if err := tx.Exec(&commands.SetEnabled{Node: n, Before: before, After: v}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *Access) SetLayers(tx *history.Transaction, nodes []ids.NodeID, mask uint64) error {
	return a.within(tx, "Set Layers", func(tx *history.Transaction) error {
		for _, n := range nodes {
			h, err := a.handle(n)
			if err != nil {
				return err
			}
			before := a.s.Layers(h)
			if before == mask {
				continue
			}
			if err := tx.Exec(&commands.SetLayers{Node: n, Before: before, After: mask}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *Access) SetSpace(tx *history.Transaction, nodes []ids.NodeID, sp scene.Space) error {
	return a.within(tx, "Change Space", func(tx *history.Transaction) error {
		for _, n := range nodes {
			h, err := a.handle(n)
			if err != nil {
				return err
			}
			if a.s.Space(h) == sp {
				continue
			}
			if err := tx.Exec(&commands.SetSpace{Node: n, After: sp}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *Access) EditTransform(tx *history.Transaction, nodes []ids.NodeID, fn func(*scene.Transform2D)) error {
	return a.within(tx, "Transform", func(tx *history.Transaction) error {
		for _, n := range nodes {
			h, err := a.handle(n)
			if err != nil {
				return err
			}
			if a.s.Space(h) != scene.Space2D {
				continue
			}
			before := a.s.Local(h)
			after := before
			fn(&after)
			if after == before {
				continue
			}
			if err := tx.Exec(&commands.SetTransform{Node: n, Before: before, After: after}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *Access) EditTransform3D(tx *history.Transaction, nodes []ids.NodeID, fn func(*scene.Transform3D)) error {
	return a.within(tx, "Transform", func(tx *history.Transaction) error {
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
			fn(&after)
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

func (a *Access) CommonTransform(nodes []ids.NodeID) (scene.Transform2D, [5]bool) {
	var first scene.Transform2D
	var mixed [5]bool
	for i, n := range nodes {
		h := a.s.Handle(n)
		if h.IsZero() {
			continue
		}
		t := a.s.Local(h)
		if i == 0 {
			first = t
			continue
		}
		mixed[0] = mixed[0] || t.Position.X != first.Position.X
		mixed[1] = mixed[1] || t.Position.Y != first.Position.Y
		mixed[2] = mixed[2] || t.Rotation != first.Rotation
		mixed[3] = mixed[3] || t.Scale.X != first.Scale.X
		mixed[4] = mixed[4] || t.Scale.Y != first.Scale.Y
	}
	return first, mixed
}

func transform3Values(t scene.Transform3D, euler mathx.Vec3) [9]float32 {
	return [9]float32{
		t.Position.X, t.Position.Y, t.Position.Z,
		euler.X, euler.Y, euler.Z,
		t.Scale.X, t.Scale.Y, t.Scale.Z,
	}
}

func (a *Access) CommonTransform3D(nodes []ids.NodeID) (scene.Transform3D, mathx.Vec3, [9]bool) {
	first := scene.IdentityTransform3D()
	var euler mathx.Vec3
	var mixed [9]bool
	var base [9]float32
	seen := false
	for _, n := range nodes {
		h := a.s.Handle(n)
		if h.IsZero() || a.s.Space(h) != scene.Space3D {
			continue
		}
		t := a.s.Local3D(h)
		x, y, z := t.Rotation.Euler()
		e := mathx.Vec3{X: x, Y: y, Z: z}
		vals := transform3Values(t, e)
		if !seen {
			first, euler, base, seen = t, e, vals, true
			continue
		}
		for i := range vals {
			mixed[i] = mixed[i] || vals[i] != base[i]
		}
	}
	return first, euler, mixed
}
