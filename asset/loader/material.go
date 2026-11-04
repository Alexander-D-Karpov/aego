package loader

import (
	"bytes"
	"fmt"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
	"aego/render/material"
	"aego/render/texture"
)

// materials holds the runtime shaders and materials; a reimported shader reaches its materials through Reload (D393).
type materials struct {
	cache   *material.Cache
	schemas material.Schemas
	shaders map[ids.AssetID]*material.Shader
	mats    map[ids.AssetID]*material.Material
	uses    map[ids.AssetID]ids.AssetID
	pending map[ids.AssetID]*material.Shader
	retired []gpu.TextureHandle
}

// SetMaterials gives the loader the renderer's material cache, which retires replaced shaders and frees dropped materials.
func (l *Loader) SetMaterials(c *material.Cache) {
	l.mat.cache = c
	for _, m := range l.mat.mats {
		if c != nil {
			c.Track(m)
		}
	}
}

// Schemas are the parameter schemas of the loaded shaders, by shader asset ID.
func (l *Loader) Schemas() *material.Schemas {
	return &l.mat.schemas
}

func (l *Loader) decodeShader(e *asset.Entry, data []byte) (*material.Shader, error) {
	a, err := asset.DecodeShader(data)
	if err != nil {
		return nil, err
	}
	sh := a.Runtime(e.ID, e.Name)
	if e.ID == asset.BuiltinShaderLit {
		stock, ok := asset.BuiltinShader(e.ID)
		sh.StockLit = ok && bytes.Equal(data, stock)
		sh.OpaqueWithOpaqueAlbedo = sh.StockLit
	}
	return sh, nil
}

func (l *Loader) Shader(id ids.AssetID) (*material.Shader, bool) {
	if sh, ok := l.mat.shaders[id]; ok {
		return sh, true
	}
	data, ok := l.blob(id, asset.ArtShader)
	if !ok {
		return nil, false
	}
	e, _ := l.entry(id)
	sh, err := l.decodeShader(e, data)
	if err != nil {
		l.fail(id, "cannot read the shader", err)
		return nil, false
	}
	l.mat.shaders[id] = sh
	l.mat.schemas.Set(id, sh.Schema)
	return sh, true
}

func (l *Loader) Material(id ids.AssetID) (*material.Material, bool) {
	if m, ok := l.mat.mats[id]; ok {
		return m, true
	}
	data, ok := l.blob(id, asset.ArtMaterial)
	if !ok {
		return nil, false
	}
	e, _ := l.entry(id)
	doc, err := asset.DecodeMaterial(data, e.Name)
	if err != nil {
		l.fail(id, "cannot read the material", err)
		return nil, false
	}
	sh, ok := l.Shader(doc.Shader)
	if !ok {
		l.fail(id, "the material's shader is missing", nil)
		return nil, false
	}
	m, probs := doc.Build(e.Name, sh, l.materialTexture)
	for _, p := range probs {
		say := l.log.Error
		if p.Warn {
			say = l.log.Warn
		}
		say(p.Msg, log.F("asset", e.Name), log.F("code", p.Code), log.F("line", p.Line))
	}
	if m == nil {
		l.failed[id] = true
		return nil, false
	}
	m.ID = id
	l.mat.mats[id] = m
	l.mat.uses[id] = doc.Shader
	if l.mat.cache != nil {
		l.mat.cache.Track(m)
	}
	return m, true
}

func (l *Loader) materialTexture(id ids.AssetID) (material.Tex, bool) {
	e, exists := l.entry(id)
	if !exists || (e.Kind != asset.ArtTexture && e.Kind != asset.ArtAtlasPage && e.Kind != asset.ArtSpriteRegion) {
		return material.Tex{}, false
	}
	r, ok := l.Texture(id)
	if !ok {
		return material.Tex{}, false
	}
	page := l.pages[l.region[id].page]
	return material.Tex{Texture: r.Texture, Region: r, RegionOnly: e.Kind == asset.ArtSpriteRegion, NormalMap: page.semantic == asset.TextureNormal, EmissionMode: emissionMode(page.semantic)}, true
}

// MaterialTexture resolves a complete texture for a material sampler. The
// editor preview uses this same contract as runtime loading: a sprite's atlas
// region cannot be substituted for a secondary noise or data texture.
func (l *Loader) MaterialTexture(id ids.AssetID) (material.Tex, bool) {
	t, ok := l.materialTexture(id)
	if !ok || t.RegionOnly {
		return material.Tex{}, false
	}
	return t, true
}

