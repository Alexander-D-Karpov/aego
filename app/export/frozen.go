package export

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"aego/asset"
	"aego/engine/problems"
	"aego/vfs"
	"aego/vfs/memfs"
)

// Export owns immutable bytes. Reimport overwrites cache paths in place; an
// index clone alone cannot keep an asynchronous export on one revision.
func freezeArtifacts(idx *asset.Index, files vfs.FS) (vfs.FS, []problems.Diagnostic) {
	mounts := map[string]map[string][]byte{}
	var out []problems.Diagnostic
	idx.Each(func(e *asset.Entry) {
		if e.Missing {
			return
		}
		for _, a := range e.Artifacts {
			b, err := files.ReadFile(a.Path)
			if err == nil && a.Hash != "" {
				sum := sha256.Sum256(b)
				if hex.EncodeToString(sum[:8]) != a.Hash || int64(len(b)) != a.Size {
					err = fmt.Errorf("artifact bytes do not match the imported revision: %s", a.Path)
				}
			}
			scheme, name, valid := strings.Cut(a.Path, "://")
			if err == nil && (!valid || scheme == "" || name == "") {
				err = fmt.Errorf("invalid artifact path %q", a.Path)
			}
			if err != nil {
				out = append(out, diag(CodeImport, problems.Error, "cannot freeze export snapshot: "+err.Error(), e.Source))
				continue
			}
			if mounts[scheme] == nil {
				mounts[scheme] = map[string][]byte{}
			}
			mounts[scheme][name] = slices.Clone(b)
		}
	})
	frozen, _ := vfs.New()
	for scheme, data := range mounts {
		if err := frozen.Mount(vfs.Mount{Scheme: scheme, ReadOnly: true, Source: memfs.New(data)}); err != nil {
			out = append(out, diag(CodeInternal, problems.Error, "cannot freeze export snapshot: "+err.Error(), ""))
		}
	}
	return frozen, out
}
