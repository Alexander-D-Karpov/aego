package loader

import (
	"bytes"

	"aego/anim"
	"aego/asset"
	"aego/asset/silhouette"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/gpu"
	"aego/render/material"
	"aego/render/mesh"
	"aego/render/texture"
	"aego/vfs"
)

const ServiceID = "asset.loader"

type pageEntry struct {
	handle   gpu.TextureHandle
	width    int
	height   int
	filter   gpu.FilterMode
	semantic asset.TextureSemantic
}

type regionEntry struct {
	region texture.Region
	page   ids.AssetID
}

type decodedKey struct {
	id  ids.AssetID
	key string
}

type decodedValue struct {
	v   any
	err error
}

type Loader struct {
	shadowFrames      map[ids.AssetID]silhouette.Shape
	textureMetadata   map[ids.AssetID]asset.TextureMetadata
	metadataDeviceGen uint64
	masks             maskCache
	dev               gpu.Device
	up                *texture.Uploader
	log               *log.ChannelLogger
	index             *asset.Index
	files             vfs.FS
	pages             map[ids.AssetID]pageEntry
	region            map[ids.AssetID]regionEntry
	normals           map[ids.AssetID]ids.AssetID
	emissions         map[ids.AssetID]ids.AssetID
	rejected          map[ids.AssetID]bool
	blobs             map[ids.AssetID][]byte
	meshes            map[ids.AssetID]*mesh.Mesh
	failed            map[ids.AssetID]bool
	decoded           map[decodedKey]decodedValue
	mat               materials
	subs              []subscriber
	nextID            uint64
	gen               uint32
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
		dev:       dev,
		up:        up,
		log:       lg.With(log.Asset),
		pages:     map[ids.AssetID]pageEntry{},
		region:    map[ids.AssetID]regionEntry{},
		normals:   map[ids.AssetID]ids.AssetID{},
		emissions: map[ids.AssetID]ids.AssetID{},
		rejected:  map[ids.AssetID]bool{},
		blobs:     map[ids.AssetID][]byte{},
		meshes:    map[ids.AssetID]*mesh.Mesh{},
		failed:    map[ids.AssetID]bool{},
		decoded:   map[decodedKey]decodedValue{},
		mat: materials{
			shaders: map[ids.AssetID]*material.Shader{},
			mats:    map[ids.AssetID]*material.Material{},
			uses:    map[ids.AssetID]ids.AssetID{},
		},
		gen: 1,
	}
}

func (l *Loader) Mount(idx *asset.Index, files vfs.FS) error {
	if idx == nil || files == nil {
		return errs.New(errs.AssetNotMounted, "loader needs both an index and an artifact file system")
	}
	l.index = idx.Clone()
	l.files = files
	clear(l.normals)
	clear(l.emissions)
	clear(l.rejected)
	clear(l.decoded)
	l.bumpGeneration()
	return nil
}

func (l *Loader) Decoded(id ids.AssetID, key string, build func() (any, error)) (any, error) {
	k := decodedKey{id: id, key: key}
	if d, ok := l.decoded[k]; ok {
		return d.v, d.err
	}
	v, err := build()
	l.decoded[k] = decodedValue{v: v, err: err}
	return v, err
}

func (l *Loader) Cached(id ids.AssetID, key string) (any, bool, error) {
	d, ok := l.decoded[decodedKey{id: id, key: key}]
	return d.v, ok, d.err
}

func (l *Loader) bumpGeneration() {
	l.gen++
	l.masks = maskCache{}
	clear(l.shadowFrames)
	clear(l.textureMetadata)
}

func (l *Loader) Generation() uint32 {
	return l.gen
}

func (l *Loader) Resident() int {
	return len(l.pages) + len(l.blobs) + len(l.meshes)
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
	if builtin, ok := asset.LookupBuiltin(id); ok {
		return &asset.Entry{ID: id, Name: builtin.Name, Kind: builtin.Kind}, true
	}
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
	return l.files.ReadFile(a.Path)
}

func (l *Loader) Scripts() (map[string][]byte, error) {
	out := map[string][]byte{}
	if l.index == nil {
		return out, nil
	}
	var first error
	l.index.Each(func(e *asset.Entry) {
		if e.Kind != asset.ArtScript || e.Missing {
			return
		}
		for _, a := range e.Artifacts {
			if a.Kind != asset.ArtScript || a.Name == "" {
				continue
			}
			data, err := l.read(a)
			if err != nil {
				if first == nil {
					first = errs.Wrap(err, errs.AssetLoadFailed, "cannot read a script artifact").With(log.F("package", a.Name))
				}
				continue
			}
			out[a.Name] = data
		}
	})
	return out, first
}

