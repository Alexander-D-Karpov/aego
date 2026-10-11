//go:build aego_runtime

package scriptengine

import "aego/engine/module"

const reflectFallback = false

func registerDev(*module.Registry) {}
