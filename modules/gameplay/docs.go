//go:build !aego_runtime

package gameplay

import (
	"embed"
	"io/fs"

	"aego/engine/module"
)

//go:embed res/docs
var docFiles embed.FS

func registerDocs(r *module.Registry) {
	if sub, err := fs.Sub(docFiles, "res/docs"); err == nil {
		r.Docs(sub)
	}
}