func (l *Loader) builtinTexture(id ids.AssetID, d *texture.Data) (texture.Region, bool) {
	h, err := l.up.Upload(d, texture.UploadOptions{Label: "builtin"})
	if err != nil {
		l.fail(id, "cannot upload a builtin texture", err)
		return texture.Region{}, false
	}
	l.pages[id] = pageEntry{handle: h, width: d.Width, height: d.Height, filter: gpu.FilterNearest}
	reg := texture.Whole(h, d.Width, d.Height)
	l.region[id] = regionEntry{region: reg, page: id}
	return reg, true
}

func (l *Loader) Texture(id ids.AssetID) (texture.Region, bool) {
	if r, ok := l.region[id]; ok {
		return r.region, true
	}
	if l.failed[id] {
		return texture.Region{}, false
	}
	if d, ok := asset.BuiltinTexture(id); ok {
		return l.builtinTexture(id, d)
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
	l.normals[id] = ra.Normal
	l.emissions[id] = ra.Emission
	page, ok := l.pages[ra.Page]
	if !ok {
		pe, pok := l.entry(ra.Page)
		if !pok {
			l.fail(id, "sprite region points at a missing page", nil)
			return texture.Region{}, false
		}
		page, ok = l.loadPage(ra.Page, pe)
		if !ok {
			return texture.Region{}, false
		}
	}
	whole := texture.Whole(page.handle, page.width, page.height)
	reg := whole.Sub(ra.X, ra.Y, ra.W, ra.H)
	reg.SourceW, reg.SourceH = uint16(ra.SourceW), uint16(ra.SourceH)
	reg.TrimX, reg.TrimY = uint16(ra.TrimX), uint16(ra.TrimY)
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
		NonColor:      ta.Semantic != asset.TextureColor,
	}
	h, err := l.up.Upload(d, texture.UploadOptions{Mips: ta.Mips, Label: "asset"})
	if err != nil {
		return pageEntry{}, err
	}
	return pageEntry{handle: h, width: ta.Width, height: ta.Height, filter: ta.Filter, semantic: ta.Semantic}, nil
}

func (l *Loader) readMesh(e *asset.Entry) (*mesh.Data, error) {
	art, ok := e.Artifact(asset.ArtMesh)
	if !ok {
		return nil, errs.New(errs.AssetBadArtifact, "mesh artifact is missing", log.F("asset", e.ID.String()))
	}
	raw, err := l.read(art)
	if err != nil {
		return nil, err
	}
	return mesh.Decode(raw)
}

