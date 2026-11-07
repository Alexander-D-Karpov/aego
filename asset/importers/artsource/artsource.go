// Package artsource prepares editable art using Aego's existing texture and
// animation codecs. It does not introduce a new runtime resource format.
package artsource

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"aego/anim"
	"aego/art"
	"aego/asset"
	"aego/asset/importers/animclip"
	texImporter "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/curve"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/gpu"
	"aego/schema"
)

const Version = 1
const Extension = ".aeart"

type Importer struct{}

func New() *Importer                          { return &Importer{} }
func (*Importer) ID() string                  { return "artsource" }
func (*Importer) Version() int                { return Version }
func (*Importer) Extensions() []string        { return []string{Extension} }
func (*Importer) Defaults() pipeline.Settings { return asset.TextureDefaults() }

// SubName addresses immutable entity IDs, never frame positions or tag names.
// Reordering frames, renaming tags and repainting therefore preserve AssetRefs.
func FrameName(frame art.ID, channel art.Channel) string {
	return "frame_" + string(frame) + "_" + string(channel)
}
func ClipName(tag art.ID) string {
	if tag == "" {
		return "animation"
	}
	return "tag_" + string(tag)
}
func StableID(source ids.AssetID, name string) ids.AssetID {
	sum := sha256.Sum256([]byte("aego.artsource.v1\x00" + source.String() + "\x00" + name))
	id, _ := ids.ParseAssetID(hex.EncodeToString(sum[:16]))
	return id
}

func sourceID(meta *pipeline.Meta, root ids.AssetID, name string) ids.AssetID {
	// Explicit existing metadata always wins, including imported legacy mappings.
	for _, s := range meta.Subs {
		if s.Name == name && !s.ID.IsZero() {
			return s.ID
		}
	}
	return StableID(root, name)
}

