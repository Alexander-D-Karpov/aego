package shaderx

import "aego/render/shader"

// MapLog is retained for dev callers; runtime code uses shader.MapLog directly.
func MapLog(log string, locs []Loc, files []string, code string) []Diag {
	return shader.MapLog(log, locs, files, code)
}
