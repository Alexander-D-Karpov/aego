// Package art owns CPU pixel documents and reversible authoring operations.
// It has no dependency on an editor, graphics API, browser, or file system.
package art

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"sync"
)

const TileSize = 16

type ID string
type Mode string
type Channel string
type LayerKind string
type Blend string
type Direction string

const (
	RGBA          Mode      = "rgba"
	Grayscale     Mode      = "grayscale"
	Indexed       Mode      = "indexed"
	Albedo        Channel   = "albedo"
	Normal        Channel   = "normal"
	Height        Channel   = "height"
	Emission      Channel   = "emission"
	Raster        LayerKind = "raster"
	Group         LayerKind = "group"
	Reference     LayerKind = "reference"
	BlendNormal   Blend     = "normal"
	BlendMultiply Blend     = "multiply"
	BlendScreen   Blend     = "screen"
	BlendAdd      Blend     = "add"
	Forward       Direction = "forward"
	Reverse       Direction = "reverse"
	PingPong      Direction = "ping-pong"
)

var (
	ErrConflict = errors.New("art: document revision changed")
	ErrClosed   = errors.New("art: document is closed")
	ErrLocked   = errors.New("art: layer is locked")
	ErrLimit    = errors.New("art: document limit exceeded")
	ErrDone     = errors.New("art: operation is already finished")
)

type Color struct{ R, G, B, A uint8 }

func (c Color) packed() uint32 {
	return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16 | uint32(c.A)<<24
}
func unpack(v uint32) Color { return Color{uint8(v), uint8(v >> 8), uint8(v >> 16), uint8(v >> 24)} }

