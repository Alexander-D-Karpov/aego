package module

import (
	"aego/asset"
	"aego/core/ids"
	"aego/engine/problems"
	"aego/schema"
	"aego/vfs"
)

type AssetCheck struct {
	ID      ids.AssetID
	Path    string
	Kind    string
	Source  []byte
	Schema  *schema.Registry
	Data    asset.DataSource
	Exists  func(ids.AssetID) bool
	Partial bool
}

type AssetValidator func(*AssetCheck) []problems.Diagnostic

type ownedValidator struct {
	kind  string
	owner string
	fn    AssetValidator
}

type AssetStore interface {
	Revision() uint64
	Index() *asset.Index
	Artifacts() vfs.FS
}

func (r *Registry) Validator(kind string, fn AssetValidator) {
	if kind == "" || fn == nil {
		r.fail("asset validator needs a kind and a function")
		return
	}
	r.validators = append(r.validators, ownedValidator{kind: kind, owner: r.Owner(), fn: fn})
}

type storeData struct {
	s AssetStore
}

func (d storeData) blob(id ids.AssetID, kind asset.ArtifactKind) ([]byte, bool) {
	idx := d.s.Index()
	if idx == nil {
		return nil, false
	}
	e, ok := idx.Lookup(id)
	if !ok || e.Missing || e.Kind != kind {
		return nil, false
	}
	a, ok := e.Artifact(kind)
	if !ok {
		return nil, false
	}
	data, err := d.s.Artifacts().ReadFile(a.Path)
	return data, err == nil
}

func (d storeData) Data(id ids.AssetID) ([]byte, bool) {
	return d.blob(id, asset.ArtData)
}

func (d storeData) Table(id ids.AssetID) ([]byte, bool) {
	return d.blob(id, asset.ArtTable)
}

func (m *Manifest) ValidateAssets(reg *schema.Registry, s AssetStore, partial bool, dst []problems.Diagnostic) []problems.Diagnostic {
	idx := s.Index()
	if len(m.validators) == 0 || idx == nil || s.Artifacts() == nil {
		return dst
	}
	ds := storeData{s: s}
	exists := func(id ids.AssetID) bool {
		if asset.IsBuiltin(id) {
			return true
		}
		e, ok := idx.Lookup(id)
		return ok && !e.Missing
	}
	idx.Each(func(e *asset.Entry) {
		if e.Missing {
			return
		}
		kind := string(e.Kind)
		var c *AssetCheck
		for _, v := range m.validators {
			if v.kind != kind {
				continue
			}
			if c == nil {
				c = &AssetCheck{ID: e.ID, Path: e.Source, Kind: kind, Schema: reg, Data: ds, Exists: exists, Partial: partial}
				if a, ok := e.Artifact(e.Kind); ok {
					c.Source, _ = s.Artifacts().ReadFile(a.Path)
				}
			}
			for _, d := range v.fn(c) {
				if d.Owner == "" {
					d.Owner = v.owner
				}
				if d.Subject.Kind == problems.SubjectNone {
					d.Subject.Kind = problems.SubjectAsset
				}
				if d.Subject.Asset.IsZero() {
					d.Subject.Asset = e.ID
				}
				if d.Subject.File == "" {
					d.Subject.File = e.Source
				}
				dst = append(dst, d)
			}
		}
	})
	return dst
}

func (m *Manifest) AssetProblems(reg *schema.Registry, s AssetStore) problems.Source {
	return problems.Source{
		Revision: s.Revision,
		Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			return m.ValidateAssets(reg, s, false, dst)
		},
	}
}
