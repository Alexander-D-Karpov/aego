package art

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"

	"aego/art/internal/aseprite"
)

// AsepriteExportDiagnostic describes data that a plain Aseprite interchange
// file cannot retain. This is not a substitute for a versioned Aego metadata
// contract and must be shown before the caller writes an interchange copy.
type AsepriteExportDiagnostic struct {
	Code    string
	Entity  ID
	Message string
}

// EncodeAseprite creates an actual Aseprite file from one immutable snapshot.
// It is an interchange encoder, not yet a replacement for the canonical Aego
// save: a plain Aseprite file does not retain Aego entity/asset identities.
// Unsupported source data is rejected rather than silently flattened. No file
// is written and the original document and its history are never mutated.
func (s *Snapshot) EncodeAseprite() ([]byte, []AsepriteExportDiagnostic, error) {
	if s == nil || s.view == nil || s.view.current == nil {
		return nil, nil, fmt.Errorf("art: invalid Aseprite export snapshot")
	}
	a := aseEncoder{state: s.view.current, limits: s.view.limits}
	if err := a.prepare(); err != nil {
		return nil, a.notes, fmt.Errorf("art: Aseprite export: %w", err)
	}
	data, err := a.encode()
	if err != nil {
		return nil, a.notes, fmt.Errorf("art: Aseprite export: %w", err)
	}
	return data, a.notes, nil
}

type aseEncoder struct {
	state  *state
	limits Limits
	layers []Layer
	frames map[ID]int
	cels   map[[2]ID]Cel
	notes  []AsepriteExportDiagnostic
	depth  int
}

func (a *aseEncoder) reject(code string, entity ID, message string) error {
	a.notes = append(a.notes, AsepriteExportDiagnostic{code, entity, message})
	return fmt.Errorf("%s", message)
}

func aseStringOK(s string) bool { return len(s) <= math.MaxUint16 && utf8.ValidString(s) }

