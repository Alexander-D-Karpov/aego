//go:build !js

package audio

func (*Module) newWorkerBackend() (workerBackend, error) { return nil, nil }
