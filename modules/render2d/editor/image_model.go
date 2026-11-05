package editor

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"io"
	"math"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/gpu"
	"aego/render/texture"
)

const (
	maxViewerPixels = 16 << 20
	maxViewerSource = 32 << 20
)

type imageChannel uint8

const (
	imageRGBA imageChannel = iota
	imageRGB
	imageR
	imageG
	imageB
	imageA
)

var imageChannelNames = []string{"RGBA", "RGB", "R", "G", "B", "A"}

// imagePixels always retains the original decoded bytes. Channel projection
// and premultiplication produce a separate UI texture, never edit this buffer.
type imagePixels struct {
	data     texture.Data
	semantic asset.TextureSemantic
}

func readImageSource(docs *document.Manager, name string) ([]byte, error) {
	if docs == nil {
		return nil, fmt.Errorf("image source requires a document manager")
	}
	f, err := docs.OpenSource(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if info.Size() > maxViewerSource {
		return nil, fmt.Errorf("image source exceeds the viewer's 32 MiB encoded-byte limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxViewerSource+1))
	if err == nil && len(data) > maxViewerSource {
		return nil, fmt.Errorf("image source exceeds the viewer's 32 MiB encoded-byte limit")
	}
	return data, err
}

func decodeImageSource(source []byte) (*imagePixels, error) {
	if len(source) > maxViewerSource {
		return nil, fmt.Errorf("image source exceeds the viewer's 32 MiB encoded-byte limit")
	}
	config, err := png.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 65535 || config.Height > 65535 || config.Width > maxViewerPixels/config.Height {
		return nil, fmt.Errorf("image exceeds the viewer's 16 megapixel limit")
	}
	data, err := texture.DecodePNG(bytes.NewReader(source), true)
	if err != nil {
		return nil, err
	}
	return &imagePixels{data: *data, semantic: asset.TextureColor}, nil
}

func decodeImageArtifact(encoded []byte) (*imagePixels, error) {
	v, err := asset.DecodeTexture(encoded)
	if err != nil {
		return nil, err
	}
	if v.Width > maxViewerPixels/v.Height {
		return nil, fmt.Errorf("imported image exceeds the viewer's 16 megapixel limit")
	}
	return &imagePixels{data: texture.Data{Width: v.Width, Height: v.Height, Format: v.Format, Pixels: v.Pixels, Premultiplied: v.Semantic == asset.TextureColor, NonColor: v.Semantic != asset.TextureColor}, semantic: v.Semantic}, nil
}

func (p *imagePixels) pixel(x, y int) ([4]byte, bool) {
	if p == nil || x < 0 || y < 0 || x >= p.data.Width || y >= p.data.Height {
		return [4]byte{}, false
	}
	i := (y*p.data.Width + x) * 4
	return [4]byte{p.data.Pixels[i], p.data.Pixels[i+1], p.data.Pixels[i+2], p.data.Pixels[i+3]}, true
}

// projection displays encoded channels: color RGBA keeps its PMA/color-space contract, scalar/RGB ignore coverage to expose hidden pixels, and readout uses p.data, not the projection.
func (p *imagePixels) projection(channel imageChannel) (*texture.Data, error) {
	if p == nil || channel > imageA {
		return nil, fmt.Errorf("image channel is unavailable")
	}
	d := texture.New(p.data.Width, p.data.Height, gpu.TexSRGBA8)
	copy(d.Pixels, p.data.Pixels)
	if channel == imageRGBA {
		if p.semantic == asset.TextureColor {
			d.Format = p.data.Format
			d.Premultiplied = p.data.Premultiplied
		}
		d.Premultiply()
		return d, nil
	}
	for i := 0; i < len(d.Pixels); i += 4 {
		if channel >= imageR {
			v := p.data.Pixels[i+int(channel-imageR)]
			d.Pixels[i], d.Pixels[i+1], d.Pixels[i+2] = v, v, v
		}
		d.Pixels[i+3] = 255
	}
	d.Premultiplied = true
	return d, nil
}

type imageViewport struct {
	Center mathx.Vec2
	Zoom   float32 // logical UI units per source texel; 1:1 accounts for UI.Scale.
	Ready  bool
}

func (v *imageViewport) fit(area mathx.Rect, w, h int) {
	if w <= 0 || h <= 0 || area.W <= 0 || area.H <= 0 {
		return
	}
	v.Center = mathx.Vec2{X: float32(w) * .5, Y: float32(h) * .5}
	v.Zoom = max(float32(1.0/1024), min(area.W/float32(w), area.H/float32(h))*.94)
	v.Ready = true
}
func (v *imageViewport) oneToOne(area mathx.Rect, w, h int, scale float32) {
	v.fit(area, w, h)
	v.Zoom = 1 / max(scale, .01)
}
func (v imageViewport) imagePoint(area mathx.Rect, mouse mathx.Vec2) mathx.Vec2 {
	return v.Center.Add(mouse.Sub(mathx.Vec2{X: area.X + area.W*.5, Y: area.Y + area.H*.5}).Scale(1 / max(v.Zoom, 1.0/1024)))
}
func (v imageViewport) screenPoint(area mathx.Rect, p mathx.Vec2) mathx.Vec2 {
	return mathx.Vec2{X: area.X + area.W*.5, Y: area.Y + area.H*.5}.Add(p.Sub(v.Center).Scale(v.Zoom))
}
func (v *imageViewport) zoomAt(area mathx.Rect, mouse mathx.Vec2, wheel float32) {
	before := v.imagePoint(area, mouse)
	v.Zoom = mathx.Clamp(v.Zoom*float32(math.Pow(1.2, float64(wheel))), 1.0/1024, 256)
	after := v.imagePoint(area, mouse)
	v.Center = v.Center.Add(before.Sub(after))
}
func (v *imageViewport) pan(delta mathx.Vec2) {
	v.Center = v.Center.Sub(delta.Scale(1 / max(v.Zoom, 1.0/1024)))
}
func (v imageViewport) rect(area mathx.Rect, w, h int) mathx.Rect {
	p := v.screenPoint(area, mathx.Vec2{})
	return mathx.Rect{X: p.X, Y: p.Y, W: float32(w) * v.Zoom, H: float32(h) * v.Zoom}
}

type imageContent struct {
	Source, Imported           *imagePixels
	SourceID, Selected, Page   ids.AssetID
	Path                       string
	SourceRect                 [4]int
	Region                     asset.RegionArtifact
	Frames                     []pipeline.SubAsset
	SourceError, ImportedError string
	Dependencies, Consumers    []ids.AssetID
	SourceHash                 [32]byte
}

func imageSourcePath(ed *context.Editor, id ids.AssetID) (string, ids.AssetID) {
	if ed.Pipeline == nil {
		return "", id
	}
	p, ok := ed.Pipeline.PathOf(id)
	if !ok {
		return "", id
	}
	file, _, _ := strings.Cut(p, pipeline.SubSep)
	root, ok := ed.Pipeline.Lookup(file)
	if !ok {
		return file, id
	}
	return file, root
}

func loadImageContent(ed *context.Editor, id, selected ids.AssetID) imageContent {
	if selected.IsZero() {
		selected = id
	}
	out := imageContent{SourceID: id, Selected: selected}
	out.Path, out.SourceID = imageSourcePath(ed, id)
	if out.Path != "" && ed.Docs != nil {
		data, err := readImageSource(ed.Docs, out.Path)
		if err == nil {
			out.SourceHash = sha256.Sum256(data)
			out.Source, err = decodeImageSource(data)
		}
		if err != nil {
			out.SourceError = err.Error()
		}
	} else if data, ok := asset.BuiltinTexture(id); ok {
		copyData := *data
		copyData.Pixels = bytes.Clone(data.Pixels)
		out.Source = &imagePixels{data: copyData, semantic: asset.TextureColor}
		out.SourceHash = sha256.Sum256(copyData.Pixels)
		if data.NonColor {
			out.Source.semantic = asset.TextureData
		}
	} else {
		out.SourceError = "this derived or unavailable asset has no image source"
	}
	if out.Source != nil {
		out.SourceRect = [4]int{0, 0, out.Source.data.Width, out.Source.data.Height}
	}
	if ed.Pipeline != nil {
		if meta, ok := ed.Pipeline.MetaOf(out.SourceID); ok {
			out.Frames = meta.Subs
			for _, frame := range meta.Subs {
				if frame.ID == selected {
					out.SourceRect = frame.Rect
					break
				}
			}
		}
	}
	if ed.Loader == nil || ed.Loader.Index() == nil {
		out.ImportedError = "this session has no runtime asset loader"
		return out
	}
	index := ed.Loader.Index()
	entry, ok := index.Lookup(selected)
	if !ok {
		if out.Source != nil && asset.IsBuiltin(id) {
			out.Imported = out.Source
		} else {
			out.ImportedError = "no committed imported artifact is available"
		}
		return out
	}
	out.Dependencies = append(out.Dependencies, entry.Deps...)
	index.Each(func(e *asset.Entry) {
		for _, dep := range e.Deps {
			if dep == selected || dep == out.SourceID {
				out.Consumers = append(out.Consumers, e.ID)
				break
			}
		}
	})
	page := entry
	if region, ok := entry.Artifact(asset.ArtSpriteRegion); ok {
		data, err := ed.Loader.Artifacts().ReadFile(region.Path)
		var r *asset.RegionArtifact
		if err == nil {
			r, err = asset.DecodeRegion(data)
		}
		if err != nil {
			out.ImportedError = err.Error()
			return out
		}
		out.Region = *r
		if p, found := index.Lookup(r.Page); found {
			page = p
		} else {
			out.ImportedError = "sprite atlas page is missing"
			return out
		}
	}
	out.Page = page.ID
	art, ok := page.Artifact(page.Kind)
	if !ok || (page.Kind != asset.ArtTexture && page.Kind != asset.ArtAtlasPage) {
		out.ImportedError = "committed resource has no compatible texture artifact"
		return out
	}
	data, err := ed.Loader.Artifacts().ReadFile(art.Path)
	if err == nil {
		out.Imported, err = decodeImageArtifact(data)
	}
	if err != nil {
		out.ImportedError = err.Error()
		return out
	}
	if out.Region.W == 0 {
		out.Region = asset.RegionArtifact{Page: page.ID, W: out.Imported.data.Width, H: out.Imported.data.Height, PageW: out.Imported.data.Width, PageH: out.Imported.data.Height}
	}
	return out
}