func (a *aseEncoder) prepare() error {
	s, lim := a.state, a.limits
	if err := s.validate(lim); err != nil {
		return err
	}
	if s.Canvas.Width > math.MaxUint16 || s.Canvas.Height > math.MaxUint16 || len(s.Frames) > math.MaxUint16 || len(s.Layers) > math.MaxUint16 || len(s.Tags) > math.MaxUint16 {
		return ErrLimit
	}
	if len(s.Tags) > lim.MaxFrames || len(s.Slices) > lim.MaxFrames {
		return ErrLimit // The current importer also bounds these collections.
	}
	switch s.Canvas.Mode {
	case RGBA:
		a.depth = 32
	case Grayscale:
		a.depth = 16
	case Indexed:
		a.depth = 8
	default:
		return fmt.Errorf("unsupported color mode %q", s.Canvas.Mode)
	}
	if len(s.Properties) != 0 {
		return a.reject("properties", "", "Aego document properties require a metadata contract before Aseprite save")
	}
	for _, swatch := range s.Palette {
		if !aseStringOK(swatch.Name) {
			return fmt.Errorf("palette name is not a valid Aseprite string")
		}
	}
	// The core stores siblings in their draw order but does not require children
	// to be adjacent to their parent. Aseprite requires a depth-first sequence.
	children := make(map[ID][]Layer)
	for _, layer := range s.Layers {
		if !aseStringOK(layer.Name) {
			return fmt.Errorf("layer name is not a valid Aseprite string")
		}
		if layer.AlphaLock {
			return a.reject("alpha-lock", layer.ID, "Aego alpha lock requires a metadata contract before Aseprite save")
		}
		children[layer.Parent] = append(children[layer.Parent], layer)
	}
	var appendChildren func(ID, int) error
	appendChildren = func(parent ID, depth int) error {
		if depth > len(s.Layers) || depth > math.MaxUint16 {
			return ErrLimit
		}
		for _, layer := range children[parent] {
			a.layers = append(a.layers, layer)
			if layer.Kind == Group {
				if err := appendChildren(layer.ID, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := appendChildren("", 0); err != nil {
		return err
	}
	if len(a.layers) != len(s.Layers) {
		return fmt.Errorf("invalid layer hierarchy")
	}
	a.frames = make(map[ID]int, len(s.Frames))
	for i, frame := range s.Frames {
		if frame.DurationMS > math.MaxUint16 {
			return a.reject("frame-duration", frame.ID, "Aseprite frame durations cannot exceed 65535 milliseconds")
		}
		a.frames[frame.ID] = i
	}
	for _, tag := range s.Tags {
		if !aseStringOK(tag.Name) {
			return fmt.Errorf("tag name is not a valid Aseprite string")
		}
	}
	// Aseprite slices use persistent keyframes. Aego's frame-specific slices
	// have exact-frame semantics, so writing them as persistent keys would
	// silently change other frames. Static, uniquely named slices are safe.
	names := make(map[string]bool)
	for _, slice := range s.Slices {
		if !aseStringOK(slice.Name) {
			return fmt.Errorf("slice name is not a valid Aseprite string")
		}
		if names[slice.Name] || slice.Frame != "" {
			return a.reject("slice-timing", slice.ID, "frame-specific or duplicate slices require a metadata contract before Aseprite save")
		}
		names[slice.Name] = true
		r := slice.Rect
		if r.Empty() || r.Min.X < -lim.MaxDimension || r.Min.X > lim.MaxDimension || r.Min.Y < -lim.MaxDimension || r.Min.Y > lim.MaxDimension ||
			int64(r.Min.X) < math.MinInt32 || int64(r.Min.X) > math.MaxInt32 || int64(r.Min.Y) < math.MinInt32 || int64(r.Min.Y) > math.MaxInt32 ||
			int64(r.Max.X) < math.MinInt32 || int64(r.Max.X) > math.MaxInt32 || int64(r.Max.Y) < math.MinInt32 || int64(r.Max.Y) > math.MaxInt32 ||
			r.Max.X < r.Min.X || r.Max.Y < r.Min.Y || int64(r.Max.X)-int64(r.Min.X) > int64(lim.MaxDimension) || int64(r.Max.Y)-int64(r.Min.Y) > int64(lim.MaxDimension) {
			return a.reject("slice-bounds", slice.ID, "slice bounds are outside the supported Aseprite range")
		}
		if int64(slice.Pivot.X) < math.MinInt32 || int64(slice.Pivot.X) > math.MaxInt32 || int64(slice.Pivot.Y) < math.MinInt32 || int64(slice.Pivot.Y) > math.MaxInt32 {
			return a.reject("slice-pivot", slice.ID, "slice pivot is outside the Aseprite range")
		}
		px, py := int64(slice.Pivot.X)-int64(r.Min.X), int64(slice.Pivot.Y)-int64(r.Min.Y)
		if px < math.MinInt32 || px > math.MaxInt32 || py < math.MinInt32 || py > math.MaxInt32 {
			return a.reject("slice-pivot", slice.ID, "slice pivot is outside the Aseprite range")
		}
	}
	a.cels = make(map[[2]ID]Cel, len(s.Cels))
	owners := make(map[ID]ID)
	var decoded int64
	for _, cel := range s.Cels {
		if cel.Channel != Albedo {
			return a.reject("surface-channel", cel.ID, "normal, emission and height channels require a metadata contract before Aseprite save")
		}
		if cel.X < math.MinInt16 || cel.X > math.MaxInt16 || cel.Y < math.MinInt16 || cel.Y > math.MaxInt16 {
			return a.reject("cel-position", cel.ID, "cel position is outside the Aseprite range")
		}
		if owner, exists := owners[cel.Image]; exists && owner != cel.Layer {
			return a.reject("cross-layer-link", cel.ID, "Aseprite cannot preserve a shared cel image across different layers")
		} else if !exists {
			owners[cel.Image] = cel.Layer
			p := s.Images[cel.Image]
			if p.Width > math.MaxUint16 || p.Height > math.MaxUint16 {
				return ErrLimit
			}
			n := int64(p.Width) * int64(p.Height) * int64(a.depth/8)
			if n > lim.MaxPixelBytes-decoded {
				return ErrLimit
			}
			decoded += n
		}
		a.cels[[2]ID{cel.Frame, cel.Layer}] = cel
	}
	a.notes = append(a.notes, AsepriteExportDiagnostic{
		Code: "aego-identities", Message: "Plain Aseprite interchange does not preserve Aego document, frame, cel, layer, tag or asset identities; keep the original source and its metadata.",
	})
	return nil
}

// The adapter supplies Art's limit error; the codec owns byte framing.
type aseOutput struct{ *aseprite.Output }

func (w *aseOutput) u8(v uint8)                  { w.U8(v) }
func (w *aseOutput) u16(v uint16)                { w.U16(v) }
func (w *aseOutput) u32(v uint32)                { w.U32(v) }
func (w *aseOutput) zeros(n int)                 { w.Zeros(n) }
func (w *aseOutput) text(s string)               { w.Text(s) }
func (w *aseOutput) patchSize(start int)         { w.PatchSize(start) }
func (w *aseOutput) chunk(kind uint16, f func()) { w.Chunk(kind, f) }

func (a *aseEncoder) encode() ([]byte, error) {
	s := a.state
	w := &aseOutput{aseprite.NewOutput(min(a.limits.MaxFileBytes, int64(math.MaxUint32)), ErrLimit)}
	w.zeros(128)
	layerIndex := make(map[ID]uint16, len(a.layers))
	layerDepth := make(map[ID]uint16, len(a.layers))
	for i, layer := range a.layers {
		layerIndex[layer.ID] = uint16(i)
		if layer.Parent != "" {
			layerDepth[layer.ID] = layerDepth[layer.Parent] + 1
		}
	}
	type imageOwner struct{ image, layer ID }
	firstFrame := make(map[imageOwner]uint16)
	chunksTotal := 0
	for frameNo, frame := range s.Frames {
		start := w.Len()
		w.u32(0)
		w.u16(0xf1fa)
		w.u16(0) // old chunk count, patched below
		w.u16(uint16(frame.DurationMS))
		w.u16(0)
		w.u32(0) // new chunk count, patched below
		count := 0
		chunk := func(kind uint16, payload func()) {
			count++
			w.chunk(kind, payload)
		}
		if frameNo == 0 {
			for _, layer := range a.layers {
				chunk(0x2004, func() {
					var flags uint16
					if layer.Visible {
						flags |= 1
					}
					if !layer.Locked {
						flags |= 2
					}
					if layer.Kind == Reference {
						flags |= 64
					}
					w.u16(flags)
					if layer.Kind == Group {
						w.u16(1)
					} else {
						w.u16(0)
					}
					w.u16(layerDepth[layer.ID])
					w.u16(0)
					w.u16(0)
					switch layer.Blend {
					case BlendNormal:
						w.u16(0)
					case BlendMultiply:
						w.u16(1)
					case BlendScreen:
						w.u16(2)
					case BlendAdd:
						w.u16(16)
					}
					w.u8(layer.Opacity)
					w.zeros(3)
					w.text(layer.Name)
				})
			}
			chunk(0x2007, func() { w.u16(1); w.u16(0); w.u32(0); w.zeros(8) })
			chunk(0x2019, func() {
				w.u32(uint32(len(s.Palette)))
				w.u32(0)
				w.u32(uint32(len(s.Palette) - 1))
				w.zeros(8)
				for _, swatch := range s.Palette {
					if swatch.Name != "" {
						w.u16(1)
					} else {
						w.u16(0)
					}
					c := swatch.Color
					_, _ = w.Write([]byte{c.R, c.G, c.B, c.A})
					if swatch.Name != "" {
						w.text(swatch.Name)
					}
				}
			})
			if len(s.Tags) > 0 {
				chunk(0x2018, func() {
					w.u16(uint16(len(s.Tags)))
					w.zeros(8)
					for _, tag := range s.Tags {
						w.u16(uint16(a.frames[tag.From]))
						w.u16(uint16(a.frames[tag.To]))
						switch tag.Direction {
						case Forward:
							w.u8(0)
						case Reverse:
							w.u8(1)
						case PingPong:
							w.u8(2)
						}
						w.u16(0)
						w.zeros(10)
						w.text(tag.Name)
					}
				})
			}
			for _, slice := range s.Slices {
				chunk(0x2022, func() {
					w.u32(1)
					w.u32(2)
					w.u32(0)
					w.text(slice.Name)
					w.u32(0)
					w.u32(uint32(int32(slice.Rect.Min.X)))
					w.u32(uint32(int32(slice.Rect.Min.Y)))
					w.u32(uint32(slice.Rect.Dx()))
					w.u32(uint32(slice.Rect.Dy()))
					w.u32(uint32(int32(int64(slice.Pivot.X) - int64(slice.Rect.Min.X))))
					w.u32(uint32(int32(int64(slice.Pivot.Y) - int64(slice.Rect.Min.Y))))
				})
			}
		}
		for _, layer := range a.layers {
			cel, ok := a.cels[[2]ID{frame.ID, layer.ID}]
			if !ok {
				continue
			}
			chunk(0x2005, func() {
				w.u16(layerIndex[layer.ID])
				w.u16(uint16(int16(cel.X)))
				w.u16(uint16(int16(cel.Y)))
				w.u8(cel.Opacity)
				key := imageOwner{cel.Image, layer.ID}
				target, linked := firstFrame[key]
				if linked {
					w.u16(1)
				} else {
					w.u16(2)
				}
				w.u16(0)
				w.zeros(5)
				if linked {
					w.u16(target)
					return
				}
				firstFrame[key] = uint16(frameNo)
				p := s.Images[cel.Image]
				w.u16(uint16(p.Width))
				w.u16(uint16(p.Height))
				if w.Err() == nil {
					w.SetError(a.writePixels(w, p))
				}
			})
		}
		chunksTotal += count
		if chunksTotal > 65536 {
			return nil, ErrLimit
		}
		w.patchSize(start)
		if w.Err() != nil {
			return nil, w.Err()
		}
		binary.LittleEndian.PutUint16(w.Bytes()[start+6:], uint16(min(count, math.MaxUint16)))
		binary.LittleEndian.PutUint32(w.Bytes()[start+12:], uint32(count))
	}
	if w.Err() != nil {
		return nil, w.Err()
	}
	header := w.Bytes()[:128]
	binary.LittleEndian.PutUint32(header, uint32(w.Len()))
	binary.LittleEndian.PutUint16(header[4:], 0xa5e0)
	binary.LittleEndian.PutUint16(header[6:], uint16(len(s.Frames)))
	binary.LittleEndian.PutUint16(header[8:], uint16(s.Canvas.Width))
	binary.LittleEndian.PutUint16(header[10:], uint16(s.Canvas.Height))
	binary.LittleEndian.PutUint16(header[12:], uint16(a.depth))
	binary.LittleEndian.PutUint32(header[14:], 3) // layer opacity and group blend/opacity
	binary.LittleEndian.PutUint16(header[18:], uint16(s.Frames[0].DurationMS))
	header[28] = 0 // Aego indexed documents reserve palette entry zero.
	binary.LittleEndian.PutUint16(header[32:], uint16(len(s.Palette)))
	header[34], header[35] = 1, 1
	return w.Bytes(), nil
}

func (a *aseEncoder) writePixels(w *aseOutput, p *pixels) error {
	row := make([]byte, p.Width*(a.depth/8))
	return aseprite.DeflateRows(w, p.Height, func(y int) ([]byte, error) {
		for x := 0; x < p.Width; x++ {
			v := p.at(x, y)
			switch a.depth {
			case 32:
				binary.LittleEndian.PutUint32(row[x*4:], v)
			case 16:
				c := unpack(v)
				if c.R != c.G || c.R != c.B {
					return nil, fmt.Errorf("non-grayscale pixel in grayscale cel")
				}
				row[x*2], row[x*2+1] = c.R, c.A
			case 8:
				if v >= uint32(len(a.state.Palette)) {
					return nil, fmt.Errorf("pixel refers to missing palette entry")
				}
				row[x] = byte(v)
			}
		}
		return row, nil
	})
}
