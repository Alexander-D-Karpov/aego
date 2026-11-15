package artsource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"path"
	"slices"
	"strconv"
	"strings"

	"aego/art"
	"aego/asset"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/aetext"
)

// TextureLink binds a PNG's existing region IDs to an editable art source.
// The PNG is never overwritten; its prepared texture is derived from art.
type TextureLink struct {
	Source       ids.AssetID
	SHA256       string
	LayoutSHA256 string // Empty for legacy links created before geometry guards.
	Frames       []TextureLinkFrame
}

type TextureLinkFrame struct {
	Asset ids.AssetID
	Frame art.ID
}

func TextureLinkOf(meta *pipeline.Meta) (*TextureLink, error) {
	if meta == nil {
		return nil, fmt.Errorf("artsource: texture metadata is required")
	}
	v, exists := meta.Settings["art_source"]
	if !exists {
		if _, ok := meta.Settings["art_frames"]; ok {
			return nil, fmt.Errorf("artsource: art_frames has no art_source")
		}
		if _, ok := meta.Settings["art_source_layout_sha256"]; ok {
			return nil, fmt.Errorf("artsource: art_source_layout_sha256 has no art_source")
		}
		return nil, nil
	}
	if v.Kind != aetext.VAsset {
		return nil, fmt.Errorf("artsource: art_source must be an AssetRef")
	}
	id, err := ids.ParseAssetID(v.Str)
	if err != nil || id.IsZero() || id == meta.ID {
		return nil, fmt.Errorf("artsource: invalid art_source identity")
	}
	hash := meta.Settings.String("art_source_sha256", "")
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("artsource: invalid original PNG hash")
	}
	value, ok := meta.Settings["art_frames"]
	if !ok || value.Kind != aetext.VList || len(value.List) == 0 || len(value.List) > art.DefaultLimits().MaxFrames {
		return nil, fmt.Errorf("artsource: a bounded art_frames mapping is required")
	}
	link := &TextureLink{Source: id, SHA256: strings.ToLower(hash)}
	if value, ok := meta.Settings["art_source_layout_sha256"]; ok {
		if value.Kind != aetext.VString {
			return nil, fmt.Errorf("artsource: invalid original PNG slicing hash")
		}
		decoded, err := hex.DecodeString(value.Str)
		if err != nil || len(decoded) != sha256.Size || value.Str != strings.ToLower(value.Str) {
			return nil, fmt.Errorf("artsource: invalid original PNG slicing hash")
		}
		link.LayoutSHA256 = value.Str
	}
	seenAssets := map[ids.AssetID]bool{}
	seenFrames := map[art.ID]bool{}
	for _, item := range value.List {
		if item.Kind != aetext.VStruct || len(item.Obj) != 2 {
			return nil, fmt.Errorf("artsource: invalid art_frames entry")
		}
		var frame TextureLinkFrame
		for _, field := range item.Obj {
			switch field.Name {
			case "asset":
				if field.Value.Kind != aetext.VAsset || !frame.Asset.IsZero() {
					return nil, fmt.Errorf("artsource: invalid art_frames AssetRef")
				}
				frame.Asset, err = ids.ParseAssetID(field.Value.Str)
				if err != nil || frame.Asset.IsZero() {
					return nil, fmt.Errorf("artsource: invalid art_frames AssetRef")
				}
			case "frame":
				if field.Value.Kind != aetext.VString || frame.Frame != "" {
					return nil, fmt.Errorf("artsource: invalid art frame identity")
				}
				frame.Frame = art.ID(field.Value.Str)
				counter, err := strconv.ParseUint(string(frame.Frame), 16, 64)
				if err != nil || len(frame.Frame) != 16 || counter == 0 {
					return nil, fmt.Errorf("artsource: invalid art frame identity")
				}
			default:
				return nil, fmt.Errorf("artsource: unknown art_frames field %q", field.Name)
			}
		}
		if frame.Asset.IsZero() || frame.Frame == "" || seenAssets[frame.Asset] || seenFrames[frame.Frame] {
			return nil, fmt.Errorf("artsource: incomplete or duplicate art_frames mapping")
		}
		// Each original source region has its own authoring frame ID. Linked
		// cels may share their image, but reusing a frame ID here would silently
		// replace two distinct original regions with the same rendered frame.
		seenAssets[frame.Asset], seenFrames[frame.Frame] = true, true
		link.Frames = append(link.Frames, frame)
	}
	if link.LayoutSHA256 != "" && link.LayoutSHA256 != textureLayoutSHA256(meta, link.Frames) {
		return nil, fmt.Errorf("artsource: PNG slicing changed or frame mapping changed after linking; resolve the mapping before importing")
	}
	return link, nil
}

