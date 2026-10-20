package player

import (
	"sort"

	"aego/asset/loader"
	"aego/core/errs"
	"aego/core/log"
	"aego/script/bytecode"
)

func Scripts(l *loader.Loader) ([]*bytecode.Module, error) {
	blobs, err := l.Scripts()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(blobs))
	for n := range blobs {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*bytecode.Module, 0, len(names))
	for _, n := range names {
		m, err := bytecode.Decode(blobs[n])
		if err != nil {
			return nil, errs.Wrap(err, errs.AssetBadArtifact, "script package cannot be decoded").With(log.F("package", n))
		}
		out = append(out, m)
	}
	return out, nil
}
