package presets

import (
	"path"
	"sort"
	"strings"

	"aego/asset"
	"aego/asset/importers/preset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/clipboard"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/schema"
)

const Dir = "assets/presets"

type Preset struct {
	ID     ids.AssetID
	Path   string
	Name   string
	Type   string
	Script bool
	hash   string
}

type Service struct {
	ctx   *context.Editor
	byID  map[ids.AssetID]Preset
	list  []Preset
	rev   uint64
	valid bool
}

func New(ctx *context.Editor) *Service {
	return &Service{ctx: ctx, byID: map[ids.AssetID]Preset{}}
}

func (s *Service) data(id ids.AssetID) ([]byte, string, bool) {
	pl := s.ctx.Pipeline
	if pl == nil {
		return nil, "", false
	}
	e, ok := pl.Entry(id)
	if !ok {
		return nil, "", false
	}
	art, ok := e.Artifact(asset.ArtPreset)
	if !ok {
		return nil, "", false
	}
	b, err := pl.Artifacts().ReadFile(art.Path)
	if err != nil {
		return nil, "", false
	}
	return b, art.Hash, true
}

func (s *Service) refresh() {
	pl := s.ctx.Pipeline
	if pl == nil {
		return
	}
	if s.valid && s.rev == pl.Revision() {
		return
	}
	s.rev, s.valid = pl.Revision(), true
	s.list = s.list[:0]
	for _, a := range pl.Assets() {
		if a.Kind != asset.ArtPreset {
			continue
		}
		e, ok := pl.Entry(a.ID)
		if !ok {
			continue
		}
		art, _ := e.Artifact(asset.ArtPreset)
		p, cached := s.byID[a.ID]
		if !cached || p.hash != art.Hash {
			data, hash, ok := s.data(a.ID)
			if !ok {
				continue
			}
			doc, err := aetext.Parse(data, a.Path)
			if err != nil {
				continue
			}
			name, script, ok := preset.Type(doc)
			if !ok {
				continue
			}
			p = Preset{ID: a.ID, Type: name, Script: script, hash: hash}
		}
		p.Path = a.Path
		p.Name = strings.TrimSuffix(path.Base(a.Path), preset.Ext)
		s.byID[a.ID] = p
		s.list = append(s.list, p)
	}
	sort.Slice(s.list, func(i, j int) bool { return s.list[i].Name < s.list[j].Name })
}

func (s *Service) For(typeName string) []Preset {
	s.refresh()
	var out []Preset
	for _, p := range s.list {
		if p.Type == typeName {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) Save(d *document.SceneDocument, node ids.NodeID, td *schema.TypeDescriptor, name string) (string, error) {
	name = strings.TrimSpace(name)
	if !assetops.ValidName(name) {
		return "", errs.New(errs.EditorCommand, "\""+name+"\" is not a valid preset name")
	}
	env := clipboard.EnvOf(s.ctx)
	var data []byte
	var err error
	if td.Kind == schema.TypeScript {
		data, err = env.EncodeScript(d.Scene(), node, td.FullName)
	} else {
		data, err = env.EncodeComponent(d.Scene(), node, td.ID)
	}
	if err != nil {
		return "", err
	}
	if err := assetops.EnsureDir(s.ctx, Dir); err != nil {
		return "", err
	}
	rel := assetops.UniqueFile(s.ctx, Dir, name, preset.Ext)
	if _, err := assetops.CreateFile(s.ctx, rel, data); err != nil {
		return "", err
	}
	s.valid = false
	return rel, nil
}

func (s *Service) Apply(d *document.SceneDocument, nodes []ids.NodeID, id ids.AssetID) error {
	s.refresh()
	p, ok := s.byID[id]
	if !ok {
		return errs.New(errs.EditorCommand, "the preset is not imported")
	}
	data, _, ok := s.data(id)
	if !ok {
		return errs.New(errs.EditorCommand, "the preset cannot be read")
	}
	env := clipboard.EnvOf(s.ctx)
	label := i18n.Tr("editor.presets.apply_preset_name", "Apply Preset {name}", i18n.Str("name", p.Name))
	if p.Script {
		return env.ApplyScriptData(d, nodes, data, label)
	}
	return env.ApplyComponentData(d, nodes, data, label)
}
