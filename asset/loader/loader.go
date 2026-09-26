package loader

import (
	"bytes"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
	"aego/render/texture"
	"aego/vfs"
)

const ServiceID = "asset.loader"

type pageEntry struct {
	handle gpu.TextureHandle
	width  int
	height int
	filter gpu.FilterMode
}

type regionEntry struct {
	region texture.Region
	page   ids.AssetID
}

type Loader struct {
	dev    gpu.Device
	up     *texture.Uploader
	log    *log.ChannelLogger
	index  *asset.Index
	files  vfs.FS
	pages  map[ids.AssetID]pageEntry
	region map[ids.AssetID]regionEntry
	blobs  map[ids.AssetID][]byte
	failed map[ids.AssetID]bool
	subs   []subscriber
	nextID uint64
	gen    uint32
}

type subscriber struct {
	id uint64
	fn func(asset.Reloaded)
}

func New(dev gpu.Device, up *texture.Uploader, lg *log.Logger) *Loader {
	if lg == nil {
		lg = log.Nop()
	}
	return &Loader{
		dev:    dev,
		up:     up,
		log:    lg.With(log.Asset),
		pages:  map[ids.AssetID]pageEntry{},
		region: map[ids.AssetID]regionEntry{},
		blobs:  map[ids.AssetID][]byte{},
		failed: map[ids.AssetID]bool{},
		gen:    1,
	}
}

func (l *Loader) Mount(idx *asset.Index, files vfs.FS) error {
	if idx == nil || files == nil {
		return errs.New(errs.AssetNotMounted, "loader needs both an index and an artifact file system")
	}
	l.index = idx
	l.files = files
	l.gen++
	return nil
}

func (l *Loader) Generation() uint32 {
	return l.gen
}

func (l *Loader) Resident() int {
	return len(l.pages) + len(l.blobs)
}

func (l *Loader) Subscribe(fn func(asset.Reloaded)) func() {
	l.nextID++
	id := l.nextID
	l.subs = append(l.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range l.subs {
			if l.subs[i].id == id {
				l.subs = append(l.subs[:i], l.subs[i+1:]...)
				return
			}
		}
	}
}

func (l *Loader) notify(r asset.Reloaded) {
	for i := range l.subs {
		l.subs[i].fn(r)
	}
}

func (l *Loader) entry(id ids.AssetID) (*asset.Entry, bool) {
	if l.index == nil {
		return nil, false
	}
	e, ok := l.index.Lookup(id)
	if !ok || e.Missing {
		return nil, false
	}
	return e, true
}