func (*Importer) Import(ctx *pipeline.ImportContext) (*pipeline.Result, error) {
	if ctx == nil || ctx.Meta == nil || ctx.ID.IsZero() {
		return nil, fmt.Errorf("artsource: source identity and metadata are required")
	}
	if ctx.Meta.Settings.String("atlas", "") != "" || ctx.Meta.Settings.Bool("trim", false) || ctx.Meta.Settings.Bool("rotate", false) || ctx.Meta.Settings.Bool("mips", false) {
		return nil, fmt.Errorf("artsource: frame sheets cannot use atlas packing, trim, rotation or sheet-wide mipmaps")
	}
	// Reject unsupported external bindings explicitly rather than discarding them.
	for _, key := range []string{"normal", "emission", "height", "normal_frames", "emission_frames", "height_frames"} {
		if _, exists := ctx.Meta.Settings[key]; exists {
			return nil, fmt.Errorf("artsource: %s is an external surface binding; use embedded channels in this source", key)
		}
	}
	if ctx.Meta.Settings.Bool("generate_normal", false) {
		return nil, fmt.Errorf("artsource: generated normals are not supported for embedded frame sheets; paint a normal channel")
	}
	seenNames := map[string]bool{}
	seenIDs := map[ids.AssetID]bool{ctx.ID: true}
	for _, sub := range ctx.Meta.Subs {
		if sub.Name == "" || seenNames[sub.Name] || sub.ID.IsZero() || seenIDs[sub.ID] {
			return nil, fmt.Errorf("artsource: invalid or duplicate source subasset identity")
		}
		seenNames[sub.Name], seenIDs[sub.ID] = true, true
	}
	d, err := art.Decode(ctx.Source, art.DefaultLimits())
	if err != nil {
		return nil, fmt.Errorf("artsource: %w", err)
	}
	defer d.Close()
	snapshot, err := d.Snapshot()
	if err != nil {
		return nil, err
	}
	info := snapshot.Info()
	columns := int(math.Ceil(math.Sqrt(float64(len(info.Frames)))))
	channels := []art.Channel{art.Albedo}
	for _, ch := range []art.Channel{art.Normal, art.Height, art.Emission} {
		for _, c := range info.Cels {
			if c.Channel == ch {
				channels = append(channels, ch)
				break
			}
		}
	}
	type prepared struct {
		page   ids.AssetID
		sheet  art.Sheet
		result *pipeline.Result
	}
	preparedChannels := map[art.Channel]prepared{}
	for _, ch := range channels {
		sheet, err := snapshot.ExportSheet(columns, ch)
		if err != nil {
			return nil, fmt.Errorf("artsource %s sheet: %w", ch, err)
		}
		pageID := ctx.ID
		if ch != art.Albedo {
			pageID = sourceID(ctx.Meta, ctx.ID, "surface_"+string(ch))
		}
		settings := asset.TextureDefaults()
		for k, v := range ctx.Meta.Settings {
			settings[k] = v
		}
		settings["atlas"] = aetext.String("")
		settings["trim"] = aetext.Bool(false)
		settings["auto_normal"] = aetext.Bool(false)
		settings["auto_emission"] = aetext.Bool(false)
		// Bind embedded surfaces after importing; they are not external files.
		for _, key := range []string{"normal", "emission", "height", "normal_frames", "emission_frames", "height_frames"} {
			delete(settings, key)
		}
		role := "color"
		switch ch {
		case art.Normal:
			role = "normal"
		case art.Height:
			role = "height"
		case art.Emission:
			role = "emission-rgb"
		}
		settings["role"] = aetext.String(role)
		settings["semantic"] = aetext.String("auto")
		if ch == art.Normal || ch == art.Height {
			settings["colorspace"] = aetext.String("linear")
		}
		settings["generate_normal"] = aetext.Bool(false)
		meta := &pipeline.Meta{ID: pageID, Importer: "texture", Version: texImporter.Version, Settings: settings}
		local := *ctx
		local.ID, local.Path, local.Source, local.Meta = pageID, ctx.Path+"."+string(ch)+".png", sheet.PNG, meta
		result, err := texImporter.New().Import(&local)
		if err != nil {
			return nil, fmt.Errorf("artsource %s import: %w", ch, err)
		}
		if result.Error != nil {
			return nil, result.Error
		}
		preparedChannels[ch] = prepared{pageID, sheet, result}
	}
	albedo := preparedChannels[art.Albedo]
	result := &pipeline.Result{Kind: asset.ArtTexture}
	for _, a := range albedo.result.Artifacts {
		if a.Kind == asset.ArtTexture {
			result.Artifacts = append(result.Artifacts, a)
		}
	}
	filter := gpu.FilterNearest
	if ctx.Meta.Settings.String("filter", "nearest") == "linear" {
		filter = gpu.FilterLinear
	}
	frameIDs := map[art.ID]ids.AssetID{}
	for _, ch := range channels {
		p := preparedChannels[ch]
		if ch != art.Albedo {
			var textures []pipeline.Produced
			for _, a := range p.result.Artifacts {
				if a.Kind == asset.ArtTexture {
					textures = append(textures, a)
				}
			}
			result.Subs = append(result.Subs, pipeline.SubResult{ID: p.page, Name: "surface_" + string(ch), Kind: asset.ArtTexture, Artifacts: textures})
		}
		for i, f := range p.sheet.Frames {
			name := FrameName(f.ID, ch)
			id := sourceID(ctx.Meta, ctx.ID, name)
			region := &asset.RegionArtifact{Page: p.page, X: f.Rect.Min.X, Y: f.Rect.Min.Y, W: f.Rect.Dx(), H: f.Rect.Dy(), PageW: p.sheet.Width, PageH: p.sheet.Height, Filter: filter, Pivot: ctx.Meta.Pivot, HasPivot: ctx.Meta.HasPivot}
			for _, sl := range info.Slices {
				if sl.Name == "pivot" && (sl.Frame == "" || sl.Frame == f.ID) {
					region.Pivot = [2]float32{float32(sl.Pivot.X) / float32(info.Canvas.Width), float32(sl.Pivot.Y) / float32(info.Canvas.Height)}
					region.HasPivot = true
				}
			}
			deps := []ids.AssetID{p.page}
			if ch == art.Albedo {
				frameIDs[f.ID] = id
				if _, ok := preparedChannels[art.Normal]; ok {
					region.Normal = sourceID(ctx.Meta, ctx.ID, FrameName(f.ID, art.Normal))
					deps = append(deps, region.Normal)
				}
				if _, ok := preparedChannels[art.Emission]; ok {
					region.Emission = sourceID(ctx.Meta, ctx.ID, FrameName(f.ID, art.Emission))
					deps = append(deps, region.Emission)
				}
			}
			bytes := asset.EncodeRegion(region)
			result.Subs = append(result.Subs, pipeline.SubResult{ID: id, Name: name, Kind: asset.ArtSpriteRegion, Deps: deps, Artifacts: []pipeline.Produced{{Kind: asset.ArtSpriteRegion, Data: bytes}}})
			if ch == art.Albedo && i == 0 {
				// A SpriteRenderer assigned to the source shows its first frame,
				// not the entire sheet; existing source AssetID remains unchanged.
				result.Artifacts = append(result.Artifacts, pipeline.Produced{Kind: asset.ArtSpriteRegion, Name: ctx.ID.String() + ".aereg", Data: bytes})
			}
		}
	}
	tags := []art.ID{""}
	for _, tag := range info.Tags {
		tags = append(tags, tag.ID)
	}
	for _, tag := range tags {
		text, err := ClipText(snapshot, tag, frameIDs)
		if err != nil {
			return nil, err
		}
		name := ClipName(tag)
		clipID := sourceID(ctx.Meta, ctx.ID, name)
		local := *ctx
		local.ID, local.Path, local.Source = clipID, ctx.Path+"#"+name+".aeanim", text
		clip, err := animclip.New().Import(&local)
		if err != nil {
			return nil, err
		}
		if clip.Error != nil {
			return nil, clip.Error
		}
		result.Subs = append(result.Subs, pipeline.SubResult{ID: clipID, Name: name, Kind: asset.ArtAnim, Deps: clip.Deps, Artifacts: clip.Artifacts})
	}
	used := map[ids.AssetID]bool{ctx.ID: true}
	for _, sub := range result.Subs {
		if used[sub.ID] {
			return nil, fmt.Errorf("artsource: prepared subasset identity collision: %s", sub.Name)
		}
		used[sub.ID] = true
	}
	return result, nil
}

