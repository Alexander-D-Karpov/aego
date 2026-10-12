//go:build !aego_runtime && js

package scriptengine

import "aego/engine/module"

const reflectFallback = true

func registerDev(r *module.Registry) {
	registerDocs(r)
}
