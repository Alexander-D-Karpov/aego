//go:build !darwin

package remoteexample

func platformStorePath(abs string) (string, error) { return abs, nil }
