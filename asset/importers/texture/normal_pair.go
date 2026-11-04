package texture

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"path"
	"slices"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
)

func assetSetting(s pipeline.Settings, key string) (ids.AssetID, bool, error) {
	v, ok := s[key]
	if !ok {
		return ids.AssetID{}, false, nil
	}
	if v.Kind != aetext.VAsset {
		return ids.AssetID{}, true, fmt.Errorf("%s must be an asset(ID) reference", key)
	}
	id, err := ids.ParseAssetID(v.Str)
	return id, true, err
}
func readSurfaceMeta(ctx *pipeline.ImportContext, id ids.AssetID) (string, *pipeline.Meta, error) {
	if id.IsZero() || ctx.SourcePath == nil || ctx.Read == nil {
		return "", nil, nil
	}
	name, ok := ctx.SourcePath(id)
	if !ok {
		return "", nil, nil
	}
	source, _, _ := strings.Cut(name, pipeline.SubSep)
	encoded, err := ctx.Read(source + pipeline.MetaSuffix)
	if err != nil {
		return "", nil, err
	}
	m, err := pipeline.ParseMeta(encoded, source+pipeline.MetaSuffix)
	return source, m, err
}
func sourceInfo(ctx *pipeline.ImportContext, id ids.AssetID) (asset.TextureSourceInfo, bool, error) {
	name, m, err := readSurfaceMeta(ctx, id)
	if err != nil || m == nil {
		return asset.TextureSourceInfo{}, false, err
	}
	b, err := ctx.Read(name)
	if err != nil {
		return asset.TextureSourceInfo{}, false, err
	}
	c, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return asset.TextureSourceInfo{}, false, err
	}
	info, ok := asset.TextureSource(m, name, c.Width, c.Height, id)
	return info, ok, nil
}

// ResolveMetadata persists discovered source and frame IDs once. Explicit
// zero/unknown mappings, renamed frames and custom settings always survive.
func (Importer) ResolveMetadata(ctx *pipeline.ImportContext) (pipeline.Settings, error) {
	original := ctx.Meta.Settings
	role, err := asset.TextureRoleOf(ctx.Path, original)
	if err != nil {
		return nil, err
	}
	updated := original.Clone()
	changed := false
	if original.String("role", "auto") == "auto" && original.String("semantic", "auto") == "auto" && role != asset.RoleColor {
		updated["semantic"] = aetext.Ident(string(role))
		changed = true
	}
	if role != asset.RoleColor {
		if changed {
			return updated, nil
		}
		return nil, nil
	}
	copyMeta := *ctx.Meta
	copyMeta.Settings = updated
	for _, channel := range []string{"normal", "height", "emission"} {
		target, explicit, err := assetSetting(updated, channel)
		if err != nil {
			return nil, err
		}
		if !explicit && ctx.Lookup != nil && ((channel == "normal" && updated.Bool("auto_normal", true)) || (channel == "emission" && updated.Bool("auto_emission", true))) {
			suffixes := []string{"_n"}
			if channel == "emission" {
				suffixes = []string{"_e", "_em"}
			}
			for _, suffix := range suffixes {
				candidate, _ := ctx.Lookup(strings.TrimSuffix(ctx.Path, path.Ext(ctx.Path)) + suffix + path.Ext(ctx.Path))
				if candidate.IsZero() {
					continue
				}
				if !target.IsZero() {
					return nil, fmt.Errorf("ambiguous %s companions; choose an explicit AssetID", channel)
				}
				target = candidate
			}
			if !target.IsZero() {
				updated[channel] = aetext.AssetRef(target.String())
				changed = true
			}
		}
		if target.IsZero() || len(ctx.Meta.Subs) == 0 {
			continue
		}
		name, other, err := readSurfaceMeta(ctx, target)
		if err != nil {
			return nil, err
		}
		if other == nil {
			continue
		}
		if target != other.ID {
			return nil, fmt.Errorf("sliced %s source must reference a source sheet, not a single frame", channel)
		}
		bindings, err := asset.TextureFrameChannels(updated, channel)
		if err != nil {
			return nil, err
		}
		for _, frame := range ctx.Meta.Subs {
			if _, exists := bindings[frame.ID]; exists {
				continue
			}
			var candidate *asset.SourceFrame
			for i := range other.Subs {
				f := &other.Subs[i]
				if f.Name != frame.Name {
					continue
				}
				if candidate != nil {
					return nil, fmt.Errorf("%s source %s has ambiguous frame %q", channel, name, frame.Name)
				}
				candidate = f
			}
			if candidate == nil {
				continue
			}
			if candidate.Rect[2] != frame.Rect[2] || candidate.Rect[3] != frame.Rect[3] || candidate.EffectivePivot() != frame.EffectivePivot() {
				return nil, fmt.Errorf("%s frame %s logical size/pivot differs from the albedo", channel, frame.Name)
			}
			if err := asset.SetTextureChannel(&copyMeta, frame.ID, channel, candidate.ID, true); err != nil {
				return nil, err
			}
			changed = true
		}
	}
	if changed {
		return updated, nil
	}
	return nil, nil
}

