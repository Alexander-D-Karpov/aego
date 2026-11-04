package editor

import (
	"errors"
	"fmt"

	"io"
	"os"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/preview"
	"aego/editor/ext"
	"aego/render/material"
)

func (s *materialsEditor) registerThumbnails(r *ext.Registry) error {
	for _, kind := range []asset.ArtifactKind{asset.ArtMaterial, asset.ArtShader} {
		if err := r.Thumbnail(kind, preview.Handler{Version: "render2d.material-surface.v3", Render: s.thumbnail}); err != nil {
			return err
		}
	}
	return nil
}

func (s *materialsEditor) thumbnail(e asset.Entry, size int) (preview.Surface, error) {
	ed := s.ctx
	if ed.Renderer == nil || ed.Loader == nil || ed.Docs == nil {
		return preview.Surface{}, errors.New("material preview has no renderer or loader")
	}
	var m *material.Material
	var err error
	resolve := func(id ids.AssetID) (*material.Shader, error) {
		dirty := false
		for _, d := range ed.Docs.Scripts() {
			if d.Dirty() && d.Code.GLSL {
				dirty = true
				break
			}
		}
		if !dirty {
			if sh, ok := ed.Loader.Shader(id); ok {
				return sh, nil
			}
		}
		if sh, ok := s.schema(id); ok {
			return sh, nil
		}
		if v := s.shaders[id]; v.err != nil {
			return nil, v.err
		}
		return nil, fmt.Errorf("shader %s is unavailable", id.Short())
	}
	if e.Kind == asset.ArtShader {
		sh, err := resolve(e.ID)
		if err != nil {
			return preview.Surface{}, err
		}
		if len(sh.Variants) == 0 {
			return preview.Surface{}, errors.New("no usable shader variant")
		}
		var ok bool
		m, ok = material.New(e.Name, sh, sh.Variants[0].Mask, sh.Blend)
		if !ok {
			return preview.Surface{}, errors.New("incompatible preview material")
		}
	} else if asset.IsBuiltin(e.ID) {
		base, ok := ed.Loader.Material(e.ID)
		if !ok {
			return preview.Surface{}, errors.New("built-in material is unavailable")
		}
		m, err = base.CloneDefinition()
		if err != nil {
			return preview.Surface{}, err
		}
	} else {
		var src []byte
		if d := ed.Docs.Script(e.Source); d != nil {
			src = d.Code.Bytes()
		} else {
			var f *os.File
			f, err = os.OpenInRoot(ed.Pipeline.Abs("."), e.Source)
			if err == nil {
				defer f.Close()
				src, err = io.ReadAll(io.LimitReader(f, 1<<20+1))
				if len(src) > 1<<20 {
					return preview.Surface{}, errors.New("material source exceeds 1 MiB")
				}
			}
			if err != nil {
				return preview.Surface{}, err
			}
		}
		doc, err := asset.ParseMaterial(src, e.Source)
		if err != nil {
			return preview.Surface{}, err
		}
		sh, err := resolve(doc.Shader)
		if err != nil {
			return preview.Surface{}, err
		}
		var issues []material.Problem
		m, issues = doc.Build(e.Name, sh, ed.Loader.MaterialParameterTexture)
		if len(issues) > 0 {
			return preview.Surface{}, errors.New(issues[0].Msg)
		}
	}
	if m == nil {
		return preview.Surface{}, errors.New("could not prepare material")
	}
	var p materialPreview
	p.update(ed, m, 1, 1)
	keep := false
	defer func() {
		if !keep {
			p.close(ed)
		}
	}()
	if p.reason != "" {
		return preview.Surface{}, errors.New(p.reason)
	}
	if err := p.prepare(ed, size); err != nil {
		return preview.Surface{}, err
	}
	p.renderSurface(ed, size)
	if p.renderError != "" {
		return preview.Surface{}, errors.New(p.renderError)
	}
	if !ed.Renderer.Materials().Ready(m) {
		return preview.Surface{}, errors.New("shader did not link; see Problems")
	}
	keep = true
	return preview.Surface{Target: p.target.Texture, Close: func() { p.close(ed) }}, nil
}
