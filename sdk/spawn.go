package sdk

import (
	"aego/modules/spawn"
	"aego/scene"
)

type (
	Spawner = spawn.Service
	Tags    = scene.Tags
)

func SpawnerOf(ctx *Context) *Spawner {
	if ctx == nil {
		return nil
	}
	return spawn.Of(ctx.World)
}
