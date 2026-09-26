package props

import (
	"aego/core/ids"
	"aego/editor/history"
	"aego/editor/history/commands"
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

func (a *Access) EditTransform(tx *history.Transaction, nodes []ids.NodeID, fn func(*scene.Transform2D)) error {
	return a.within(tx, "Transform", func(tx *history.Transaction) error {
		for _, n := range nodes {
			h, err := a.handle(n)
			if err != nil {
				return err
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
