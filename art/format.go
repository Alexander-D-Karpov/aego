package art

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"sort"
	"strconv"
)

const FormatVersion = 1
const SourceExtension = ".aeart"

type imageRecord struct {
	ID     ID     `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	File   string `json:"file"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}
type manifest struct {
	Version            int           `json:"version"`
	Document           Info          `json:"document"`
	Next               uint64        `json:"next"`
	Images             []imageRecord `json:"images"`
	RequiredExtensions []string      `json:"requiredExtensions,omitempty"`
}

func pixelBytes(p *pixels) []byte {
	keys := make([]tileKey, 0, len(p.Tiles))
	for k := range p.Tiles {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Y < keys[j].Y || (keys[i].Y == keys[j].Y && keys[i].X < keys[j].X) })
	data := make([]byte, 4+len(keys)*(8+TileSize*TileSize*4))
	binary.LittleEndian.PutUint32(data, uint32(len(keys)))
	off := 4
	for _, k := range keys {
		binary.LittleEndian.PutUint32(data[off:], uint32(k.X))
		binary.LittleEndian.PutUint32(data[off+4:], uint32(k.Y))
		off += 8
		for _, v := range p.Tiles[k] {
			binary.LittleEndian.PutUint32(data[off:], v)
			off += 4
		}
	}
	return data
}

