package playground

import (
	"encoding/base64"
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

// MountTry gives the loader the prefabs of a try scene, keyed by asset id; none unmounts them.
func MountTry(l *loader.Loader, prefabs map[string]string) error {
	idx := asset.NewIndex()
	files := map[string][]byte{}
	for key, text := range prefabs {
		id, err := ids.ParseAssetID(key)
		if err != nil {
			return err
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
	return l.Mount(idx, fs)
}