func (l *Loader) Mesh(id ids.AssetID) (*mesh.Mesh, bool) {
	if m, ok := l.meshes[id]; ok {
		return m, true
	}
	if id.IsZero() || l.failed[id] {
		return nil, false
	}
	data, ok := asset.BuiltinMesh(id)
	if !ok {
		e, found := l.entry(id)
		if !found || e.Kind != asset.ArtMesh {
			return nil, false
		}
		var err error
		data, err = l.readMesh(e)
		if err != nil {
			l.fail(id, "cannot load the mesh", err)
			return nil, false
		}
	}
	m, err := mesh.Upload(l.dev, data, "asset.mesh")
	if err != nil {
		l.fail(id, "cannot upload the mesh", err)
		return nil, false
	}
	l.meshes[id] = m
	return m, true
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
	if asset.IsBuiltin(id) {
		switch kind {
		case asset.ArtShader:
			return asset.BuiltinShader(id)
		case asset.ArtMaterial:
			return asset.BuiltinMaterial(id)
		}
	}
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

func (l *Loader) Data(id ids.AssetID) ([]byte, bool) {
	return l.blob(id, asset.ArtData)
}

func (l *Loader) Table(id ids.AssetID) ([]byte, bool) {
	return l.blob(id, asset.ArtTable)
}

func (l *Loader) Audio(id ids.AssetID) ([]byte, bool) {
	if data, ok := asset.BuiltinAudio(id); ok {
		return data, true
	}
	return l.blob(id, asset.ArtAudio)
}

func (l *Loader) Anim(id ids.AssetID) ([]byte, bool) {
	if src, ok := asset.BuiltinAnimText(id); ok {
		return l.builtinAnim(id, src)
	}
	return l.blob(id, asset.ArtAnim)
}

func (l *Loader) Controller(id ids.AssetID) ([]byte, bool) {
	if src, ok := asset.BuiltinCtrlText(id); ok {
		return l.builtinCtrl(id, src)
	}
	return l.blob(id, asset.ArtCtrl)
}

func (l *Loader) builtinCtrl(id ids.AssetID, src string) ([]byte, bool) {
	if b, ok := l.blobs[id]; ok {
		return b, true
	}
	if l.failed[id] {
		return nil, false
	}
	doc, err := aetext.Parse([]byte(src), "builtin.aectrl")
	if err != nil {
		l.fail(id, "cannot parse a builtin controller", err)
		return nil, false
	}
	g, probs, err := anim.DecodeGraph(doc)
	if err == nil && (len(probs) > 0 || len(g.Check(nil)) > 0) {
		err = errs.New(errs.AssetBadArtifact, "builtin controller has problems")
	}
	if err != nil {
		l.fail(id, "cannot decode a builtin controller", err)
		return nil, false
	}
	g.Layout()
	data := anim.MarshalGraph(g)
	l.blobs[id] = data
	return data, true
}

func (l *Loader) builtinAnim(id ids.AssetID, src string) ([]byte, bool) {
	if b, ok := l.blobs[id]; ok {
		return b, true
	}
	if l.failed[id] {
		return nil, false
	}
	doc, err := aetext.Parse([]byte(src), "builtin.aeanim")
	if err != nil {
		l.fail(id, "cannot parse a builtin clip", err)
		return nil, false
	}
	c, err := anim.Decode(doc)
	if err != nil {
		l.fail(id, "cannot decode a builtin clip", err)
		return nil, false
	}
	data := anim.Marshal(c)
	l.blobs[id] = data
	return data, true
}

func (l *Loader) Revision() uint64 {
	return uint64(l.gen)
}

func (l *Loader) Index() *asset.Index {
	return l.index
}

func (l *Loader) Artifacts() vfs.FS {
	return l.files
}

func (l *Loader) Scene(id ids.AssetID) ([]byte, bool) {
	return l.blob(id, asset.ArtScene)
}

func (l *Loader) Prefab(id ids.AssetID) ([]byte, bool) {
	return l.blob(id, asset.ArtPrefab)
}

func (l *Loader) Preset(id ids.AssetID) ([]byte, bool) { return l.blob(id, asset.ArtPreset) }

func (l *Loader) Preload(list ...ids.AssetID) error {
	for _, id := range list {
		if b, ok := asset.LookupBuiltin(id); ok {
			loaded := false
			switch b.Kind {
			case asset.ArtTexture:
				_, loaded = l.Texture(id)
			case asset.ArtAudio:
				_, loaded = l.Audio(id)
			case asset.ArtAnim:
				_, loaded = l.Anim(id)
			case asset.ArtCtrl:
				_, loaded = l.Controller(id)
			case asset.ArtShader:
				_, loaded = l.Shader(id)
			case asset.ArtMaterial:
				_, loaded = l.Material(id)
			default:
				_, loaded = l.Mesh(id)
			}
			if !loaded {
				return errs.New(errs.AssetLoadFailed, "builtin asset failed to load", log.F("asset", id.String()))
			}
			continue
		}
		e, ok := l.entry(id)
		if !ok {
			return errs.New(errs.AssetNotFound, "asset is not in the index", log.F("asset", id.String()))
		}
		ok = true
		switch e.Kind {
		case asset.ArtTexture, asset.ArtAtlasPage, asset.ArtSpriteRegion:
			_, ok = l.Texture(id)
		case asset.ArtMesh:
			_, ok = l.Mesh(id)
		case asset.ArtScene:
			_, ok = l.Scene(id)
		case asset.ArtPrefab:
			_, ok = l.Prefab(id)
		case asset.ArtPreset:
			_, ok = l.Preset(id)
		case asset.ArtData:
			_, ok = l.Data(id)
		case asset.ArtTable:
			_, ok = l.Table(id)
		case asset.ArtAudio:
			_, ok = l.Audio(id)
		case asset.ArtAnim:
			_, ok = l.Anim(id)
		case asset.ArtCtrl:
			_, ok = l.Controller(id)
		case asset.ArtShader:
			_, ok = l.Shader(id)
		case asset.ArtMaterial:
			_, ok = l.Material(id)
		}
		if !ok {
			return errs.New(errs.AssetLoadFailed, "asset failed to load", log.F("asset", id.String()), log.F("kind", string(e.Kind)))
		}
	}
	return nil
}

type staged struct {
	id        ids.AssetID
	kind      asset.ArtifactKind
	page      pageEntry
	geo       *mesh.Mesh
	blob      []byte
	isTex     bool
	unchanged bool
}

func (l *Loader) prepare(e *asset.Entry) (staged, bool, error) {
	switch e.Kind {
	case asset.ArtTexture, asset.ArtAtlasPage:
		art, ok := e.Artifact(e.Kind)
		if !ok {
			return staged{}, false, errs.New(errs.AssetBadArtifact, "entry has no artifact of its own kind", log.F("asset", e.ID.String()))
		}
		data, err := l.read(art)
		if err != nil {
			return staged{}, false, err
		}
		if region, found := e.Artifact(asset.ArtSpriteRegion); found {
			encoded, err := l.read(region)
			if err != nil {
				return staged{}, false, err
			}
			if _, err := asset.DecodeRegion(encoded); err != nil {
				return staged{}, false, err
			}
		}
		p, err := l.uploadTexture(data)
		if err != nil {
			return staged{}, false, err
		}
		return staged{id: e.ID, kind: e.Kind, page: p, isTex: true}, true, nil
	case asset.ArtMesh:
		data, err := l.readMesh(e)
		if err != nil {
			return staged{}, false, err
		}
		m, err := mesh.Upload(l.dev, data, "asset.mesh")
		if err != nil {
			return staged{}, false, err
		}
		return staged{id: e.ID, kind: e.Kind, geo: m}, true, nil
	case asset.ArtScript:
		return staged{id: e.ID, kind: e.Kind}, true, nil
	case asset.ArtSpriteRegion, asset.ArtScene, asset.ArtPrefab, asset.ArtPreset, asset.ArtData, asset.ArtTable, asset.ArtAudio, asset.ArtAnim, asset.ArtCtrl,
		asset.ArtShader, asset.ArtShaderInclude, asset.ArtMaterial:
		art, ok := e.Artifact(e.Kind)
		if !ok {
			return staged{}, false, errs.New(errs.AssetBadArtifact, "entry has no artifact of its own kind", log.F("asset", e.ID.String()))
		}
		data, err := l.read(art)
		if err != nil {
			return staged{}, false, err
		}
		if e.Kind == asset.ArtShader {
			if _, err := asset.DecodeShader(data); err != nil {
				return staged{}, false, err
			}
		}
		if e.Kind == asset.ArtMaterial {
			if _, err := asset.DecodeMaterial(data, e.Source); err != nil {
				return staged{}, false, err
			}
		}
		if e.Kind == asset.ArtSpriteRegion {
			if _, err := asset.DecodeRegion(data); err != nil {
				return staged{}, false, err
			}
		}
		return staged{id: e.ID, kind: e.Kind, blob: data, unchanged: l.sameUncachedBlob(e, art, len(data))}, true, nil
	}
	return staged{}, false, nil
}

// A dependency reimport may produce identical scene or data bytes. The
// mounted artifact identity still describes the previous generation when no
// consumer has read the blob yet (for example, run --scene reads its path).
// Compare it before committing the new index. Cached bytes keep their stronger
// direct comparison in Swap; shaders, materials and sprite regions retain their
// own resource publication rules.
func (l *Loader) sameUncachedBlob(next *asset.Entry, art asset.Artifact, size int) bool {
	switch next.Kind {
	case asset.ArtScene, asset.ArtPrefab, asset.ArtPreset, asset.ArtData, asset.ArtTable, asset.ArtAudio, asset.ArtAnim, asset.ArtCtrl:
	default:
		return false
	}
	if _, resident := l.blobs[next.ID]; resident || l.failed[next.ID] || art.Hash == "" || art.Size != int64(size) {
		return false
	}
	old, found := l.index.Lookup(next.ID)
	if !found || old.Missing || old.Error != "" || old.Kind != next.Kind {
		return false
	}
	previous, found := old.Artifact(next.Kind)
	return found && previous.Kind == art.Kind && previous.Size == art.Size && previous.Hash == art.Hash
}

func (l *Loader) Swap(entries []asset.Entry, files vfs.FS) error {
	previousFiles := l.files
	if files != nil {
		l.files = files
	}
	if l.index == nil {
		l.files = previousFiles
		return errs.New(errs.AssetNotMounted, "loader has no index mounted")
	}
	prepared := make([]staged, 0, len(entries))
	for i := range entries {
		s, ok, err := l.prepare(&entries[i])
		if err != nil {
			l.rollback(prepared)
			l.files = previousFiles
			for _, entry := range entries {
				if entry.Kind == asset.ArtShader || entry.Kind == asset.ArtMaterial {
					l.RejectSource(entry.ID)
				}
			}
			return err
		}
		if ok {
			prepared = append(prepared, s)
		}
	}
	if err := l.validateNormalSwap(entries, prepared); err != nil {
		l.rollback(prepared)
		l.files = previousFiles
		return err
	}
	for i := range entries {
		l.index.Put(entries[i])
	}
	for i := range prepared {
		s := &prepared[i]
		delete(l.failed, s.id)
		delete(l.normals, s.id)
		delete(l.emissions, s.id)
		delete(l.rejected, s.id)
		switch {
		case s.isTex:
			if old, ok := l.pages[s.id]; ok {
				l.retireMaterialTexture(old.handle)
			}
			l.pages[s.id] = s.page
			l.invalidateRegionsOf(s.id)
		case s.geo != nil:
			if old, ok := l.meshes[s.id]; ok {
				old.Destroy(l.dev)
			}
			l.meshes[s.id] = s.geo
		case s.kind == asset.ArtScript:
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
	l.queueMaterialChanges(prepared)
	// A prepared shader/material is not a published runtime revision. Its
	// schema, values and notifications become visible together at Cache.End.
	publish := false
	for i := range prepared {
		s := &prepared[i]
		deferred := (s.kind == asset.ArtShader && l.mat.shaders[s.id] != nil) || (s.kind == asset.ArtMaterial && l.mat.mats[s.id] != nil)
		if !s.unchanged && !deferred {
			publish = true
		}
	}
	if publish {
		clear(l.decoded)
		l.bumpGeneration()
	}
	for i := range prepared {
		s := &prepared[i]
		if s.unchanged || (s.kind == asset.ArtShader && l.mat.shaders[s.id] != nil) || (s.kind == asset.ArtMaterial && l.mat.mats[s.id] != nil) {
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
		if s.geo != nil {
			s.geo.Destroy(l.dev)
		}
	}
}

func (l *Loader) Drop(id ids.AssetID) {
	if l.index != nil {
		l.index.Delete(id)
	}
	if p, ok := l.pages[id]; ok {
		l.retireMaterialTexture(p.handle)
		delete(l.pages, id)
		l.invalidateRegionsOf(id)
	}
	if m, ok := l.meshes[id]; ok {
		m.Destroy(l.dev)
		delete(l.meshes, id)
	}
	delete(l.region, id)
	delete(l.normals, id)
	delete(l.emissions, id)
	delete(l.rejected, id)
	delete(l.blobs, id)
	delete(l.failed, id)
	l.dropMaterial(id)
	l.dropShader(id)
	l.sweepMaterialTextures()
	clear(l.decoded)
	l.bumpGeneration()
}

func (l *Loader) Close() {
	clear(l.textureMetadata)
	l.masks = maskCache{}
	for id := range l.mat.mats {
		l.dropMaterial(id)
	}
	for id := range l.mat.shaders {
		l.dropShader(id)
	}
	l.sweepMaterialTextures()
	for id, p := range l.pages {
		l.dev.DestroyTexture(p.handle)
		delete(l.pages, id)
	}
	for id, m := range l.meshes {
		m.Destroy(l.dev)
		delete(l.meshes, id)
	}
	l.region = map[ids.AssetID]regionEntry{}
	clear(l.normals)
	clear(l.emissions)
	clear(l.rejected)
	l.blobs = map[ids.AssetID][]byte{}
	l.failed = map[ids.AssetID]bool{}
	clear(l.mat.shaders)
	clear(l.mat.mats)
	clear(l.mat.uses)
	clear(l.mat.pending)
	l.decoded = map[decodedKey]decodedValue{}
	l.subs = nil
}

var (
	_ asset.Resolver     = (*Loader)(nil)
	_ asset.Decoder      = (*Loader)(nil)
	_ asset.DataSource   = (*Loader)(nil)
	_ asset.AudioSource  = (*Loader)(nil)
	_ asset.AnimSource   = (*Loader)(nil)
	_ asset.CtrlSource   = (*Loader)(nil)
	_ asset.NormalSource = (*Loader)(nil)
)