// Encode writes a versioned, bounded ZIP container. Only CPU data is saved;
// tiles and cels have distinct identities, so linked cels survive reload.
func (d *Document) Encode() ([]byte, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrClosed
	}
	s, id, rev, next, lim := d.current, d.id, d.revision, d.next, d.limits
	d.mu.Unlock()
	m := manifest{Version: FormatVersion, Document: info(id, rev, s), Next: next}
	ids := make([]ID, 0, len(s.Images))
	for id := range s.Images {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, id := range ids {
		p := s.Images[id]
		b := pixelBytes(p)
		hash := sha256.Sum256(b)
		name := "images/" + string(id) + ".bin"
		m.Images = append(m.Images, imageRecord{id, p.Width, p.Height, name, len(b), hex.EncodeToString(hash[:])})
		f, err := z.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err = f.Write(b); err != nil {
			return nil, err
		}
		if int64(out.Len()) > lim.MaxFileBytes {
			return nil, ErrLimit
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	f, err := z.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(b); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if int64(out.Len()) > lim.MaxFileBytes {
		return nil, ErrLimit
	}
	return out.Bytes(), nil
}
func readZip(f *zip.File, limit int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) || f.Mode()&^0777 != 0 {
		return nil, fmt.Errorf("art: unsupported or oversized archive entry")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrLimit
	}
	return data, nil
}
func Decode(data []byte, limits Limits) (*Document, error) {
	l := limitsOrDefault(limits)
	if int64(len(data)) > l.MaxFileBytes {
		return nil, ErrLimit
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("art: invalid source: %w", err)
	}
	if len(z.File) < 1 || len(z.File) > l.MaxCels+1 {
		return nil, ErrLimit
	}
	files := make(map[string]*zip.File)
	var total uint64
	for _, f := range z.File {
		if files[f.Name] != nil {
			return nil, fmt.Errorf("art: duplicate archive entry")
		}
		files[f.Name] = f
		budget := uint64(l.MaxPixelBytes) + (8 << 20)
		if f.UncompressedSize64 > budget-total {
			return nil, ErrLimit
		}
		total += f.UncompressedSize64
	}
	mf := files["manifest.json"]
	if mf == nil {
		return nil, fmt.Errorf("art: manifest is missing")
	}
	raw, err := readZip(mf, 8<<20)
	if err != nil {
		return nil, err
	}
	var m manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("art: invalid manifest: %w", err)
	}
	var trailing any
	if dec.Decode(&trailing) != io.EOF {
		return nil, fmt.Errorf("art: trailing manifest data")
	}
	if m.Version != FormatVersion {
		return nil, fmt.Errorf("art: unsupported version %d", m.Version)
	}
	if len(m.RequiredExtensions) != 0 {
		return nil, fmt.Errorf("art: unsupported required extensions")
	}
	if len(m.Document.ID) != 32 {
		return nil, fmt.Errorf("art: invalid document ID")
	}
	if _, err = hex.DecodeString(m.Document.ID); err != nil {
		return nil, fmt.Errorf("art: invalid document ID")
	}
	i := m.Document
	c := i.Canvas
	if c.Width < 1 || c.Height < 1 || c.Width > l.MaxDimension || c.Height > l.MaxDimension || int64(c.Width)*int64(c.Height) > int64(l.MaxCanvasPixels) {
		return nil, ErrLimit
	}
	if len(m.Images) > l.MaxCels || len(m.Images)+1 != len(files) {
		return nil, fmt.Errorf("art: unexpected archive entries")
	}
	s := &state{Canvas: c, Palette: i.Palette, Layers: i.Layers, Frames: i.Frames, Cels: i.Cels, Tags: i.Tags, Slices: i.Slices, Properties: i.Properties, Images: make(map[ID]*pixels)}
	if len(s.Palette) < 1 || len(s.Palette) > 256 {
		return nil, fmt.Errorf("art: invalid palette")
	}
	// Restore independent COW blocks by content while preserving image IDs. A
	// duplicate image and a linked image therefore remain semantically distinct.
	tiles := make(map[[32]byte]*tile)
	var expanded int64
	for _, rec := range m.Images {
		if !validID(rec.ID) || s.Images[rec.ID] != nil || rec.File != "images/"+string(rec.ID)+".bin" {
			return nil, fmt.Errorf("art: invalid image record")
		}
		f := files[rec.File]
		if f == nil || rec.Size < 4 || int64(rec.Size) > l.MaxPixelBytes || uint64(rec.Size) != f.UncompressedSize64 {
			return nil, fmt.Errorf("art: missing or oversized image")
		}
		if rec.Width < 1 || rec.Height < 1 || rec.Width > l.MaxDimension || rec.Height > l.MaxDimension || int64(rec.Width)*int64(rec.Height) > int64(l.MaxCanvasPixels) {
			return nil, ErrLimit
		}
		b, err := readZip(f, int64(rec.Size))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != rec.SHA256 {
			return nil, fmt.Errorf("art: image checksum mismatch")
		}
		count := int64(binary.LittleEndian.Uint32(b))
		if count > (int64(rec.Width+TileSize-1)/TileSize)*(int64(rec.Height+TileSize-1)/TileSize) || 4+count*(8+TileSize*TileSize*4) != int64(len(b)) {
			return nil, fmt.Errorf("art: invalid tile length")
		}
		imageChannel := Albedo
		for _, cel := range s.Cels {
			if cel.Image == rec.ID {
				imageChannel = cel.Channel
				break
			}
		}
		imageMode := s.forChannel(imageChannel).Canvas.Mode
		p := newPixels(rec.Width, rec.Height)
		off := 4
		for n := int64(0); n < count; n++ {
			x, y := binary.LittleEndian.Uint32(b[off:]), binary.LittleEndian.Uint32(b[off+4:])
			off += 8
			if uint64(x)*TileSize >= uint64(rec.Width) || uint64(y)*TileSize >= uint64(rec.Height) {
				return nil, fmt.Errorf("art: tile outside image")
			}
			key := tileKey{int(x), int(y)}
			if p.Tiles[key] != nil {
				return nil, fmt.Errorf("art: duplicate tile")
			}
			payload := b[off : off+TileSize*TileSize*4]
			hash := sha256.Sum256(payload)
			for k := 0; k < TileSize*TileSize; k++ {
				v := binary.LittleEndian.Uint32(payload[k*4:])
				if imageMode == Indexed && uint64(v) >= uint64(len(s.Palette)) {
					return nil, fmt.Errorf("art: invalid pixel index")
				}
				if imageMode == Grayscale {
					color := unpack(v)
					if color.R != color.G || color.G != color.B {
						return nil, fmt.Errorf("art: invalid grayscale pixel")
					}
				}
			}
			t := tiles[hash]
			if t == nil {
				t = new(tile)
				for k := range t {
					v := binary.LittleEndian.Uint32(payload[k*4:])
					if imageMode == Indexed && uint64(v) >= uint64(len(s.Palette)) {
						return nil, fmt.Errorf("art: invalid pixel index")
					}
					if imageMode == Grayscale {
						color := unpack(v)
						if color.R != color.G || color.G != color.B {
							return nil, fmt.Errorf("art: invalid grayscale pixel")
						}
					}
					t[k] = v
				}
				tiles[hash] = t
				expanded += TileSize * TileSize * 4
				if expanded > l.MaxPixelBytes {
					return nil, ErrLimit
				}
			}
			p.Tiles[key] = t
			off += TileSize * TileSize * 4
		}
		s.Images[rec.ID] = p
	}
	if err = s.validate(l); err != nil {
		return nil, err
	}
	var high uint64
	check := func(id ID) error {
		if !validID(id) {
			return fmt.Errorf("art: invalid entity ID")
		}
		n, _ := strconv.ParseUint(string(id), 16, 64)
		if n > high {
			high = n
		}
		return nil
	}
	for _, x := range s.Layers {
		if err = check(x.ID); err != nil {
			return nil, err
		}
	}
	for _, x := range s.Frames {
		if err = check(x.ID); err != nil {
			return nil, err
		}
	}
	for _, x := range s.Cels {
		if err = check(x.ID); err != nil {
			return nil, err
		}
	}
	for _, x := range s.Tags {
		if err = check(x.ID); err != nil {
			return nil, err
		}
	}
	for _, x := range s.Slices {
		if err = check(x.ID); err != nil {
			return nil, err
		}
	}
	for id := range s.Images {
		if err = check(id); err != nil {
			return nil, err
		}
	}
	if m.Next < high || m.Next > 1<<52 || i.Revision > 1<<52 {
		return nil, fmt.Errorf("art: invalid identity counter")
	}
	return &Document{id: i.ID, revision: max(1, i.Revision), next: m.Next, current: s, limits: l}, nil
}
func validID(id ID) bool {
	if len(id) != 16 {
		return false
	}
	n, err := strconv.ParseUint(string(id), 16, 64)
	return err == nil && n > 0
}

