package art

import "fmt"

// CellSlot includes empty cells, so an overlapping rectangular move has the
// same semantics as cutting and pasting a range in the Layers x Frames grid.
type CellSlot struct {
	Layer   ID      `json:"layer"`
	Frame   ID      `json:"frame"`
	Channel Channel `json:"channel"`
}

// TransferCels preflights every destination before altering the private edit.
// Moving preserves Cel and Image IDs. Copying gives independent image IDs while
// retaining links within the copied range; explicit Link retains source links.
func (e *Edit) TransferCels(slots []CellSlot, layerOffset, frameOffset int, copyCells, linked, overwrite bool) error {
	if err := e.check(); err != nil {
		return err
	}
	if len(slots) == 0 || len(slots) > e.doc.limits.MaxCels {
		return ErrLimit
	}
	if !copyCells && linked {
		return fmt.Errorf("art: linked transfer requires Copy")
	}
	source := make(map[CellSlot]bool, len(slots))
	dest := make(map[CellSlot]bool, len(slots))
	type item struct {
		from, to CellSlot
		cel      Cel
		exists   bool
	}
	items := make([]item, 0, len(slots))
	existing := make(map[CellSlot]Cel, len(e.work.Cels))
	for _, c := range e.work.Cels {
		existing[CellSlot{c.Layer, c.Frame, c.Channel}] = c
	}
	for _, s := range slots {
		if s.Channel == "" {
			s.Channel = Albedo
		}
		if source[s] {
			return fmt.Errorf("art: duplicate source cell")
		}
		li, lok := e.work.layer(s.Layer)
		fi, fok := e.work.frame(s.Frame)
		if !lok || !fok || !validChannel(s.Channel) || e.work.Layers[li].Kind == Group {
			return fmt.Errorf("art: invalid source cell")
		}
		if !copyCells && e.work.isLocked(s.Layer) {
			return ErrLocked
		}
		dl, df := li+layerOffset, fi+frameOffset
		if dl < 0 || dl >= len(e.work.Layers) || df < 0 || df >= len(e.work.Frames) {
			return fmt.Errorf("art: cell transfer exceeds timeline bounds")
		}
		layer := e.work.Layers[dl]
		if layer.Kind == Group {
			return fmt.Errorf("art: a group cannot contain a cel")
		}
		if e.work.isLocked(layer.ID) {
			return ErrLocked
		}
		to := CellSlot{layer.ID, e.work.Frames[df].ID, s.Channel}
		if dest[to] {
			return fmt.Errorf("art: duplicate destination cell")
		}
		c, ok := existing[s]
		items = append(items, item{s, to, c, ok})
		source[s] = true
		dest[to] = true
	}
	if layerOffset == 0 && frameOffset == 0 {
		return nil
	}
	if !overwrite {
		for s := range dest {
			if _, ok := existing[s]; ok && !source[s] {
				return fmt.Errorf("art: destination contains a cel; confirm overwrite")
			}
		}
	}
	kept := make([]Cel, 0, len(e.work.Cels)+len(items))
	for _, c := range e.work.Cels {
		s := CellSlot{c.Layer, c.Frame, c.Channel}
		if dest[s] || (!copyCells && source[s]) {
			continue
		}
		kept = append(kept, c)
	}
	count := len(kept)
	for _, v := range items {
		if v.exists {
			count++
		}
	}
	if count > e.doc.limits.MaxCels {
		return ErrLimit
	}
	copies := map[ID]ID{}
	for _, v := range items {
		if !v.exists {
			continue
		}
		c := v.cel
		c.Layer, c.Frame = v.to.Layer, v.to.Frame
		if copyCells {
			c.ID = e.doc.newID()
			if !linked {
				im := copies[c.Image]
				if im == "" {
					im = e.doc.newID()
					e.shareIndependent(c.Image, im)
					copies[c.Image] = im
				}
				c.Image = im
			}
		}
		kept = append(kept, c)
	}
	e.work.Cels = kept
	e.structural = true
	return nil
}

// ReorderFrames preserves stable identities and exact tag membership. Since a
// tag is a contiguous range, a reorder that splits its members is refused rather
// than silently including unrelated frames or dropping part of the animation.
func (e *Edit) ReorderFrames(order []ID) error {
	if err := e.check(); err != nil {
		return err
	}
	if len(order) != len(e.work.Frames) {
		return fmt.Errorf("art: frame order must be a permutation")
	}
	positions := map[ID]int{}
	frames := make([]Frame, len(order))
	for i, id := range order {
		if _, ok := positions[id]; ok {
			return fmt.Errorf("art: duplicate frame")
		}
		n, ok := e.work.frame(id)
		if !ok {
			return fmt.Errorf("art: missing frame")
		}
		positions[id] = i
		frames[i] = e.work.Frames[n]
	}
	tags := append([]Tag(nil), e.work.Tags...)
	for n, t := range tags {
		from, _ := e.work.frame(t.From)
		to, _ := e.work.frame(t.To)
		low, high := len(frames), -1
		for _, f := range e.work.Frames[from : to+1] {
			pos := positions[f.ID]
			low = min(low, pos)
			high = max(high, pos)
		}
		if high-low != to-from {
			return fmt.Errorf("art: frame reorder would split animation tag %q", t.Name)
		}
		tags[n].From, tags[n].To = frames[low].ID, frames[high].ID
	}
	e.work.Frames, e.work.Tags = frames, tags
	e.structural = true
	return nil
}

func (e *Edit) DeleteTag(id ID) error {
	if err := e.check(); err != nil {
		return err
	}
	for i, t := range e.work.Tags {
		if t.ID == id {
			e.work.Tags = append(e.work.Tags[:i], e.work.Tags[i+1:]...)
			e.structural = true
			return nil
		}
	}
	return fmt.Errorf("art: missing animation tag")
}
