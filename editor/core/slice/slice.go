// Package slice cuts a sprite sheet into cells stored as sub-assets in its meta (D339).
package slice

import (
	"cmp"
	"fmt"
	"slices"

	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/history"
)

type Rect = [4]int

// Grid cuts w×h into cells of size cell, after offset, with padding between them, row by row; count 0 keeps every cell.
func Grid(w, h int, cell, offset, padding [2]int, count int) []Rect {
	var out []Rect
	if cell[0] <= 0 || cell[1] <= 0 {
		return out
	}
	for y := offset[1]; y+cell[1] <= h; y += cell[1] + padding[1] {
		for x := offset[0]; x+cell[0] <= w; x += cell[0] + padding[0] {
			if count > 0 && len(out) == count {
				return out
			}
			out = append(out, Rect{x, y, cell[0], cell[1]})
		}
	}
	return out
}

// Alpha finds the bounding boxes of the 8-connected areas whose alpha is above threshold, in rows from the top, each row from the left.
func Alpha(rgba []byte, w, h int, threshold uint8) []Rect {
	seen := make([]bool, w*h)
	var out []Rect
	var stack []int
	solid := func(i int) bool { return rgba[i*4+3] > threshold }
	for start := range w * h {
		if seen[start] || !solid(start) {
			continue
		}
		x0, y0, x1, y1 := w, h, -1, -1
		stack = append(stack[:0], start)
		seen[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := i%w, i/w
			x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if j := ny*w + nx; !seen[j] && solid(j) {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		out = append(out, Rect{x0, y0, x1 - x0 + 1, y1 - y0 + 1})
	}
	return rows(out)
}

// rows orders boxes in rows: a box joins the row of the first box it overlaps vertically.
func rows(boxes []Rect) []Rect {
	slices.SortFunc(boxes, func(a, b Rect) int { return cmp.Or(cmp.Compare(a[1], b[1]), cmp.Compare(a[0], b[0])) })
	var out []Rect
	for len(boxes) > 0 {
		top := boxes[0]
		var row, rest []Rect
		for _, b := range boxes {
			if b[1] < top[1]+top[3] {
				row = append(row, b)
			} else {
				rest = append(rest, b)
			}
		}
		slices.SortFunc(row, func(a, b Rect) int { return cmp.Compare(a[0], b[0]) })
		out = append(out, row...)
		boxes = rest
	}
	return out
}

// Name is the name of cell i of a sheet: hero_000, hero_001…
func Name(base string, i int) string {
	return fmt.Sprintf("%s_%03d", base, i)
}

// Plan names the cells and keeps the id of every cell whose name existed before, so references survive a new slice.
func Plan(base string, old []pipeline.SubAsset, rects []Rect) []pipeline.SubAsset {
	byName := map[string]ids.AssetID{}
	for _, s := range old {
		byName[s.Name] = s.ID
	}
	out := make([]pipeline.SubAsset, len(rects))
	for i, r := range rects {
		name := Name(base, i)
		id, ok := byName[name]
		if !ok {
			id = ids.NewAssetID()
		}
		out[i] = pipeline.SubAsset{Name: name, ID: id, Rect: r}
	}
	return out
}

// Edit writes the cells or, when AfterSettings is set, the import settings of a sheet and reimports it; Undo writes the previous ones back.
type Edit struct {
	Pipeline       *pipeline.Pipeline
	Sheet          ids.AssetID
	Before         []pipeline.SubAsset
	After          []pipeline.SubAsset
	BeforeSettings pipeline.Settings
	AfterSettings  pipeline.Settings
	Apply          func(*pipeline.Batch)
}

func (e *Edit) Label() string {
	if e.AfterSettings != nil {
		return "Import Settings"
	}
	return "Slice Sprite Sheet"
}

func (e *Edit) Do(history.Target) error   { return e.write(e.After, e.AfterSettings) }
func (e *Edit) Undo(history.Target) error { return e.write(e.Before, e.BeforeSettings) }
func (e *Edit) Merge(history.Command) bool {
	return false
}

func (e *Edit) write(subs []pipeline.SubAsset, set pipeline.Settings) error {
	var err error
	if e.AfterSettings != nil {
		err = e.Pipeline.SetSettings(e.Sheet, set)
	} else {
		err = e.Pipeline.SetSubs(e.Sheet, subs)
	}
	if err != nil {
		return err
	}
	b, err := e.Pipeline.Reimport(e.Sheet)
	if e.Apply != nil {
		e.Apply(b)
	}
	if err != nil {
		return err
	}
	if ferr := b.Failed[e.Sheet]; ferr != nil {
		return ferr
	}
	return nil
}

var _ history.Command = (*Edit)(nil)
