package pipeline

import (
	"slices"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
)

func (p *Pipeline) MetaOf(id ids.AssetID) (Meta, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return Meta{}, false
	}
	m := *r.meta
	m.Settings = r.meta.Settings.Clone()
	m.Subs = slices.Clone(r.meta.Subs)
	return m, true
}

// SetSubs writes the cells of a sliced sheet into its meta; the next import turns them into sprite assets (D339).
func (p *Pipeline) SetSubs(id ids.AssetID, subs []SubAsset) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shared {
		return errs.New(errs.AssetBadMeta, "another process holds the asset index, slice the sheet there")
	}
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return errs.New(errs.AssetNotFound, "the asset is not in the project", log.F("asset", id.String()))
	}
	m := *r.meta
	m.Subs = slices.Clone(subs)
	if err := project.WriteAtomic(p.abs(r.path)+MetaSuffix, WriteMeta(&m)); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write "+r.path+MetaSuffix).With(log.F("path", r.path))
	}
	r.meta, r.derived, r.dup, r.key = &m, false, "", ""
	p.metas[r.path+MetaSuffix] = true
	p.rev++
	return nil
}

func (p *Pipeline) SetSettings(id ids.AssetID, s Settings) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.shared {
		return errs.New(errs.AssetBadMeta, "another process holds the asset index, change the import settings there")
	}
	r := p.byID[id]
	if r == nil || r.meta == nil {
		return errs.New(errs.AssetNotFound, "the asset is not in the project", log.F("asset", id.String()))
	}
	m := *r.meta
	m.Settings = s.Clone()
	m.Subs = slices.Clone(r.meta.Subs)
	if err := project.WriteAtomic(p.abs(r.path)+MetaSuffix, WriteMeta(&m)); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write "+r.path+MetaSuffix).With(log.F("path", r.path))
	}
	r.meta, r.derived, r.dup, r.key = &m, false, "", ""
	p.metas[r.path+MetaSuffix] = true
	p.rev++
	return nil
}
