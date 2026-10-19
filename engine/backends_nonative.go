//go:build aego_nonative

package engine

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/platform"
)

func newNativePlatform() (platform.Platform, error) {
	return nil, errs.New(errs.EngineConfig, "built with the aego_nonative tag: no window or GPU backend is linked")
}

func newNativeDevice(platform.Window, Config, *log.Logger) (gpu.Device, error) {
	return nil, nil
}