func (l *Loader) read(a asset.Artifact) ([]byte, error) {
	data, err := l.files.ReadFile(a.Path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (l *Loader) Texture(id ids.AssetID) (texture.Region, bool) {
	if r, ok := l.region[id]; ok {
		return r.region, true
	}
	if l.failed[id] {
		return texture.Region{}, false
	}
	e, ok := l.entry(id)
	if !ok {
		return texture.Region{}, false
	}
	switch e.Kind {
	case asset.ArtSpriteRegion:
		return l.loadRegion(id, e)
	case asset.ArtTexture, asset.ArtAtlasPage:
		page, ok := l.loadPage(id, e)
		if !ok {
			return texture.Region{}, false
		}
		reg := texture.Whole(page.handle, page.width, page.height)
		l.region[id] = regionEntry{region: reg, page: id}
		return reg, true
	}
	return texture.Region{}, false
}

func (l *Loader) loadRegion(id ids.AssetID, e *asset.Entry) (texture.Region, bool) {
	art, ok := e.Artifact(asset.ArtSpriteRegion)
	if !ok {
		l.fail(id, "sprite region artifact is missing", nil)
		return texture.Region{}, false
	}
	data, err := l.read(art)
	if err != nil {
		l.fail(id, "cannot read the sprite region artifact", err)
		return texture.Region{}, false
	}
	ra, err := asset.DecodeRegion(data)
	if err != nil {
		l.fail(id, "malformed sprite region artifact", err)
		return texture.Region{}, false
	}
	pageEntryValue, ok := l.pages[ra.Page]
	if !ok {
		pe, pok := l.entry(ra.Page)
		if !pok {
			l.fail(id, "sprite region points at a missing page", nil)
			return texture.Region{}, false
		}
		pageEntryValue, ok = l.loadPage(ra.Page, pe)
		if !ok {
			return texture.Region{}, false
		}
	}
	whole := texture.Whole(pageEntryValue.handle, pageEntryValue.width, pageEntryValue.height)
	reg := whole.Sub(ra.X, ra.Y, ra.W, ra.H)
	l.region[id] = regionEntry{region: reg, page: ra.Page}
	return reg, true
}

func (l *Loader) loadPage(id ids.AssetID, e *asset.Entry) (pageEntry, bool) {
	if p, ok := l.pages[id]; ok {
		return p, true
	}
	kind := asset.ArtTexture
	if e.Kind == asset.ArtAtlasPage {
		kind = asset.ArtAtlasPage
	}
	art, ok := e.Artifact(kind)
	if !ok {
		l.fail(id, "texture artifact is missing", nil)
		return pageEntry{}, false
	}
	data, err := l.read(art)
	if err != nil {
		l.fail(id, "cannot read the texture artifact", err)
		return pageEntry{}, false
	}
	p, err := l.uploadTexture(data)
	if err != nil {
		l.fail(id, "cannot upload the texture", err)
		return pageEntry{}, false
	}
	l.pages[id] = p
	return p, true
}

func (l *Loader) uploadTexture(data []byte) (pageEntry, error) {
	ta, err := asset.DecodeTexture(data)
	if err != nil {
		return pageEntry{}, err
	}
	d := &texture.Data{
		Width:         ta.Width,
		Height:        ta.Height,
		Format:        ta.Format,
		Pixels:        ta.Pixels,
		Premultiplied: true,
	}
	h, err := l.up.Upload(d, texture.UploadOptions{Mips: ta.Mips, Label: "asset"})
	if err != nil {
		return pageEntry{}, err
	}
	return pageEntry{handle: h, width: ta.Width, height: ta.Height, filter: ta.Filter}, nil
}

func (l *Loader) fail(id ids.AssetID, msg string, cause error) {
	l.failed[id] = true
	fields := []log.Field{log.F("asset", id.String())}
	if cause != nil {
		fields = append(fields, log.F("err", cause.Error()))
	}
	l.log.Error(msg, fields...)
}

func (l *Loader) blob(id ids.AssetID, kind asset.ArtifactKind) ([]byte, bool) {
	e, ok := l.entry(id)
	if !ok || e.Kind != kind {
		return nil, false
	}
	if b, ok := l.blobs[id]; ok {
		return b, true
	}
	if l.failed[id] {
		return nil, false
	}
	art, ok := e.Artifact(kind)
	if !ok {
		l.fail(id, "artifact is missing", nil)
		return nil, false
	}
	data, err := l.read(art)
	if err != nil {
		l.fail(id, "cannot read the artifact", err)
		return nil, false
	}
	l.blobs[id] = data
	return data, true
}

func (l *Loader) Scene(id ids.AssetID) ([]byte, bool) {
	return l.blob(id, asset.ArtScene)
}

func (l *Loader) Prefab(id ids.AssetID) ([]byte, bool) {
	return l.blob(id, asset.ArtPrefab)
}

func (l *Loader) Preload(list ...ids.AssetID) error {
	for _, id := range list {
		e, ok := l.entry(id)
		if !ok {
			return errs.New(errs.AssetNotFound, "asset is not in the index", log.F("asset", id.String()))
		}
		switch e.Kind {
		case asset.ArtTexture, asset.ArtAtlasPage, asset.ArtSpriteRegion:
			if _, ok := l.Texture(id); !ok {
				return errs.New(errs.AssetLoadFailed, "texture failed to load", log.F("asset", id.String()))
			}
		case asset.ArtScene:
			if _, ok := l.Scene(id); !ok {
				return errs.New(errs.AssetLoadFailed, "scene failed to load", log.F("asset", id.String()))
			}
		case asset.ArtPrefab:
			if _, ok := l.Prefab(id); !ok {
				return errs.New(errs.AssetLoadFailed, "prefab failed to load", log.F("asset", id.String()))
			}
		}
	}
	return nil
}

type staged struct {
	id        ids.AssetID
	kind      asset.ArtifactKind
	page      pageEntry
	blob      []byte
	isTex     bool
	unchanged bool
}

func (l *Loader) Swap(entries []asset.Entry, files vfs.FS) error {
	if files != nil {
		l.files = files
	}
	if l.index == nil {
		return errs.New(errs.AssetNotMounted, "loader has no index mounted")
	}
	prepared := make([]staged, 0, len(entries))
	for i := range entries {
		e := entries[i]
		switch e.Kind {
		case asset.ArtTexture, asset.ArtAtlasPage:
			art, ok := e.Artifact(e.Kind)
			if !ok {
				return errs.New(errs.AssetBadArtifact, "entry has no artifact of its own kind",
					log.F("asset", e.ID.String()))
			}
			data, err := l.files.ReadFile(art.Path)
			if err != nil {
				l.rollback(prepared)
				return err
			}
			p, err := l.uploadTexture(data)
			if err != nil {
				l.rollback(prepared)
				return err
			}
			prepared = append(prepared, staged{id: e.ID, kind: e.Kind, page: p, isTex: true})
		case asset.ArtSpriteRegion, asset.ArtScene, asset.ArtPrefab:
			art, ok := e.Artifact(e.Kind)
			if !ok {
				return errs.New(errs.AssetBadArtifact, "entry has no artifact of its own kind",
					log.F("asset", e.ID.String()))
			}
			data, err := l.files.ReadFile(art.Path)
			if err != nil {
				l.rollback(prepared)
				return err
			}
			prepared = append(prepared, staged{id: e.ID, kind: e.Kind, blob: data})
		}
	}

	for i := range entries {
		l.index.Put(entries[i])
	}
	for i := range prepared {
		s := &prepared[i]
		delete(l.failed, s.id)
		switch {
		case s.isTex:
			if old, ok := l.pages[s.id]; ok {
				l.dev.DestroyTexture(old.handle)
			}
			l.pages[s.id] = s.page
			l.invalidateRegionsOf(s.id)
		case s.kind == asset.ArtSpriteRegion:
			delete(l.region, s.id)
		default:
			if old, ok := l.blobs[s.id]; ok && bytes.Equal(old, s.blob) {
				s.unchanged = true
				continue
			}
			l.blobs[s.id] = s.blob
		}
	}
	l.gen++
	for i := range prepared {
		if prepared[i].unchanged {
			continue
		}
		l.notify(asset.Reloaded{ID: prepared[i].id, Kind: prepared[i].kind})
	}
	return nil
}

func (l *Loader) invalidateRegionsOf(page ids.AssetID) {
	for id, r := range l.region {
		if r.page == page {
			delete(l.region, id)
		}
	}
}

func (l *Loader) rollback(prepared []staged) {
	for _, s := range prepared {
		if s.isTex {
			l.dev.DestroyTexture(s.page.handle)
		}
	}
}

func (l *Loader) Drop(id ids.AssetID) {
	if p, ok := l.pages[id]; ok {
		l.dev.DestroyTexture(p.handle)
		delete(l.pages, id)
		l.invalidateRegionsOf(id)
	}
	delete(l.region, id)
	delete(l.blobs, id)
	delete(l.failed, id)
	l.gen++
}

func (l *Loader) Close() {
	for id, p := range l.pages {
		l.dev.DestroyTexture(p.handle)
		delete(l.pages, id)
	}
	l.region = map[ids.AssetID]regionEntry{}
	l.blobs = map[ids.AssetID][]byte{}
	l.failed = map[ids.AssetID]bool{}
	l.subs = nil
}

var _ asset.Resolver = (*Loader)(nil)