// textureLayoutSHA256 protects both the original asset-to-rectangle and
// asset-to-authoring-frame correspondences, not cosmetic names or pivots.
// PNG bytes (and whole-image dimensions) are protected separately. Sorting
// by ID permits reordering metadata without changing the mapping. This is a
// content check, not an atomic CAS or proof against replacing the hash itself.
func textureLayoutSHA256(meta *pipeline.Meta, links []TextureLinkFrame) string {
	frames := slices.Clone(meta.Subs)
	slices.SortFunc(frames, func(a, b pipeline.SubAsset) int {
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	mapping := slices.Clone(links)
	slices.SortFunc(mapping, func(a, b TextureLinkFrame) int {
		return strings.Compare(a.Asset.String(), b.Asset.String())
	})
	h := sha256.New()
	fmt.Fprintf(h, "aego-linked-texture-layout-v1\n%s\n%d\n", meta.ID.String(), len(frames))
	for _, frame := range frames {
		fmt.Fprintf(h, "%s %d %d %d %d\n", frame.ID.String(), frame.Rect[0], frame.Rect[1], frame.Rect[2], frame.Rect[3])
	}
	fmt.Fprintf(h, "%d\n", len(mapping))
	for _, frame := range mapping {
		fmt.Fprintf(h, "%s %s\n", frame.Asset.String(), frame.Frame)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func SetTextureLink(meta *pipeline.Meta, source ids.AssetID, original []byte, frames []TextureLinkFrame) error {
	if meta == nil || meta.Settings == nil {
		return fmt.Errorf("artsource: texture metadata is required")
	}
	hash := sha256.Sum256(original)
	items := make([]aetext.Value, 0, len(frames))
	for _, f := range frames {
		items = append(items, aetext.Struct(
			aetext.Field{Name: "asset", Value: aetext.AssetRef(f.Asset.String())},
			aetext.Field{Name: "frame", Value: aetext.String(string(f.Frame))},
		))
	}
	updated := *meta
	updated.Settings = meta.Settings.Clone()
	updated.Settings["art_source"] = aetext.AssetRef(source.String())
	updated.Settings["art_source_sha256"] = aetext.String(hex.EncodeToString(hash[:]))
	updated.Settings["art_source_layout_sha256"] = aetext.String(textureLayoutSHA256(&updated, frames))
	updated.Settings["art_frames"] = aetext.List(items...)
	if _, err := TextureLinkOf(&updated); err != nil {
		return err
	}
	meta.Settings = updated.Settings
	return nil
}

// LinkedTextureImporter prepares linked art through the existing PNG importer;
// unlinked images keep its original behavior.
type LinkedTextureImporter struct{ *texImporter.Importer }

func NewTexture() *LinkedTextureImporter    { return &LinkedTextureImporter{texImporter.New()} }
func (*LinkedTextureImporter) Version() int { return texImporter.Version + 1 }

type linkedTextureSource struct {
	link     *TextureLink
	document *art.Document
	meta     *pipeline.Meta
	name     string
	encoded  []byte
	metaData []byte
}

func readLinkedTexture(ctx *pipeline.ImportContext) (*linkedTextureSource, error) {
	link, err := TextureLinkOf(ctx.Meta)
	if err != nil || link == nil {
		return nil, err
	}
	hash := sha256.Sum256(ctx.Source)
	if hex.EncodeToString(hash[:]) != link.SHA256 {
		return nil, fmt.Errorf("artsource: original PNG %s changed outside Asset Studio; resolve the source conflict before importing", ctx.Path)
	}
	if ctx.SourcePath == nil || ctx.Read == nil {
		return nil, fmt.Errorf("artsource: linked source resolver is unavailable")
	}
	name, ok := ctx.SourcePath(link.Source)
	if !ok || strings.Contains(name, pipeline.SubSep) || !strings.EqualFold(path.Ext(name), Extension) {
		return nil, fmt.Errorf("artsource: linked Art Document is unavailable")
	}
	encoded, err := ctx.Read(name)
	if err != nil {
		return nil, err
	}
	metaData, err := ctx.Read(name + pipeline.MetaSuffix)
	if err != nil {
		return nil, err
	}
	meta, err := pipeline.ParseMeta(metaData, name+pipeline.MetaSuffix)
	if err != nil {
		return nil, err
	}
	if meta.ID != link.Source || meta.Importer != "artsource" {
		return nil, fmt.Errorf("artsource: linked Art Document metadata changed identity")
	}
	d, err := art.Decode(encoded, art.DefaultLimits())
	if err != nil {
		return nil, fmt.Errorf("artsource: cannot read linked source %s: %w", name, err)
	}
	return &linkedTextureSource{link, d, meta, name, encoded, metaData}, nil
}

func (i *LinkedTextureImporter) CacheSalt(ctx *pipeline.ImportContext) ([]byte, error) {
	base, err := i.Importer.CacheSalt(ctx)
	if err != nil {
		return nil, err
	}
	linked, err := readLinkedTexture(ctx)
	if err != nil || linked == nil {
		return base, err
	}
	defer linked.document.Close()
	h := sha256.New()
	h.Write(base)
	h.Write([]byte(linked.name))
	h.Write(linked.encoded)
	h.Write(linked.metaData)
	return h.Sum(nil), nil
}

func (i *LinkedTextureImporter) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	linked, err := readLinkedTexture(ctx)
	if err != nil {
		return nil, err
	}
	if linked == nil {
		return i.Importer.Import(ctx)
	}
	defer linked.document.Close()
	pixels, err := decodeLinkedPNG(ctx.Source)
	if err != nil {
		return nil, err
	}
	info := linked.document.Info()
	frames := textureSourceFrames(ctx.Meta, pixels.Bounds())
	if len(frames) != len(linked.link.Frames) {
		return nil, fmt.Errorf("artsource: PNG slicing changed; resolve the source/frame mapping before importing")
	}
	byAsset := map[ids.AssetID]art.ID{}
	for _, f := range linked.link.Frames {
		byAsset[f.Asset] = f.Frame
	}
	for _, f := range frames {
		frame := byAsset[f.ID]
		if frame == "" {
			return nil, fmt.Errorf("artsource: original frame %s has no authoring frame", f.Name)
		}
		r := sourceFrameRect(f)
		if r.Empty() || !r.In(pixels.Bounds()) || r.Dx() > info.Canvas.Width || r.Dy() > info.Canvas.Height {
			return nil, fmt.Errorf("artsource: original frame %s no longer fits its art canvas", f.Name)
		}
		framePixels, err := linked.document.Render(frame, art.RenderOptions{Channel: art.Albedo})
		if err != nil {
			return nil, fmt.Errorf("artsource: linked frame %s: %w", f.Name, err)
		}
		// Never silently crop fresh pixels outside an original source region.
		for y := 0; y < framePixels.Bounds().Dy(); y++ {
			for x := 0; x < framePixels.Bounds().Dx(); x++ {
				if (x >= r.Dx() || y >= r.Dy()) && framePixels.NRGBAAt(x, y).A != 0 {
					return nil, fmt.Errorf("artsource: frame %s has pixels outside its original source bounds", f.Name)
				}
			}
		}
		draw.Draw(pixels, r, framePixels, image.Point{}, draw.Src)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		return nil, err
	}
	local := *ctx
	local.Source = encoded.Bytes()
	// Preserve the exact legacy fractional pivot unless the author explicitly
	// moves the frame pivot; never rewrite the PNG metadata here.
	local.Meta, err = pipeline.ParseMeta(pipeline.WriteMeta(ctx.Meta), ctx.Path+pipeline.MetaSuffix)
	if err != nil {
		return nil, err
	}
	for _, original := range frames {
		frame := byAsset[original.ID]
		rect := sourceFrameRect(original)
		pivot := original.EffectivePivot()
		baseline := normalizedPivotToPixels(pivot, rect.Dx(), rect.Dy())
		authored, hasPivot := info.FramePivot(frame)
		if !hasPivot || authored == baseline {
			continue
		}
		pivot = pixelPivotToNormalized(authored, rect.Dx(), rect.Dy())
		if original.ID == local.Meta.ID && len(local.Meta.Subs) == 0 {
			local.Meta.Pivot, local.Meta.HasPivot = pivot, true
		}
		for n := range local.Meta.Subs {
			if local.Meta.Subs[n].ID == original.ID {
				local.Meta.Subs[n].Pivot, local.Meta.Subs[n].HasPivot = pivot, true
			}
		}
	}
	result, err := i.Importer.Import(&local)
	if err != nil || result == nil || result.Error != nil {
		return result, err
	}
	result.Deps = appendUniqueAsset(result.Deps, linked.link.Source)
	channels := map[art.Channel]bool{}
	for _, c := range info.Cels {
		channels[c.Channel] = true
	}
	bind := func(frame art.ID, region *asset.RegionArtifact, deps *[]ids.AssetID) {
		if frame == "" {
			return
		}
		if channels[art.Normal] {
			region.Normal = sourceID(linked.meta, linked.link.Source, FrameName(frame, art.Normal))
			*deps = appendUniqueAsset(*deps, region.Normal)
		}
		if channels[art.Emission] {
			region.Emission = sourceID(linked.meta, linked.link.Source, FrameName(frame, art.Emission))
			*deps = appendUniqueAsset(*deps, region.Emission)
		}
	}
	if frame := byAsset[ctx.ID]; frame != "" {
		if result.Group != "" {
			r := &asset.RegionArtifact{Normal: result.RawNormal, Emission: result.RawEmission}
			bind(frame, r, &result.Deps)
			result.RawNormal, result.RawEmission = r.Normal, r.Emission
		}
		if err := bindRegionArtifacts(result.Artifacts, frame, &result.Deps, bind); err != nil {
			return nil, err
		}
	}
	for n := range result.Subs {
		sub := &result.Subs[n]
		if err := bindRegionArtifacts(sub.Artifacts, byAsset[sub.ID], &sub.Deps, bind); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func bindRegionArtifacts(artifacts []pipeline.Produced, frame art.ID, deps *[]ids.AssetID, bind func(art.ID, *asset.RegionArtifact, *[]ids.AssetID)) error {
	if frame == "" {
		return nil
	}
	for n := range artifacts {
		a := &artifacts[n]
		if a.Kind != asset.ArtSpriteRegion {
			continue
		}
		region, err := asset.DecodeRegion(a.Data)
		if err != nil {
			return err
		}
		bind(frame, region, deps)
		a.Data = asset.EncodeRegion(region)
	}
	return nil
}

func appendUniqueAsset(list []ids.AssetID, id ids.AssetID) []ids.AssetID {
	if !id.IsZero() && !slices.Contains(list, id) {
		return append(list, id)
	}
	return list
}

// Art Core stores eight bits per channel. Do not silently reduce the source
// precision when creating a new editable document, including its surface maps.
// Existing linked imports continue to use decodeLinkedPNG for compatibility.
func decodeEditableTexturePNG(data []byte) (*image.NRGBA, error) {
	if int64(len(data)) > art.DefaultLimits().MaxFileBytes {
		return nil, art.ErrLimit
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := art.CheckPNGPrecision(config); err != nil {
		return nil, fmt.Errorf("artsource: %w", err)
	}
	return decodeTexturePNG(data, true)
}

func decodeLinkedPNG(data []byte) (*image.NRGBA, error) {
	return decodeTexturePNG(data, false)
}

func decodeTexturePNG(data []byte, preservePalette bool) (*image.NRGBA, error) {
	lim := art.DefaultLimits()
	if int64(len(data)) > lim.MaxFileBytes {
		return nil, art.ErrLimit
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width < 1 || config.Height < 1 || config.Width > lim.MaxDimension || config.Height > lim.MaxDimension || int64(config.Width)*int64(config.Height) > int64(lim.MaxCanvasPixels) {
		return nil, art.ErrLimit
	}
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	pixels := image.NewNRGBA(source.Bounds())
	if indexed, ok := source.(*image.Paletted); ok && preservePalette {
		// draw.Draw converts a paletted source through premultiplied RGBA64,
		// which discards RGB under zero alpha and may round partial-alpha
		// entries. Preserve the PNG palette's straight channels in Art Core.
		bounds := indexed.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				pixels.SetNRGBA(x, y, color.NRGBAModel.Convert(indexed.At(x, y)).(color.NRGBA))
			}
		}
		return pixels, nil
	}
	draw.Draw(pixels, pixels.Bounds(), source, source.Bounds().Min, draw.Src)
	return pixels, nil
}

func textureSourceFrames(meta *pipeline.Meta, bounds image.Rectangle) []pipeline.SubAsset {
	if len(meta.Subs) != 0 {
		return meta.Subs
	}
	return []pipeline.SubAsset{{ID: meta.ID, Name: "sprite", Rect: [4]int{0, 0, bounds.Dx(), bounds.Dy()}, Pivot: meta.Pivot, HasPivot: meta.HasPivot}}
}

func sourceFrameRect(f pipeline.SubAsset) image.Rectangle {
	return image.Rect(f.Rect[0], f.Rect[1], f.Rect[0]+f.Rect[2], f.Rect[1]+f.Rect[3])
}

var _ pipeline.Importer = (*LinkedTextureImporter)(nil)
var _ pipeline.SourceDependencySalter = (*LinkedTextureImporter)(nil)
