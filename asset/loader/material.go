package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/render/material"
)

// materials holds the runtime shaders and materials; a reimported shader reaches its materials through Reload (D393).
type materials struct {
	cache   *material.Cache
	schemas material.Schemas
	shaders map[ids.AssetID]*material.Shader
	mats    map[ids.AssetID]*material.Material
	uses    map[ids.AssetID]ids.AssetID
}

// SetMaterials gives the loader the renderer's material cache, which retires replaced shaders and frees dropped materials.
func (l *Loader) SetMaterials(c *material.Cache) {
	l.mat.cache = c
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
	return a.Runtime(e.ID, e.Name), nil
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
	doc, err := asset.ParseMaterial(data, e.Name)
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
	return m, true
}

func (l *Loader) materialTexture(id ids.AssetID) (material.Tex, bool) {
	r, ok := l.Texture(id)
	if !ok {
		return material.Tex{}, false
	}
	return material.Tex{Texture: r.Texture}, true
}

// swapShader moves the materials on a reimported shader to its new version; a shader that does not decode keeps the old one.
func (l *Loader) swapShader(id ids.AssetID, data []byte) {
	old, ok := l.mat.shaders[id]
	if !ok {
		return
	}
	e, _ := l.entry(id)
	sh, err := l.decodeShader(e, data)
	if err != nil {
		l.log.Error("the reimported shader cannot be read, the previous version stays", log.F("asset", id.String()), log.F("err", err.Error()))
		return
	}
	l.mat.shaders[id] = sh
	l.mat.schemas.Set(id, sh.Schema)
	for mid, sid := range l.mat.uses {
		if sid == id {
			l.mat.mats[mid].Reload(sh)
		}
	}
	if l.mat.cache != nil {
		l.mat.cache.Retire(old)
	}
}

func (l *Loader) dropMaterial(id ids.AssetID) {
	if m, ok := l.mat.mats[id]; ok {
		if l.mat.cache != nil {
			l.mat.cache.Release(m)
		}
		delete(l.mat.mats, id)
		delete(l.mat.uses, id)
	}
}

func (l *Loader) dropShader(id ids.AssetID) {
	if sh, ok := l.mat.shaders[id]; ok {
		if l.mat.cache != nil {
			l.mat.cache.Retire(sh)
		}
		delete(l.mat.shaders, id)
		l.mat.schemas.Delete(id)
	}
}
