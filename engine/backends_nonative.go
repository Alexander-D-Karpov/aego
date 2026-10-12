//go:build aego_nonative || js

package engine

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/platform"
)

func newNativePlatform() (platform.Platform, error) {
	return nil, errs.New(errs.EngineConfig, "built with the aego_nonative tag: SDL and OpenGL backends are not linked")
}

func newNativeDevice(win platform.Window, cfg Config, l *log.Logger) (gpu.Device, error) {
	return nil, errs.New(errs.GPUInit, "built with the aego_nonative tag: SDL and OpenGL backends are not linked")
}
