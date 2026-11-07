package art

import (
	"context"
	"fmt"
	"image"
)

// Operation is shared by pointer clients, CLI replay, and confirmed proposals.
// It describes intent, never a filesystem path or executable command.
type Operation struct {
	Palette      []Swatch      `json:"palette,omitempty"`
	ColorLimit   int           `json:"colorLimit,omitempty"`
	Quantize     bool          `json:"quantize,omitempty"`
	RampFirst    int           `json:"rampFirst,omitempty"`
	RampLast     int           `json:"rampLast,omitempty"`
	Cells        []CellSlot    `json:"cells,omitempty"`
	FrameOrder   []ID          `json:"frameOrder,omitempty"`
	FrameOffset  int           `json:"frameOffset,omitempty"`
	LayerOffset  int           `json:"layerOffset,omitempty"`
	Copy         bool          `json:"copy,omitempty"`
	Overwrite    bool          `json:"overwrite,omitempty"`
	Kind         string        `json:"kind"`
	As           string        `json:"as,omitempty"`
	Cel          ID            `json:"cel,omitempty"`
	Layer        ID            `json:"layer,omitempty"`
	Frame        ID            `json:"frame,omitempty"`
	Parent       ID            `json:"parent,omitempty"`
	Channel      Channel       `json:"channel,omitempty"`
	Name         string        `json:"name,omitempty"`
	LayerKind    LayerKind     `json:"layerKind,omitempty"`
	LayerValue   *Layer        `json:"layerValue,omitempty"`
	Tag          *Tag          `json:"tag,omitempty"`
	Points       []image.Point `json:"points,omitempty"`
	Brush        Brush         `json:"brush,omitempty"`
	Filled       bool          `json:"filled,omitempty"`
	Linked       bool          `json:"linked,omitempty"`
	Tolerance    int           `json:"tolerance,omitempty"`
	DurationMS   int           `json:"durationMS,omitempty"`
	Offset       int           `json:"offset,omitempty"`
	Width        int           `json:"width,omitempty"`
	Height       int           `json:"height,omitempty"`
	Mode         Mode          `json:"mode,omitempty"`
	PaletteOrder []int         `json:"paletteOrder,omitempty"`
	Swatch       *Swatch       `json:"swatch,omitempty"`
	Index        int           `json:"index,omitempty"`
}