// MaterialParameterTexture shares the runtime parameter resolver with the
// editor. Material schema validation admits independently packed regions only
// for @normal, and requires their prepared normal/data import semantic.
func (l *Loader) MaterialParameterTexture(id ids.AssetID) (material.Tex, bool) {
	return l.materialTexture(id)
}

// RejectSource cancels an in-flight candidate when a newer source fails import.
// A slow, older successful compile must not commit after that failure.
func (l *Loader) RejectSource(id ids.AssetID) {
	l.rejected[id] = true
	if m, ok := l.mat.mats[id]; ok && l.mat.cache != nil {
		l.mat.cache.CancelMaterial(m)
	}
	shaderID := id
	if sid, ok := l.mat.uses[id]; ok {
		shaderID = sid
	}
	if old, ok := l.mat.shaders[shaderID]; ok && l.mat.cache != nil {
		l.mat.cache.CancelReload(old)
	}
	delete(l.mat.pending, shaderID)
}

// queueMaterialChanges prepares one CPU candidate per shader group, retaining
// the live shader, schema, values and sampler handles until a frame-end commit.
func (l *Loader) queueMaterialChanges(prepared []staged) {
	if l.mat.pending == nil {
		l.mat.pending = map[ids.AssetID]*material.Shader{}
	}
	changed := map[ids.AssetID]bool{}
	touched := map[ids.AssetID]bool{}
	for _, s := range prepared {
		if s.unchanged {
			continue
		}
		changed[s.id] = true
		if s.kind == asset.ArtMaterial && l.mat.cache != nil {
			if live := l.mat.mats[s.id]; live != nil {
				// Even a newer invalid source supersedes an older queued success.
				// Keep the pending CPU shader revision: this batch rebuilds its
				// transaction against the latest material definition below.
				l.mat.cache.CancelMaterial(live)
				l.mat.cache.CancelReload(live.Shader())
			}
		}
		if s.kind == asset.ArtShader {
			if _, loaded := l.mat.shaders[s.id]; loaded {
				e, _ := l.entry(s.id)
				sh, err := l.decodeShader(e, s.blob)
				if err != nil {
					l.fail(s.id, "cannot prepare the shader revision", err)
					l.RejectSource(s.id)
					continue
				}
				l.mat.pending[s.id] = sh
				touched[s.id] = true
			}
		}
	}
	type group struct {
		old, next   *material.Shader
		definitions map[*material.Material]*material.Material
		invalid     bool
	}
	type materialSwitch struct {
		id, shaderID ids.AssetID
		live, next   *material.Material
	}
	var switches []materialSwitch
	groups := map[ids.AssetID]*group{}
	get := func(id ids.AssetID) *group {
		if g := groups[id]; g != nil {
			return g
		}
		old := l.mat.shaders[id]
		next := l.mat.pending[id]
		if next == nil {
			next = old
		}
		g := &group{old: old, next: next, definitions: map[*material.Material]*material.Material{}}
		groups[id] = g
		return g
	}
	for id := range touched {
		get(id)
	}
	for id, live := range l.mat.mats {
		sid := l.mat.uses[id]
		data, ok := l.blob(id, asset.ArtMaterial)
		if !ok {
			continue
		}
		e, _ := l.entry(id)
		doc, err := asset.DecodeMaterial(data, e.Name)
		if err != nil {
			l.fail(id, "cannot prepare the material revision", err)
			get(sid).invalid = true
			continue
		}
		texturesChanged := false
		for _, tid := range doc.Textures {
			texturesChanged = texturesChanged || changed[tid]
			if te, ok := l.entry(tid); ok {
				for _, dep := range te.Deps {
					texturesChanged = texturesChanged || changed[dep]
				}
			}
		}
		targetChanged := doc.Shader != sid && touched[doc.Shader]
		if !changed[id] && !texturesChanged && !targetChanged {
			continue
		}
		g := get(sid)
		target := g.next
		if doc.Shader != sid {
			target = l.mat.pending[doc.Shader]
			if target == nil {
				target, ok = l.Shader(doc.Shader)
			} else {
				ok = true
			}
			if !ok {
				l.fail(id, "the new material shader is unavailable", nil)
				g.invalid = true
				continue
			}
		}
		var candidate *material.Material
		if !changed[id] && !targetChanged {
			candidate, err = l.mat.cache.ClonePendingDefinition(live)
		}
		if err == nil && candidate == nil && (changed[id] || targetChanged || doc.Shader != sid) {
			var ps []material.Problem
			candidate, ps = doc.Build(e.Name, target, l.materialTexture)
			for _, p := range ps {
				if !p.Warn {
					err = fmt.Errorf("%s: %s", p.Code, p.Msg)
					break
				}
			}
		}
		if candidate == nil && err == nil {
			candidate, err = live.CloneDefinition()
		}
		if candidate != nil && err == nil && !changed[id] {
			for name, tid := range doc.Textures {
				if tex, ok := l.materialTexture(tid); ok {
					if !candidate.SetTexture(name, tex) {
						if _, known := candidate.Shader().Schema.Param(name); known {
							err = fmt.Errorf("%s texture has an incompatible region or normal import semantic", name)
							break
						}
						candidate.ResolveRetainedTexture(name, tex)
					}
				} else if p, known := candidate.Shader().Schema.Param(name); known && p.Normal {
					candidate.SetUnresolvedTexture(name, tid)
				}
			}
		}
		if err != nil || candidate == nil {
			l.fail(id, "material reload rejected; keeping the previous revision", err)
			g.invalid = true
			continue
		}
		candidate.ID = id
		if doc.Shader != sid {
			if candidate.Shader().Domain != live.Shader().Domain {
				l.fail(id, "the new shader has an incompatible material domain", nil)
				g.invalid = true
				continue
			}
			if l.mat.cache != nil {
				switches = append(switches, materialSwitch{id, doc.Shader, live, candidate})
			}
			continue
		}
		g.definitions[live] = candidate
	}
	for id, g := range groups {
		if g.invalid {
			l.RejectSource(id)
			continue
		}
		if g.old == nil || g.next == nil || (g.old == g.next && len(g.definitions) == 0) {
			continue
		}
		if l.mat.cache == nil {
			l.log.Warn("shader candidate is prepared but GPU validation is unavailable; keeping last-good", log.F("asset", id.String()))
			continue
		}
		next := g.next
		l.mat.cache.QueueReloadDefinitions(g.old, next, g.definitions, func() {
			l.mat.shaders[id] = next
			l.mat.schemas.Set(id, next.Schema)
			if l.mat.pending[id] == next {
				delete(l.mat.pending, id)
			}
			l.materialCommitted(id, asset.ArtShader)
		})
	}
	// Register switches after shader transactions, so a switch to a candidate
	// shader waits for that shader's existing consumers to validate and commit.
	for _, sw := range switches {
		if g := groups[sw.shaderID]; g != nil && g.invalid {
			l.mat.cache.CancelMaterial(sw.live)
			l.fail(sw.id, "material reload rejected because its target shader transaction is invalid", nil)
			continue
		}
		l.mat.cache.QueueMaterial(sw.live, sw.next, func() {
			l.mat.uses[sw.id] = sw.shaderID
			l.materialCommitted(sw.id, asset.ArtMaterial)
		})
	}
	l.sweepMaterialTextures()
}