// ClipText uses the engine's codec and variable-duration step keys. The tag
// order is expanded once, so Ping-pong never doubles its endpoint holds.
func ClipText(snapshot *art.Snapshot, tag art.ID, frames map[art.ID]ids.AssetID) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("artsource: snapshot required")
	}
	order, err := snapshot.PlaybackFrames(tag)
	if err != nil {
		return nil, err
	}
	clip := &anim.Clip{Loop: anim.LoopRepeat}
	track := anim.Track{Property: anim.SpriteProp, Kind: schema.KindAssetRef}
	var ms int64
	for _, frame := range order {
		id := frames[frame.ID]
		if id.IsZero() {
			return nil, fmt.Errorf("artsource: unresolved frame %s", frame.ID)
		}
		track.Keys = append(track.Keys, anim.Key{T: float32(float64(ms) / 1000), V: anim.Asset(id), Interp: curve.InterpStep})
		ms += int64(frame.DurationMS)
	}
	clip.Length = float32(float64(ms) / 1000)
	clip.Tracks = []anim.Track{track}
	if problems := clip.Check(nil); len(problems) != 0 {
		return nil, fmt.Errorf("artsource: invalid generated animation: %+v", problems[0])
	}
	return aetext.Write(anim.Encode(clip)), nil
}
