package sdk

import "aego/modules/camera"

type (
	Cameras    = camera.Service
	CameraPose = camera.Pose
)

func CamerasOf(ctx *Context) *Cameras {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	return camera.Of(ctx.World)
}