func (l *Loader) materialCommitted(id ids.AssetID, kind asset.ArtifactKind) {
	clear(l.decoded)
	l.bumpGeneration()
	delete(l.failed, id)
	l.sweepMaterialTextures()
	l.notify(asset.Reloaded{ID: id, Kind: kind})
}

func (l *Loader) retireMaterialTexture(h gpu.TextureHandle) {
	if !h.IsZero() {
		l.mat.retired = append(l.mat.retired, h)
	}
}

func (l *Loader) sweepMaterialTextures() {
	keep := l.mat.retired[:0]
	for _, h := range l.mat.retired {
		used := false
		for _, m := range l.mat.mats {
			if m.UsesTexture(h) {
				used = true
				break
			}
		}
		if used {
			keep = append(keep, h)
		} else {
			l.dev.DestroyTexture(h)
		}
	}
	l.mat.retired = keep
}

func (l *Loader) dropMaterial(id ids.AssetID) {
	if m, ok := l.mat.mats[id]; ok {
		if l.mat.cache != nil {
			l.mat.cache.Release(m)
		}
		delete(l.mat.mats, id)
		delete(l.mat.uses, id)
		l.sweepMaterialTextures()
	}
}

func (l *Loader) dropShader(id ids.AssetID) {
	// A deleted/temporarily missing source cancels candidates but keeps the
	// last-good definition of its active materials. Reappearance with the same
	// AssetID must still migrate those stable material pointers.
	l.RejectSource(id)
	for _, sid := range l.mat.uses {
		if sid == id {
			return
		}
	}
	if sh, ok := l.mat.shaders[id]; ok {
		if l.mat.cache != nil {
			l.mat.cache.Retire(sh)
		}
		delete(l.mat.shaders, id)
		l.mat.schemas.Delete(id)
	}
}

func emissionMode(s asset.TextureSemantic) texture.EmissionMode {
	switch s {
	case asset.TextureEmissionRGB:
		return texture.EmissionRGB
	case asset.TextureEmissionMask:
		return texture.EmissionMask
	}
	return texture.EmissionNone
}
