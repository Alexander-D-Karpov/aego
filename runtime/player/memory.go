package player

import (
	"strings"

	"aego/asset"
	"aego/core/errs"
	"aego/core/log"
	"aego/runtime/bundle"
	"aego/vfs"
	"aego/vfs/memfs"
)

type Bundle struct {
	Manifest []byte
	Files    map[string][]byte
}

func MountBytes(b *Bundle) (*bundle.Data, *vfs.VFS, error) {
	d, err := bundle.DecodeData(b.Manifest, bundle.ManifestFile)
	if err != nil {
		return nil, nil, err
	}
	prefix := bundle.Scheme + "://"
	var bad error
	d.Index.Each(func(e *asset.Entry) {
		for _, a := range e.Artifacts {
			rel, ok := strings.CutPrefix(a.Path, prefix)
			if !ok || bad != nil {
				continue
			}
			data, ok := b.Files[rel]
			switch {
			case !ok:
				bad = errs.New(errs.AssetNotMounted, "the game data is missing a file", log.F("path", rel))
			case int64(len(data)) != a.Size:
				bad = errs.New(errs.VFSIO, "a game data file differs from the manifest", log.F("path", rel), log.F("size", len(data)), log.F("want", a.Size))
			}
		}
	})
	if bad != nil {
		return nil, nil, bad
	}
	v, err := vfs.New(vfs.Mount{Scheme: bundle.Scheme, Source: memfs.New(b.Files), ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	return d, v, nil
}

func (p *Player) mount() (*bundle.Data, *vfs.VFS, error) {
	if p.cfg.Bundle != nil {
		return MountBytes(p.cfg.Bundle)
	}
	return Mount(p.cfg.Data)
}