type Swatch struct {
	Color Color  `json:"color"`
	Name  string `json:"name,omitempty"`
}
type Canvas struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Mode    Mode   `json:"mode"`
	Profile string `json:"profile"`
}
type Layer struct {
	ID        ID        `json:"id"`
	Parent    ID        `json:"parent,omitempty"`
	Name      string    `json:"name"`
	Kind      LayerKind `json:"kind"`
	Visible   bool      `json:"visible"`
	Locked    bool      `json:"locked"`
	AlphaLock bool      `json:"alphaLock"`
	Opacity   uint8     `json:"opacity"`
	Blend     Blend     `json:"blend"`
}
type Frame struct {
	ID         ID  `json:"id"`
	DurationMS int `json:"durationMS"`
}
type Cel struct {
	ID      ID      `json:"id"`
	Layer   ID      `json:"layer"`
	Frame   ID      `json:"frame"`
	Image   ID      `json:"image"`
	Channel Channel `json:"channel"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Opacity uint8   `json:"opacity"`
}
type Tag struct {
	ID        ID        `json:"id"`
	Name      string    `json:"name"`
	From      ID        `json:"from"`
	To        ID        `json:"to"`
	Direction Direction `json:"direction"`
}
type Slice struct {
	ID    ID              `json:"id"`
	Name  string          `json:"name"`
	Frame ID              `json:"frame,omitempty"`
	Rect  image.Rectangle `json:"rect"`
	Pivot image.Point     `json:"pivot"`
}
type Limits struct {
	MaxDimension    int
	MaxCanvasPixels int
	MaxLayers       int
	MaxFrames       int
	MaxCels         int
	MaxPixelBytes   int64
	MaxFileBytes    int64
	MaxHistoryBytes int64
	MaxHistorySteps int
}

func DefaultLimits() Limits {
	return Limits{4096, 4194304, 256, 1024, 262144, 256 << 20, 64 << 20, 64 << 20, 256}
}
func limitsOrDefault(l Limits) Limits {
	d := DefaultLimits()
	if l.MaxDimension > 0 {
		d.MaxDimension = l.MaxDimension
	}
	if l.MaxCanvasPixels > 0 {
		d.MaxCanvasPixels = l.MaxCanvasPixels
	}
	if l.MaxLayers > 0 {
		d.MaxLayers = l.MaxLayers
	}
	if l.MaxFrames > 0 {
		d.MaxFrames = l.MaxFrames
	}
	if l.MaxCels > 0 {
		d.MaxCels = l.MaxCels
	}
	if l.MaxPixelBytes > 0 {
		d.MaxPixelBytes = l.MaxPixelBytes
	}
	if l.MaxFileBytes > 0 {
		d.MaxFileBytes = l.MaxFileBytes
	}
	if l.MaxHistoryBytes > 0 {
		d.MaxHistoryBytes = l.MaxHistoryBytes
	}
	if l.MaxHistorySteps > 0 {
		d.MaxHistorySteps = l.MaxHistorySteps
	}
	return d
}

type tile [TileSize * TileSize]uint32
type tileKey struct{ X, Y int }
type pixels struct {
	Width, Height int
	Tiles         map[tileKey]*tile
}

func newPixels(w, h int) *pixels { return &pixels{w, h, make(map[tileKey]*tile)} }
func (p *pixels) at(x, y int) uint32 {
	if x < 0 || y < 0 || x >= p.Width || y >= p.Height {
		return 0
	}
	t := p.Tiles[tileKey{x / TileSize, y / TileSize}]
	if t == nil {
		return 0
	}
	return t[(y%TileSize)*TileSize+x%TileSize]
}
func (p *pixels) clone() *pixels {
	q := newPixels(p.Width, p.Height)
	for k, t := range p.Tiles {
		q.Tiles[k] = t
	}
	return q
}

type state struct {
	Canvas     Canvas
	Palette    []Swatch
	Layers     []Layer
	Frames     []Frame
	Cels       []Cel
	Tags       []Tag
	Slices     []Slice
	Images     map[ID]*pixels
	Properties map[string]string
}

func (s *state) clone() *state {
	q := *s
	q.Palette = append([]Swatch(nil), s.Palette...)
	q.Layers = append([]Layer(nil), s.Layers...)
	q.Frames = append([]Frame(nil), s.Frames...)
	q.Cels = append([]Cel(nil), s.Cels...)
	q.Tags = append([]Tag(nil), s.Tags...)
	q.Slices = append([]Slice(nil), s.Slices...)
	q.Images = make(map[ID]*pixels, len(s.Images))
	for id, p := range s.Images {
		q.Images[id] = p
	}
	q.Properties = make(map[string]string, len(s.Properties))
	for k, v := range s.Properties {
		q.Properties[k] = v
	}
	return &q
}

// Info is a detached metadata snapshot. IDs are strings, so the JS bridge cannot
// round an entity ID through an IEEE-754 number.
type Info struct {
	ID         string            `json:"id"`
	Revision   uint64            `json:"revision"`
	Canvas     Canvas            `json:"canvas"`
	Palette    []Swatch          `json:"palette"`
	Layers     []Layer           `json:"layers"`
	Frames     []Frame           `json:"frames"`
	Cels       []Cel             `json:"cels"`
	Tags       []Tag             `json:"tags"`
	Slices     []Slice           `json:"slices"`
	Properties map[string]string `json:"properties"`
}

// FramePivot resolves the existing named pivot slices. A frame-specific pivot
// overrides the document-wide pivot regardless of their serialized order.
func (i Info) FramePivot(frame ID) (image.Point, bool) {
	var global, specific image.Point
	hasGlobal, hasSpecific := false, false
	for _, slice := range i.Slices {
		if slice.Name != "pivot" {
			continue
		}
		if slice.Frame == "" {
			global, hasGlobal = slice.Pivot, true
		} else if slice.Frame == frame {
			specific, hasSpecific = slice.Pivot, true
		}
	}
	if hasSpecific {
		return specific, true
	}
	return global, hasGlobal
}

type Document struct {
	mu       sync.Mutex
	id       string
	revision uint64
	next     uint64
	closed   bool
	limits   Limits
	current  *state
}

func New(w, h int, mode Mode, lim Limits) (*Document, error) {
	lim = limitsOrDefault(lim)
	if w < 1 || h < 1 || w > lim.MaxDimension || h > lim.MaxDimension || int64(w)*int64(h) > int64(lim.MaxCanvasPixels) {
		return nil, ErrLimit
	}
	if mode != RGBA && mode != Grayscale && mode != Indexed {
		return nil, fmt.Errorf("art: unsupported color mode %q", mode)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	s := &state{Canvas: Canvas{w, h, mode, "srgb"}, Palette: DefaultPalette(), Images: make(map[ID]*pixels), Properties: make(map[string]string)}
	d := &Document{id: hex.EncodeToString(id[:]), revision: 1, limits: lim, current: s}
	layer, frame, im, cel := d.allocate(), d.allocate(), d.allocate(), d.allocate()
	s.Layers = []Layer{{ID: layer, Name: "Layer 1", Kind: Raster, Visible: true, Opacity: 255, Blend: BlendNormal}}
	s.Frames = []Frame{{frame, 100}}
	s.Images[im] = newPixels(w, h)
	s.Cels = []Cel{{ID: cel, Layer: layer, Frame: frame, Image: im, Channel: Albedo, Opacity: 255}}
	return d, nil
}
func DefaultPalette() []Swatch {
	colors := []Color{{0, 0, 0, 0}, {24, 28, 42, 255}, {52, 62, 85, 255}, {98, 111, 137, 255}, {174, 187, 204, 255}, {240, 244, 248, 255}, {255, 105, 110, 255}, {232, 57, 81, 255}, {255, 174, 91, 255}, {255, 219, 110, 255}, {160, 220, 130, 255}, {64, 166, 131, 255}, {77, 195, 209, 255}, {70, 120, 208, 255}, {139, 116, 221, 255}, {224, 137, 207, 255}}
	out := make([]Swatch, len(colors))
	for i, c := range colors {
		out[i] = Swatch{Color: c}
	}
	return out
}
func (d *Document) allocate() ID     { d.next++; return ID(fmt.Sprintf("%016x", d.next)) }
func (d *Document) newID() ID        { d.mu.Lock(); defer d.mu.Unlock(); return d.allocate() }
func (d *Document) Revision() uint64 { d.mu.Lock(); defer d.mu.Unlock(); return d.revision }
func info(id string, rev uint64, s *state) Info {
	properties := make(map[string]string, len(s.Properties))
	for key, value := range s.Properties {
		properties[key] = value
	}
	return Info{id, rev, s.Canvas, append([]Swatch{}, s.Palette...),
		append([]Layer{}, s.Layers...), append([]Frame{}, s.Frames...),
		append([]Cel{}, s.Cels...), append([]Tag{}, s.Tags...),
		append([]Slice{}, s.Slices...), properties}
}
func (d *Document) Info() Info {
	d.mu.Lock()
	defer d.mu.Unlock()
	return info(d.id, d.revision, d.current)
}
func (d *Document) Close() { d.mu.Lock(); defer d.mu.Unlock(); d.closed = true; d.revision++ }
func (s *state) layer(id ID) (int, bool) {
	for i := range s.Layers {
		if s.Layers[i].ID == id {
			return i, true
		}
	}
	return 0, false
}
func (s *state) frame(id ID) (int, bool) {
	for i := range s.Frames {
		if s.Frames[i].ID == id {
			return i, true
		}
	}
	return 0, false
}
func (s *state) cel(id ID) (int, bool) {
	for i := range s.Cels {
		if s.Cels[i].ID == id {
			return i, true
		}
	}
	return 0, false
}
func (s *state) findCel(layer, frame ID, ch Channel) (int, bool) {
	for i, c := range s.Cels {
		if c.Layer == layer && c.Frame == frame && c.Channel == ch {
			return i, true
		}
	}
	return 0, false
}
func (s *state) isLocked(layer ID) bool {
	for n := 0; layer != "" && n <= len(s.Layers); n++ {
		i, ok := s.layer(layer)
		if !ok {
			return true
		}
		if s.Layers[i].Locked {
			return true
		}
		layer = s.Layers[i].Parent
	}
	return layer != ""
}
func validChannel(ch Channel) bool {
	return ch == Albedo || ch == Normal || ch == Height || ch == Emission
}
func validBlend(b Blend) bool {
	return b == BlendNormal || b == BlendMultiply || b == BlendScreen || b == BlendAdd
}
func (s *state) validate(l Limits) error {
	c := s.Canvas
	if c.Width < 1 || c.Height < 1 || c.Width > l.MaxDimension || c.Height > l.MaxDimension || int64(c.Width)*int64(c.Height) > int64(l.MaxCanvasPixels) {
		return ErrLimit
	}
	if c.Mode != RGBA && c.Mode != Grayscale && c.Mode != Indexed {
		return fmt.Errorf("art: invalid color mode")
	}
	if c.Profile != "srgb" {
		return fmt.Errorf("art: unsupported color profile %q", c.Profile)
	}
	if len(s.Layers) < 1 || len(s.Layers) > l.MaxLayers || len(s.Frames) < 1 || len(s.Frames) > l.MaxFrames || len(s.Cels) > l.MaxCels {
		return ErrLimit
	}
	if len(s.Palette) < 1 || len(s.Palette) > 256 {
		return fmt.Errorf("art: palette needs 1..256 entries")
	}
	if c.Mode == Indexed && s.Palette[0].Color.A != 0 {
		return fmt.Errorf("art: indexed entry zero is reserved for transparency")
	}
	ids := make(map[ID]bool)
	add := func(id ID) bool {
		if id == "" || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, v := range s.Layers {
		if !add(v.ID) || len(v.Name) > 1024 || !validBlend(v.Blend) || (v.Kind != Raster && v.Kind != Group && v.Kind != Reference) {
			return fmt.Errorf("art: invalid layer")
		}
		p := v.Parent
		for n := 0; p != ""; n++ {
			if n >= len(s.Layers) || p == v.ID {
				return fmt.Errorf("art: cyclic layer group")
			}
			i, ok := s.layer(p)
			if !ok || s.Layers[i].Kind != Group {
				return fmt.Errorf("art: invalid layer parent")
			}
			p = s.Layers[i].Parent
		}
	}
	for _, v := range s.Frames {
		if !add(v.ID) || v.DurationMS < 1 || v.DurationMS > 600000 {
			return fmt.Errorf("art: invalid frame")
		}
	}
	unique := make(map[[3]string]bool)
	used := make(map[ID]bool)
	imageChannels := make(map[ID]Channel)
	for _, v := range s.Cels {
		li, ok := s.layer(v.Layer)
		if !ok || s.Layers[li].Kind == Group {
			return fmt.Errorf("art: invalid cel layer")
		}
		if _, ok = s.frame(v.Frame); !ok {
			return fmt.Errorf("art: invalid cel frame")
		}
		key := [3]string{string(v.Layer), string(v.Frame), string(v.Channel)}
		if !add(v.ID) || !validChannel(v.Channel) || unique[key] || s.Images[v.Image] == nil {
			return fmt.Errorf("art: invalid or duplicate cel")
		}
		unique[key] = true
		if old, ok := imageChannels[v.Image]; ok && old != v.Channel {
			return fmt.Errorf("art: one image cannot link different surface channels")
		}
		imageChannels[v.Image] = v.Channel
		used[v.Image] = true
		if v.X < -l.MaxDimension || v.X > l.MaxDimension || v.Y < -l.MaxDimension || v.Y > l.MaxDimension {
			return ErrLimit
		}
	}
	for _, v := range s.Tags {
		a, ok := s.frame(v.From)
		b, ok2 := s.frame(v.To)
		if !add(v.ID) || !ok || !ok2 || a > b || len(v.Name) == 0 || len(v.Name) > 1024 || (v.Direction != Forward && v.Direction != Reverse && v.Direction != PingPong) {
			return fmt.Errorf("art: invalid animation tag")
		}
	}
	for _, v := range s.Slices {
		if !add(v.ID) || len(v.Name) > 1024 {
			return fmt.Errorf("art: invalid slice")
		}
		if v.Frame != "" {
			if _, ok := s.frame(v.Frame); !ok {
				return fmt.Errorf("art: invalid slice frame")
			}
		}
	}
	seenTiles := make(map[*tile]bool)
	var bytes int64
	for id, p := range s.Images {
		if !add(id) || !used[id] {
			return fmt.Errorf("art: invalid or orphan image")
		}
		if p.Width < 1 || p.Height < 1 || p.Width > l.MaxDimension || p.Height > l.MaxDimension || int64(p.Width)*int64(p.Height) > int64(l.MaxCanvasPixels) {
			return ErrLimit
		}
		for key, t := range p.Tiles {
			if t == nil || key.X < 0 || key.Y < 0 || key.X*TileSize >= p.Width || key.Y*TileSize >= p.Height {
				return fmt.Errorf("art: invalid pixel tile")
			}
			if !seenTiles[t] {
				seenTiles[t] = true
				bytes += TileSize * TileSize * 4
				if bytes > l.MaxPixelBytes {
					return ErrLimit
				}
			}
		}
	}
	if len(s.Properties) > 256 {
		return ErrLimit
	}
	for k, v := range s.Properties {
		if len(k) > 1024 || len(v) > 65536 {
			return ErrLimit
		}
	}
	return nil
}