// CheckPNGPrecision refuses source channels that the eight-bit Art model
// cannot represent. Callers can offer a separate, explicitly lossy conversion
// rather than treating this as a lossless open or source migration.
func CheckPNGPrecision(cfg image.Config) error {
	switch cfg.ColorModel {
	case color.Gray16Model, color.RGBA64Model, color.NRGBA64Model:
		return fmt.Errorf("art: 16-bit PNG channels cannot be preserved in an 8-bit Art Document")
	}
	return nil
}

func ImportPNG(data []byte, limits Limits) (*Document, error) {
	l := limitsOrDefault(limits)
	if int64(len(data)) > l.MaxFileBytes {
		return nil, ErrLimit
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := CheckPNGPrecision(cfg); err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > l.MaxDimension || cfg.Height > l.MaxDimension || int64(cfg.Width)*int64(cfg.Height) > int64(l.MaxCanvasPixels) {
		return nil, ErrLimit
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	d, err := New(cfg.Width, cfg.Height, RGBA, l)
	if err != nil {
		return nil, err
	}
	e, err := d.Begin(d.Revision(), "Import PNG")
	if err != nil {
		return nil, err
	}
	cel := e.work.Cels[0]
	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			v := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			if err = e.raw(cel.Image, x, y, Color{v.R, v.G, v.B, v.A}.packed()); err != nil {
				return nil, err
			}
		}
	}
	change, err := e.Prepare()
	if err != nil {
		return nil, err
	}
	if change != nil {
		if err = change.Apply(); err != nil {
			return nil, err
		}
	}
	return d, nil
}
func (d *Document) PNG(frame ID, channel Channel) ([]byte, error) {
	im, err := d.Render(frame, RenderOptions{Channel: channel})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err = png.Encode(&b, im); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

type SheetFrame struct {
	ID         ID              `json:"id"`
	Rect       image.Rectangle `json:"rect"`
	DurationMS int             `json:"durationMS"`
}
type Sheet struct {
	PNG    []byte       `json:"-"`
	Width  int          `json:"width"`
	Height int          `json:"height"`
	Frames []SheetFrame `json:"frames"`
	Tags   []Tag        `json:"tags"`
	Slices []Slice      `json:"slices"`
}

// ExportSheet exports pixels and stable frame IDs. Runtime AssetIDs remain the
// importer's responsibility: this core never fabricates a second AssetID space.
func (d *Document) ExportSheet(columns int, channel Channel) (Sheet, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return Sheet{}, ErrClosed
	}
	s, l := d.current, d.limits
	d.mu.Unlock()
	n := len(s.Frames)
	if columns < 1 || columns > n {
		columns = n
	}
	rows := (n + columns - 1) / columns
	w64, h64 := int64(s.Canvas.Width)*int64(columns), int64(s.Canvas.Height)*int64(rows)
	if w64 > 16384 || h64 > 16384 || w64*h64*4 > l.MaxPixelBytes {
		return Sheet{}, ErrLimit
	}
	w, h := int(w64), int(h64)
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	sheet := Sheet{Width: w, Height: h, Tags: append([]Tag(nil), s.Tags...), Slices: append([]Slice(nil), s.Slices...)}
	for i, f := range s.Frames {
		im := image.NewNRGBA(image.Rect(0, 0, s.Canvas.Width, s.Canvas.Height))
		if err := renderRegion(s, f.ID, im, im.Bounds(), options(RenderOptions{Channel: channel})); err != nil {
			return Sheet{}, err
		}
		x, y := i%columns*s.Canvas.Width, i/columns*s.Canvas.Height
		for row := 0; row < s.Canvas.Height; row++ {
			copy(out.Pix[out.PixOffset(x, y+row):out.PixOffset(x, y+row)+s.Canvas.Width*4], im.Pix[row*im.Stride:row*im.Stride+s.Canvas.Width*4])
		}
		sheet.Frames = append(sheet.Frames, SheetFrame{f.ID, image.Rect(x, y, x+s.Canvas.Width, y+s.Canvas.Height), f.DurationMS})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, out); err != nil {
		return Sheet{}, err
	}
	sheet.PNG = b.Bytes()
	return sheet, nil
}
