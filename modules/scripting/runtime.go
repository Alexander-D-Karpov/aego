//go:build aego_runtime

package scripting

import "aego/engine/module"

const reflectFallback = false

func registerDev(*module.Registry) {}
