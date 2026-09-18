//go:build !aego_nonative

package engine

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/gl33"
	"aego/platform"
	"aego/platform/sdl"
)

func newNativePlatform() (platform.Platform, error) {
	p, err := sdl.New()
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newNativeDevice(win platform.Window, cfg Config, l *log.Logger) (gpu.Device, error) {
	gw, ok := win.(glWindow)
	if !ok {
		return nil, errs.New(errs.GPUInit, "window has no OpenGL context")
	}
	d, err := gl33.New(gw.GLProcAddress, gl33.Options{Debug: cfg.Debug, Log: l})
	if err != nil {
		return nil, err
	}
	return d, nil
}
