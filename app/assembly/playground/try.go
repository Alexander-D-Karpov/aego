package playground

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"aego/asset"
	"aego/asset/loader"
	"aego/core/ids"
	"aego/script/bytecode"
	"aego/vfs"
	"aego/vfs/memfs"
)

// EncodeTry packs the compiled script of a try scene for a docs page: base64 modules joined by commas.
func EncodeTry(mods []*bytecode.Module) string {
	parts := make([]string, len(mods))
	for i, m := range mods {
		parts[i] = base64.StdEncoding.EncodeToString(bytecode.Encode(m))
	}
	return strings.Join(parts, ",")
}

func DecodeTry(code string) ([]*bytecode.Module, error) {
	if code == "" {
		return nil, nil
	}
	var mods []*bytecode.Module
	for _, part := range strings.Split(code, ",") {
		b, err := base64.StdEncoding.DecodeString(part)
		if err != nil {
			return nil, err
		}
		m, err := bytecode.Decode(b)
		if err != nil {
			return nil, err
		}
		mods = append(mods, m)
	}
	return mods, nil
}

// MountTry validates before replacing a documentation session's prefabs and optional prepared artifacts; it never imports source textures/materials/shaders.
func MountTry(l *loader.Loader, prefabs map[string]string, prepared ...[]byte) error {
	if l == nil || len(prepared) > 1 || len(prefabs) > asset.MaxTryAssets {
		return fmt.Errorf("invalid try resource mount")
	}
	idx := asset.NewIndex()
	files := map[string][]byte{}
	total := 0
	if len(prepared) != 0 && len(prepared[0]) != 0 {
		bundle, err := asset.DecodeTryArtifacts(prepared[0])
		if err != nil {
			return err
		}
		total += len(prepared[0])
		for _, entry := range bundle.Assets {
			e := asset.Entry{ID: entry.ID, Kind: entry.Kind, Name: entry.Name, Source: entry.Name, Deps: entry.Deps}
			for i, artifact := range entry.Artifacts {
				p := fmt.Sprintf("%s/%d", entry.ID, i)
				hash := sha256.Sum256(artifact.Data)
				files[p] = artifact.Data
				e.Artifacts = append(e.Artifacts, asset.Artifact{Kind: artifact.Kind, Name: artifact.Name, Path: "try://" + p, Hash: hex.EncodeToString(hash[:]), Size: int64(len(artifact.Data))})
			}
			idx.Put(e)
		}
	}
	for key, text := range prefabs {
		id, err := ids.ParseAssetID(key)
		if err != nil {
			return err
		}
		_, duplicate := idx.Lookup(id)
		total += len(text)
		if id.IsZero() || asset.IsBuiltin(id) || duplicate || len(text) > asset.MaxTryArtifactBytes || total > asset.MaxTryArtifactsBytes || idx.Len() >= asset.MaxTryAssets {
			return fmt.Errorf("try prefab ID, duplicate or byte budget is invalid")
		}
		p := id.String() + ".aeprefab"
		files[p] = []byte(text)
		idx.Put(asset.Entry{ID: id, Kind: asset.ArtPrefab, Name: p, Source: p,
			Artifacts: []asset.Artifact{{Kind: asset.ArtPrefab, Path: "try://" + p}}})
	}
	fs, err := vfs.New(vfs.Mount{Scheme: "try", ReadOnly: true, Source: memfs.New(files)})
	if err != nil {
		return err
	}
	// Mount only checks non-nil inputs; retire the old session before publishing the ready FS/index so Drop cannot remove reused IDs or leave stale GPU caches.
	var previous []ids.AssetID
	if old := l.Index(); old != nil {
		old.Each(func(entry *asset.Entry) {
			if !asset.IsBuiltin(entry.ID) {
				previous = append(previous, entry.ID)
			}
		})
	}
	for _, id := range previous {
		l.Drop(id)
	}
	return l.Mount(idx, fs)
}
