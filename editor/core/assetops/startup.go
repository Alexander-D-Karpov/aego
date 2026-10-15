package assetops

import (
	"aego/core/ids"
	"aego/editor/core/context"
)

func SetStartup(ctx *context.Editor, id ids.AssetID, rel string) {
	if ctx.Project == nil {
		return
	}
	prev := ctx.Project.MainScene
	ctx.Project.MainScene = id
	if err := ctx.Project.Save(); err != nil {
		ctx.Project.MainScene = prev
		ctx.Notify(context.LevelError, err.Error())
		return
	}
	ctx.Notify(context.LevelInfo, "Startup scene: "+rel)
}