func (e *Edit) Execute(ctx context.Context, op Operation, selection *Selection) (ID, error) {
	if err := e.check(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(op.Points) > 65536 {
		return "", ErrLimit
	}
	ch := op.Channel
	if ch == "" {
		ch = Albedo
	}
	switch op.Kind {
	case "add-layer":
		return e.AddLayer(op.Name, op.LayerKind, op.Parent)
	case "set-layer":
		if op.LayerValue == nil {
			return "", fmt.Errorf("art: layer value is required")
		}
		return "", e.SetLayer(*op.LayerValue)
	case "delete-layer":
		return "", e.DeleteLayer(op.Layer)
	case "duplicate-layer":
		return e.DuplicateLayer(op.Layer)
	case "move-layer":
		return "", e.MoveLayer(op.Layer, op.Offset)
	case "add-frame":
		return e.AddFrame(op.Frame, op.DurationMS)
	case "duplicate-frame":
		return e.DuplicateFrame(op.Frame, op.Linked)
	case "delete-frame":
		return "", e.DeleteFrame(op.Frame)
	case "duration":
		return "", e.SetDuration(op.Frame, op.DurationMS)
	case "ensure-cel":
		return e.EnsureCel(op.Layer, op.Frame, ch)
	case "copy-cel":
		return e.CopyCel(op.Cel, op.Layer, op.Frame, op.Linked)
	case "unlink":
		return "", e.Unlink(op.Cel)
	case "tag":
		if op.Tag == nil {
			return "", fmt.Errorf("art: tag is required")
		}
		return e.SetTag(*op.Tag)
	case "frame-order":
		return "", e.ReorderFrames(op.FrameOrder)
	case "cel-range":
		return "", e.TransferCels(op.Cells, op.LayerOffset, op.FrameOffset, op.Copy, op.Linked, op.Overwrite)
	case "delete-tag":
		if op.Tag == nil {
			return "", fmt.Errorf("art: tag is required")
		}
		return "", e.DeleteTag(op.Tag.ID)
	case "palette-load":
		return "", e.ReplacePalette(op.Palette, op.Quantize)
	case "palette-extract":
		return "", e.PaletteFromFrame(ctx, op.Frame, op.ColorLimit, op.Quantize)
	case "ramp-shade":
		return "", e.ShadeRamp(ctx, op.Cel, selection, op.RampFirst, op.RampLast, op.Offset)
	case "palette-reorder":
		return "", e.ReorderPalette(op.PaletteOrder)
	case "palette-remap":
		return "", e.RemapPalette(op.PaletteOrder)
	case "palette-color":
		if op.Swatch == nil {
			return "", fmt.Errorf("art: swatch is required")
		}
		return "", e.SetPaletteEntry(op.Index, *op.Swatch)
	case "mode":
		return "", e.ConvertMode(op.Mode)
	case "clear":
		return "", e.Clear(op.Cel, selection)
	case "flip-x", "flip-y", "rotate-cw", "rotate-ccw", "scale":
		return "", e.Transform(op.Cel, selection, op.Kind, op.Width, op.Height, 0, 0)
	}
	if len(op.Points) == 0 {
		return "", fmt.Errorf("art: drawing points are required")
	}
	cel := op.Cel
	if cel == "" {
		var err error
		cel, err = e.EnsureCel(op.Layer, op.Frame, ch)
		if err != nil {
			return "", err
		}
	}
	switch op.Kind {
	case "stroke":
		g, err := e.Stroke(cel, op.Brush, selection)
		if err != nil {
			return "", err
		}
		for _, p := range op.Points {
			if err = ctx.Err(); err != nil {
				return "", err
			}
			if err = g.Point(p); err != nil {
				return "", err
			}
		}
		return cel, nil
	case "fill":
		p := op.Points[0]
		return cel, e.Fill(ctx, cel, p.X, p.Y, op.Tolerance, op.Brush.Paint, selection, true)
	case "line", "rectangle", "ellipse":
		if len(op.Points) != 2 {
			return "", fmt.Errorf("art: shape needs two points")
		}
		return cel, e.Shape(cel, op.Kind, op.Points[0], op.Points[1], op.Brush, selection, op.Filled)
	case "polygon":
		return cel, e.Polygon(cel, op.Points, op.Brush, selection, op.Filled)
	case "curve":
		if len(op.Points) != 4 {
			return "", fmt.Errorf("art: curve needs four points")
		}
		return cel, e.Curve(cel, op.Points[0], op.Points[1], op.Points[2], op.Points[3], op.Brush, selection)
	case "gradient":
		if len(op.Points) != 2 {
			return "", fmt.Errorf("art: gradient needs two points")
		}
		return cel, e.Gradient(ctx, cel, op.Points[0], op.Points[1], op.Brush.Paint, op.Brush.Background, selection, op.Brush.Dither > 0)
	case "move":
		p := op.Points[0]
		return cel, e.Transform(cel, selection, "move", 0, 0, p.X, p.Y)
	default:
		return "", fmt.Errorf("art: unsupported operation %q", op.Kind)
	}
}

// Propose builds a single all-or-nothing change at an exact revision. It cannot
// publish pixels; the caller must confirm it and execute it through history.
func (d *Document) Propose(ctx context.Context, revision uint64, label string, ops []Operation) (*Change, []ID, error) {
	if len(ops) == 0 || len(ops) > 4096 {
		return nil, nil, fmt.Errorf("art: proposal needs 1..4096 operations")
	}
	e, err := d.Begin(revision, label)
	if err != nil {
		return nil, nil, err
	}
	var results []ID
	names := make(map[string]ID)
	for _, op := range ops {
		if op.As != "" {
			if _, exists := names[op.As]; exists || !validResultName(op.As) {
				e.Cancel()
				return nil, nil, fmt.Errorf("art: invalid or repeated batch result %q", op.As)
			}
		}
		op, err = resolveOperation(op, names)
		if err != nil {
			e.Cancel()
			return nil, nil, err
		}
		id, err := e.Execute(ctx, op, nil)
		if err != nil {
			e.Cancel()
			return nil, nil, err
		}
		if op.As != "" {
			if id == "" {
				e.Cancel()
				return nil, nil, fmt.Errorf("art: operation %q does not return an entity", op.Kind)
			}
			names[op.As] = id
		}
		results = append(results, id)
	}
	c, err := e.Prepare()
	if err != nil {
		e.Cancel()
		return nil, nil, err
	}
	return c, results, nil
}

type Checkpoint struct {
	owner string
	state *state
}

func (d *Document) Checkpoint(expected uint64) (Checkpoint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return Checkpoint{}, ErrClosed
	}
	if expected != d.revision {
		return Checkpoint{}, ErrConflict
	}
	return Checkpoint{d.id, d.current}, nil
}
func (d *Document) DirtySince(saved Checkpoint) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return saved.owner != d.id || saved.state != d.current
}