type surfaceLinks struct {
	normal, emission   ids.AssetID
	normals, emissions map[ids.AssetID]ids.AssetID
	deps               []ids.AssetID
}

func surfaceBindings(ctx *pipeline.ImportContext, width, height int) (surfaceLinks, error) {
	out := surfaceLinks{normals: map[ids.AssetID]ids.AssetID{}, emissions: map[ids.AssetID]ids.AssetID{}}
	m := ctx.Meta
	checked := *m
	if checked.ID.IsZero() {
		checked.ID = ids.AssetID{Lo: 1}
	}
	if checked.Importer == "" {
		checked.Importer = "texture"
	}
	var resolveErr error
	resolve := func(id ids.AssetID) (asset.TextureSourceInfo, bool) {
		info, ok, err := sourceInfo(ctx, id)
		if err != nil {
			resolveErr = err
		}
		return info, ok
	}
	for _, p := range asset.ValidateTextureSource(&checked, ctx.Path, width, height, resolve) {
		if p.Error && !p.Missing {
			return out, fmt.Errorf("%s: %s: %s", p.Code, p.Field, p.Message)
		}
		if p.Missing && ctx.Log != nil {
			ctx.Log.Warn(p.Message, log.F("asset", ctx.ID.String()), log.F("channel", p.Field))
		}
	}
	if resolveErr != nil {
		return out, resolveErr
	}
	deps := map[ids.AssetID]bool{}
	selectFor := func(frame ids.AssetID) (ids.AssetID, ids.AssetID, error) {
		normal, normalExplicit, err := asset.TextureChannel(m, frame, "normal")
		if err != nil {
			return normal, ids.AssetID{}, err
		}
		height, _, err := asset.TextureChannel(m, frame, "height")
		if err != nil {
			return normal, ids.AssetID{}, err
		}
		emission, _, err := asset.TextureChannel(m, frame, "emission")
		if err != nil {
			return normal, emission, err
		}
		for _, id := range []ids.AssetID{normal, height, emission} {
			if !id.IsZero() {
				deps[id] = true
			}
		}
		if !normalExplicit && !height.IsZero() {
			info, ok, err := sourceInfo(ctx, height)
			if err != nil {
				return normal, emission, err
			}
			if ok && !info.GenerateNormal {
				if ctx.Log != nil {
					ctx.Log.Warn("height source has normal generation disabled; flat normal retained", log.F("height", height.String()))
				}
			} else {
				normal = height
			}
		}
		return normal, emission, nil
	}
	var err error
	out.normal, out.emission, err = selectFor(m.ID)
	if err != nil {
		return out, err
	}
	for _, f := range m.Subs {
		n, e, err := selectFor(f.ID)
		if err != nil {
			return out, err
		}
		out.normals[f.ID], out.emissions[f.ID] = n, e
	}
	for id := range deps {
		out.deps = append(out.deps, id)
	}
	slices.SortFunc(out.deps, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	return out, nil
}

// Salt all source/meta channel dependencies before importing any artifact. A
// reordered sheet or a changed height setting cannot reuse stale paired pages.
func (i Importer) CacheSalt(ctx *pipeline.ImportContext) ([]byte, error) {
	settings, err := i.ResolveMetadata(ctx)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = ctx.Meta.Settings
	}
	h := sha256.New()
	m := *ctx.Meta
	m.Settings = settings
	h.Write(pipeline.WriteMeta(&m))
	refs := map[ids.AssetID]bool{}
	for _, channel := range []string{"normal", "height", "emission"} {
		id, _, err := assetSetting(settings, channel)
		if err != nil {
			return nil, err
		}
		if !id.IsZero() {
			refs[id] = true
		}
		frames, err := asset.TextureFrameChannels(settings, channel)
		if err != nil {
			return nil, err
		}
		for _, frame := range m.Subs {
			if id := frames[frame.ID]; !id.IsZero() {
				refs[id] = true
			}
		}
	}
	list := make([]ids.AssetID, 0, len(refs))
	for id := range refs {
		list = append(list, id)
	}
	slices.SortFunc(list, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	seen := map[string]bool{}
	for _, id := range list {
		name, meta, err := readSurfaceMeta(ctx, id)
		if err != nil {
			return nil, err
		}
		if meta == nil || seen[name] {
			continue
		}
		seen[name] = true
		h.Write([]byte(name))
		data, err := ctx.Read(name)
		if err != nil {
			return nil, err
		}
		h.Write(data)
		for key, value := range i.Defaults() {
			if _, ok := meta.Settings[key]; !ok {
				meta.Settings[key] = value
			}
		}
		role, err := asset.TextureRoleOf(name, meta.Settings)
		if err != nil {
			return nil, err
		}
		if meta.Settings.String("role", "auto") == "auto" && meta.Settings.String("semantic", "auto") == "auto" && role != asset.RoleColor {
			meta.Settings["semantic"] = aetext.Ident(string(role))
		}
		h.Write(pipeline.WriteMeta(meta))
	}
	return h.Sum(nil), nil
}
func (Importer) SourceDependencies() bool { return true }
