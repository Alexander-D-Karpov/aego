//go:build js && !aego_nonative

package engine

import (
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/webgl2"
	"aego/platform"
	"aego/platform/web"
)

func newNativePlatform() (platform.Platform, error) {
	p, err := web.New(web.Options{})
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newNativeDevice(win platform.Window, cfg Config, l *log.Logger) (gpu.Device, error) {
	s, ok := win.(webgl2.Surface)
	if !ok {
		return nil, nil
	}
	d, err := webgl2.New(s, webgl2.Options{Debug: cfg.Debug, Log: l})
	if err != nil {
		return nil, err
	}
	return d, nil
}
